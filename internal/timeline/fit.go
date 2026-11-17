package timeline

import (
	"cadenceforge/internal/domain"
	"fmt"
	"math"
)

// piece is one constant-rate partition of a span in source milliseconds.
// locked pieces carry pace-protected dialogue; the solver assigns rates.
type piece struct {
	in, out int64
	locked  bool
	rate    float64
}

const (
	minPieceMS  = 40
	maxPieces   = 1024
	strategyFit = "fit"
)

// resolvePieces computes the confirmed constant-rate partition of every
// span. It is the single place where source-to-output tempo is decided;
// preview, submit, captions and encoding all consume its result.
func resolvePieces(sheet domain.Sheet, sources map[string]domain.Source) ([][]piece, domain.TempoInfo, error) {
	natural := naturalMS(sheet)
	if hasExplicitTempo(sheet) {
		return explicitPieces(sheet), domain.TempoInfo{Strategy: "explicit", NaturalMS: natural}, nil
	}
	if sheet.Delivery == nil {
		return identityPieces(sheet), domain.TempoInfo{Strategy: "natural", NaturalMS: natural}, nil
	}
	d := sheet.Delivery
	info := domain.TempoInfo{TargetMS: d.TargetMS, NaturalMS: natural}
	if natural <= d.TargetMS {
		info.Strategy = "natural"
		return identityPieces(sheet), info, nil
	}
	pieces, locked, unlocked := partitionSpans(sheet)
	if countPieces(pieces) > maxPieces {
		return nil, info, domain.Invalid("delivery fit exceeds %d tempo pieces", maxPieces)
	}
	dialogueCap, hardCap := rateCaps(d)
	strategy := sheet.Resolution
	if strategy == "" {
		strategy = strategyFit
	}
	dialogueRate, otherRate, ok := solveRates(locked, unlocked, d.TargetMS, dialogueCap, hardCap, strategy)
	if !ok {
		return nil, info, domain.Conflict(fmt.Sprintf("delivery target %dms conflicts with protected dialogue; review candidates via the fit endpoint and confirm a choice", d.TargetMS))
	}
	assignRates(pieces, dialogueRate, otherRate)
	info.Strategy = strategy
	info.DialogueRate = dialogueRate
	info.OtherRate = otherRate
	return pieces, info, nil
}

// Fit evaluates the latest sheet revision's delivery intent and reports
// either the resolved strategy or the reviewable conflict candidates.
func Fit(sheet domain.Sheet, sources map[string]domain.Source) (domain.FitReport, error) {
	report := domain.FitReport{SheetID: sheet.ID, Revision: sheet.Revision, Status: "resolved", Strategy: "natural"}
	if err := domain.ValidateSheet(sheet, sources); err != nil {
		return report, err
	}
	natural := naturalMS(sheet)
	report.NaturalMS = natural
	if hasExplicitTempo(sheet) {
		report.Strategy = "explicit"
		return report, nil
	}
	if sheet.Delivery == nil {
		return report, nil
	}
	d := sheet.Delivery
	report.TargetMS = d.TargetMS
	if natural <= d.TargetMS {
		return report, nil
	}
	pieces, locked, unlocked := partitionSpans(sheet)
	if countPieces(pieces) > maxPieces {
		return report, domain.Invalid("delivery fit exceeds %d tempo pieces", maxPieces)
	}
	report.LockedMS = locked
	report.UnlockedMS = unlocked
	dialogueCap, hardCap := rateCaps(d)
	if sheet.Resolution != "" {
		dialogueRate, otherRate, ok := solveRates(locked, unlocked, d.TargetMS, dialogueCap, hardCap, sheet.Resolution)
		if !ok {
			return report, domain.Conflict("confirmed fit choice no longer satisfies the sheet")
		}
		report.Strategy = sheet.Resolution
		assignRates(pieces, dialogueRate, otherRate)
		return report, nil
	}
	if _, _, ok := solveRates(locked, unlocked, d.TargetMS, dialogueCap, hardCap, strategyFit); ok {
		report.Strategy = strategyFit
		return report, nil
	}
	report.Status = "confirmation_required"
	report.Strategy = ""
	report.Candidates = candidates(pieces, locked, unlocked, d.TargetMS, dialogueCap, hardCap)
	return report, nil
}

