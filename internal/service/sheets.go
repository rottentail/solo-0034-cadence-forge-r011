package service

import (
	"cadenceforge/internal/caption"
	"cadenceforge/internal/domain"
	"cadenceforge/internal/timeline"
	"time"
)

func (s *Service) SaveSheet(id string, expected int, input domain.Sheet) (domain.Sheet, error) {
	s.mutation.Lock()
	defer s.mutation.Unlock()
	if id == "" {
		id = ID()
	}
	var out domain.Sheet
	err := s.Store.Update(func(st *domain.State) error {
		versions := st.Sheets[id]
		if len(versions) != expected {
			return domain.Conflict("sheet revision changed")
		}
		if expected == 0 && len(versions) > 0 {
			return domain.Conflict("sheet already exists")
		}
		if expected > 0 && len(versions) == 0 {
			return domain.Missing("sheet", id)
		}
		if err := domain.ValidateSheet(input, st.Sources); err != nil {
			return err
		}
		input.ID = id
		input.Revision = len(versions) + 1
		input.CreatedAt = time.Now().UTC()
		// A fit confirmation belongs to the exact revision it was
		// reviewed on; edited content must be reviewed again.
		input.Resolution = ""
		out = input
		st.Sheets[id] = append(versions, input)
		st.Emit(id, "sheet.revised", "immutable timeline revision saved")
		return nil
	})
	return out, err
}

// ResolveSheet confirms one reviewable fit choice as a new immutable
// revision. The confirmed revision is the only source-to-output time
// relationship preview, submit, captions and renders will use.
func (s *Service) ResolveSheet(id string, expected int, choice string) (domain.Sheet, error) {
	s.mutation.Lock()
	defer s.mutation.Unlock()
	var out domain.Sheet
	err := s.Store.Update(func(st *domain.State) error {
		versions := st.Sheets[id]
		if len(versions) == 0 {
			return domain.Missing("sheet", id)
		}
		if len(versions) != expected {
			return domain.Conflict("sheet revision changed")
		}
		latest := versions[len(versions)-1]
		report, err := timeline.Fit(latest, st.Sources)
		if err != nil {
			return err
		}
		if report.Status != "confirmation_required" {
			return domain.Conflict("sheet revision has no pending fit choice")
		}
		valid := false
		for _, c := range report.Candidates {
			if c.ID == choice {
				valid = true
			}
		}
		if !valid {
			return domain.Invalid("unknown fit choice %q", choice)
		}
		latest.Resolution = choice
		latest.Revision = len(versions) + 1
		latest.CreatedAt = time.Now().UTC()
		out = latest
		st.Sheets[id] = append(versions, latest)
		st.Emit(id, "sheet.resolved", choice)
		return nil
	})
	return out, err
}

// Fit reports how the latest sheet revision's delivery intent resolves,
// including the reviewable candidates when dialogue and picture pace
// cannot both reach the target.
func (s *Service) Fit(sheetID string) (domain.FitReport, error) {
	st, err := s.Store.Snapshot()
	if err != nil {
		return domain.FitReport{}, err
	}
	versions := st.Sheets[sheetID]
	if len(versions) == 0 {
		return domain.FitReport{}, domain.Missing("sheet", sheetID)
	}
	return timeline.Fit(versions[len(versions)-1], st.Sources)
}
func (s *Service) Preview(sheetID, recipeID string) (domain.Plan, error) {
	st, err := s.Store.Snapshot()
	if err != nil {
		return domain.Plan{}, err
	}
	sheets := st.Sheets[sheetID]
	recipes := st.Recipes[recipeID]
	if len(sheets) == 0 {
		return domain.Plan{}, domain.Missing("sheet", sheetID)
	}
	if len(recipes) == 0 {
		return domain.Plan{}, domain.Missing("recipe", recipeID)
	}
	sheet := sheets[len(sheets)-1]
	recipe := recipes[len(recipes)-1]
	plan, err := timeline.Build(sheet, recipe, st.Sources)
	if err != nil {
		return plan, err
	}
	plan.VTT = caption.VTT(caption.Map(sheet, plan))
	return plan, nil
}
