package caption

import (
	"cadenceforge/internal/domain"
	"fmt"
	"html"
	"math"
	"sort"
	"strings"
)

type Interval struct {
	StartMS, EndMS int64
	Text           string
	Order          int
}

// Map projects source cues into output time through the plan's confirmed
// tempo segments — the same relationship the picture is encoded with.
// Every intersecting cue stays visible: intervals are never dropped,
// only re-timed, with a 1ms minimum so speed-up cannot erase a caption.
func Map(sheet domain.Sheet, plan domain.Plan) []Interval {
	out := []Interval{}
	for _, p := range plan.Placements {
		segments := p.Tempo()
		for i, c := range sheet.Cues {
			if c.SourceID != p.SourceID {
				continue
			}
			start := max(c.StartMS, p.SourceInMS)
			end := min(c.EndMS, p.SourceOutMS)
			if end <= start {
				continue
			}
			mapped := Interval{StartMS: point(segments, start), EndMS: point(segments, end), Text: c.Text, Order: i}
			if mapped.EndMS <= mapped.StartMS {
				mapped.EndMS = mapped.StartMS + 1
				if mapped.EndMS > plan.DurationMS {
					mapped.EndMS = plan.DurationMS
					mapped.StartMS = max(0, plan.DurationMS-1)
				}
			}
			out = append(out, mapped)
		}
	}
	sort.SliceStable(out, func(i, j int) bool {
		if out[i].StartMS != out[j].StartMS {
			return out[i].StartMS < out[j].StartMS
		}
		return out[i].Order < out[j].Order
	})
	return out
}

// point converts a source millisecond into output time using the
// placement's piecewise map. Segment boundaries map exactly so cues
// aligned to them stay contiguous.
func point(segments []domain.TempoSegment, ms int64) int64 {
	for i, s := range segments {
		if ms < s.SourceOutMS || i == len(segments)-1 {
			if ms <= s.SourceInMS {
				return s.OutputInMS
			}
			if ms >= s.SourceOutMS {
				return s.OutputOutMS
			}
			return s.OutputInMS + int64(math.Round(float64(ms-s.SourceInMS)/s.Rate))
		}
	}
	return segments[len(segments)-1].OutputOutMS
}
func stamp(ms int64) string {
	return fmt.Sprintf("%02d:%02d:%02d.%03d", ms/3600000, (ms/60000)%60, (ms/1000)%60, ms%1000)
}
func VTT(intervals []Interval) string {
	var b strings.Builder
	b.WriteString("WEBVTT\n\n")
	for i, c := range intervals {
		fmt.Fprintf(&b, "%d\n%s --> %s\n%s\n\n", i+1, stamp(c.StartMS), stamp(c.EndMS), strings.ReplaceAll(html.EscapeString(c.Text), "\n\n", "\n"))
	}
	return b.String()
}