func naturalMS(sheet domain.Sheet) int64 {
	var total int64
	for _, p := range sheet.Spans {
		total += p.OutMS - p.InMS
	}
	return total
}
func hasExplicitTempo(sheet domain.Sheet) bool {
	for _, p := range sheet.Spans {
		if len(p.Tempo) > 0 {
			return true
		}
	}
	return false
}
func identityPieces(sheet domain.Sheet) [][]piece {
	out := make([][]piece, 0, len(sheet.Spans))
	for _, p := range sheet.Spans {
		out = append(out, []piece{{in: p.InMS, out: p.OutMS, rate: 1}})
	}
	return out
}
func explicitPieces(sheet domain.Sheet) [][]piece {
	out := make([][]piece, 0, len(sheet.Spans))
	for _, p := range sheet.Spans {
		zones := []piece{}
		if len(p.Tempo) == 0 {
			zones = append(zones, piece{in: p.InMS, out: p.OutMS, rate: 1})
		}
		for _, z := range p.Tempo {
			zones = append(zones, piece{in: z.InMS, out: z.OutMS, rate: z.Rate})
		}
		out = append(out, zones)
	}
	return out
}
func rateCaps(d *domain.Delivery) (dialogue, hard float64) {
	dialogue, hard = d.MaxDialogueRate, d.MaxRate
	if dialogue == 0 {
		dialogue = 1
	}
	if hard == 0 {
		hard = 4
	}
	return dialogue, hard
}
func countPieces(pieces [][]piece) int {
	n := 0
	for _, span := range pieces {
		n += len(span)
	}
	return n
}

// partitionSpans splits every span occurrence at the boundaries of
// pace-locked cue intervals. Repeated source occurrences are partitioned
// independently, so each appearance protects its own dialogue.
func partitionSpans(sheet domain.Sheet) ([][]piece, int64, int64) {
	out := make([][]piece, 0, len(sheet.Spans))
	var lockedTotal, unlockedTotal int64
	for _, p := range sheet.Spans {
		locks := [][2]int64{}
		for _, c := range sheet.Cues {
			if !c.PaceLock || c.SourceID != p.SourceID {
				continue
			}
			start, end := max(c.StartMS, p.InMS), min(c.EndMS, p.OutMS)
			if end > start {
				locks = append(locks, [2]int64{start, end})
			}
		}
		pieces := mergeSlivers(partition(p.InMS, p.OutMS, union(locks)))
		for _, pc := range pieces {
			if pc.locked {
				lockedTotal += pc.out - pc.in
			} else {
				unlockedTotal += pc.out - pc.in
			}
		}
		out = append(out, pieces)
	}
	return out, lockedTotal, unlockedTotal
}
func union(intervals [][2]int64) [][2]int64 {
	for i := 0; i < len(intervals); i++ {
		for j := i + 1; j < len(intervals); j++ {
			if intervals[j][0] < intervals[i][0] {
				intervals[i], intervals[j] = intervals[j], intervals[i]
			}
		}
	}
	out := [][2]int64{}
	for _, iv := range intervals {
		if n := len(out); n > 0 && iv[0] <= out[n-1][1] {
			out[n-1][1] = max(out[n-1][1], iv[1])
		} else {
			out = append(out, iv)
		}
	}
	return out
}
func partition(in, out int64, locks [][2]int64) []piece {
	pieces := []piece{}
	cursor := in
	for _, iv := range locks {
		if iv[0] > cursor {
			pieces = append(pieces, piece{in: cursor, out: iv[0]})
		}
		pieces = append(pieces, piece{in: iv[0], out: iv[1], locked: true})
		cursor = iv[1]
	}
	if cursor < out {
		pieces = append(pieces, piece{in: cursor, out: out})
	}
	return pieces
}

// mergeSlivers folds pieces shorter than minPieceMS into neighbours so
// the encoder is not asked for sub-frame fragments. Unlocked slivers
// prefer a locked neighbour (slower is safer); locked slivers may only
// merge into locked neighbours so protected dialogue never speeds up.
func mergeSlivers(pieces []piece) []piece {
	for len(pieces) > 1 {
		pick, at := -1, -1
		for i, pc := range pieces {
			if pc.out-pc.in >= minPieceMS {
				continue
			}
			left, right := i-1, i+1
			leftOK := left >= 0 && (!pc.locked || pieces[left].locked)
			rightOK := right < len(pieces) && (!pc.locked || pieces[right].locked)
			choose := -1
			if !pc.locked {
				leftLocked := left >= 0 && pieces[left].locked
				rightLocked := right < len(pieces) && pieces[right].locked
				switch {
				case leftLocked && rightLocked:
					choose = longer(pieces, left, right)
				case leftLocked:
					choose = left
				case rightLocked:
					choose = right
				}
			}
			if choose < 0 {
				switch {
				case leftOK && rightOK:
					choose = longer(pieces, left, right)
				case leftOK:
					choose = left
				case rightOK:
					choose = right
				}
			}
			if choose >= 0 {
				pick, at = choose, i
				break
			}
		}
		if pick < 0 {
			return pieces
		}
		if at < pick {
			pieces[pick].in = pieces[at].in
		} else {
			pieces[pick].out = pieces[at].out
		}
		pieces[pick].locked = pieces[pick].locked || pieces[at].locked
		pieces = append(pieces[:at], pieces[at+1:]...)
	}
	return pieces
}
func longer(pieces []piece, a, b int) int {
	if pieces[b].out-pieces[b].in > pieces[a].out-pieces[a].in {
		return b
	}
	return a
}

