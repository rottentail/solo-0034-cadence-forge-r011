package timeline

import (
	"cadenceforge/internal/domain"
)

// Build resolves the confirmed source-to-output time relationship for a
// sheet revision. The same plan drives picture encoding and caption
// mapping, so both always share one time semantics. A delivery target
// whose conflict was never confirmed fails here instead of silently
// picking a speed.
func Build(sheet domain.Sheet, recipe domain.Recipe, sources map[string]domain.Source) (domain.Plan, error) {
	if err := domain.ValidateSheet(sheet, sources); err != nil {
		return domain.Plan{}, err
	}
	if err := domain.ValidateRecipe(recipe); err != nil {
		return domain.Plan{}, err
	}
	pieces, tempo, err := resolvePieces(sheet, sources)
	if err != nil {
		return domain.Plan{}, err
	}
	plan := domain.Plan{Recipe: recipe, Placements: []domain.Placement{}, Tempo: &tempo}
	segments, total := planSegments(pieces)
	for i, span := range sheet.Spans {
		source := sources[span.SourceID]
		if recipe.Format == "mp4" && !source.Media.Video {
			return plan, domain.Invalid("MP4 span requires video: %s", source.ID)
		}
		if recipe.Format == "wav" && !source.Media.Audio {
			return plan, domain.Invalid("WAV span requires audio: %s", source.ID)
		}
		placement := domain.Placement{SourceID: source.ID, SourceInMS: span.InMS, SourceOutMS: span.OutMS, GainDB: span.GainDB, Segments: segments[i]}
		placement.OutputInMS = segments[i][0].OutputInMS
		placement.OutputOutMS = segments[i][len(segments[i])-1].OutputOutMS
		plan.Placements = append(plan.Placements, placement)
	}
	plan.DurationMS = total
	if plan.DurationMS > 120000 {
		return plan, domain.Invalid("composition exceeds 120 seconds")
	}
	return plan, nil
}