// solveRates derives the shared locked/unlocked rates for a strategy.
// fit keeps dialogue at natural pace while the remainder absorbs the
// compression, and only raises dialogue within the intelligible cap.
func solveRates(locked, unlocked, target int64, dialogueCap, hardCap float64, strategy string) (float64, float64, bool) {
	T, L, U := float64(target), float64(locked), float64(unlocked)
	switch strategy {
	case strategyFit:
		if U > 0 && T > L {
			if r := U / (T - L); r <= hardCap {
				return 1, r, true
			}
		}
		if remaining := T - U/hardCap; remaining > 0 {
			if r := L / remaining; r <= dialogueCap {
				return r, hardCap, true
			}
		}
		return 0, 0, false
	case "pace-first":
		return dialogueCap, hardCap, true
	case "deadline-first":
		if remaining := T - U/hardCap; remaining > 0 {
			if r := L / remaining; r <= hardCap {
				return r, hardCap, true
			}
		}
		return 0, 0, false
	}
	return 0, 0, false
}
func assignRates(pieces [][]piece, dialogueRate, otherRate float64) {
	for _, span := range pieces {
		for i := range span {
			if span[i].locked {
				span[i].rate = dialogueRate
			} else {
				span[i].rate = otherRate
			}
		}
	}
}

// candidates produces the reviewable choices for an unresolvable fit.
// Neither choice is a global speedup and neither drops captions: both
// keep the same partition and only move rates within the hard cap.
func candidates(pieces [][]piece, locked, unlocked, target int64, dialogueCap, hardCap float64) []domain.Candidate {
	out := []domain.Candidate{}
	withRates := func(dialogueRate, otherRate float64) int64 {
		clone := make([][]piece, len(pieces))
		for i, span := range pieces {
			clone[i] = append([]piece(nil), span...)
		}
		assignRates(clone, dialogueRate, otherRate)
		_, total := planSegments(clone)
		return total
	}
	pace := withRates(dialogueCap, hardCap)
	out = append(out, domain.Candidate{
		ID:           "pace-first",
		Summary:      fmt.Sprintf("对白保持可理解节奏（不超过%.2fx），其余内容不超过%.2fx；成片约%dms，超出交付目标%dms", dialogueCap, hardCap, pace, pace-target),
		DurationMS:   pace,
		DialogueRate: dialogueCap,
		OtherRate:    hardCap,
		TargetMet:    pace <= target,
	})
	if remaining := float64(target) - float64(unlocked)/hardCap; remaining > 0 {
		if r := float64(locked) / remaining; r <= hardCap {
			deadline := withRates(r, hardCap)
			out = append(out, domain.Candidate{
				ID:           "deadline-first",
				Summary:      fmt.Sprintf("压进%dms交付目标；对白节奏升至%.2fx，高于%.2fx可理解上限", target, r, dialogueCap),
				DurationMS:   deadline,
				DialogueRate: r,
				OtherRate:    hardCap,
				TargetMet:    deadline <= target,
			})
		}
	}
	return out
}

// planSegments flattens pieces into the confirmed segment map with
// cumulative rounding. Every segment keeps at least 1ms of output so
// downstream filters never see an empty interval; the same function
// backs plan building and candidate estimates so they always agree.
func planSegments(pieces [][]piece) ([][]domain.TempoSegment, int64) {
	exact := 0.0
	var cursor int64
	out := make([][]domain.TempoSegment, 0, len(pieces))
	for _, span := range pieces {
		segments := []domain.TempoSegment{}
		for _, pc := range span {
			exact += float64(pc.out-pc.in) / pc.rate
			end := int64(math.Round(exact))
			if end <= cursor {
				end = cursor + 1
			}
			segments = append(segments, domain.TempoSegment{SourceInMS: pc.in, SourceOutMS: pc.out, OutputInMS: cursor, OutputOutMS: end, Rate: pc.rate})
			cursor = end
		}
		out = append(out, segments)
	}
	return out, cursor
}
