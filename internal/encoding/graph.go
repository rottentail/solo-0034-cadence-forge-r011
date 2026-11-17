package encoding

import (
	"cadenceforge/internal/domain"
	"cadenceforge/internal/media"
	"fmt"
	"strconv"
	"strings"
)

func seconds(ms int64) string  { return fmt.Sprintf("%.3f", float64(ms)/1000) }
func rate(rate float64) string { return strconv.FormatFloat(rate, 'f', 6, 64) }

// atempo expresses a playback rate with FFmpeg's tempo filter, which
// keeps pitch so sped-up dialogue stays intelligible. Rates below 0.5
// are chained because one atempo instance accepts 0.5..100.
func atempo(r float64) string {
	if r == 1 {
		return ""
	}
	if r < 0.5 {
		return ",atempo=0.5,atempo=" + rate(r/0.5)
	}
	return ",atempo=" + rate(r)
}

// Arguments builds the filter graph from the plan's confirmed tempo
// segments, so the encoded picture follows exactly the time relationship
// the captions were mapped with.
func Arguments(render domain.Render, paths map[string]string, output string) ([]string, error) {
	r := render.Recipe
	args := []string{"-nostdin", "-y", "-hide_banner", "-loglevel", "error", "-filter_complex_threads", "1"}
	filters := []string{}
	joins := []string{}
	for i, p := range render.Plan.Placements {
		path, ok := paths[p.SourceID]
		if !ok {
			return nil, fmt.Errorf("source object missing")
		}
		args = append(args, "-protocol_whitelist", "file,pipe", "-format_whitelist", media.Formats, "-i", path)
		for j, seg := range p.Tempo() {
			label := fmt.Sprintf("%d_%d", i, j)
			in, out := seconds(seg.SourceInMS), seconds(seg.SourceOutMS)
			var audio string
			if render.Sources[p.SourceID].Media.Audio {
				audio = fmt.Sprintf("[%d:a:0]atrim=start=%s:end=%s,asetpts=PTS-STARTPTS%s,aresample=%d,aformat=sample_fmts=fltp:channel_layouts=stereo,volume=%.3fdB[a%s]", i, in, out, atempo(seg.Rate), r.SampleRate, p.GainDB, label)
			} else {
				audio = fmt.Sprintf("anullsrc=r=%d:cl=stereo,atrim=duration=%s,asetpts=PTS-STARTPTS[a%s]", r.SampleRate, seconds(seg.OutputOutMS-seg.OutputInMS), label)
			}
			filters = append(filters, audio)
			if r.Format == "mp4" {
				stretch := ",setpts=PTS-STARTPTS"
				if seg.Rate != 1 {
					stretch = fmt.Sprintf(",setpts=(PTS-STARTPTS)/%s", rate(seg.Rate))
				}
				filters = append(filters, fmt.Sprintf("[%d:v:0]trim=start=%s:end=%s%s,scale=%d:%d:force_original_aspect_ratio=decrease,pad=%d:%d:(ow-iw)/2:(oh-ih)/2,setsar=1,fps=%d,format=yuv420p[v%s]", i, in, out, stretch, r.Width, r.Height, r.Width, r.Height, r.FPS, label))
				joins = append(joins, fmt.Sprintf("[v%s][a%s]", label, label))
			} else {
				joins = append(joins, fmt.Sprintf("[a%s]", label))
			}
		}
	}
	if len(joins) == 0 {
		return nil, fmt.Errorf("empty render")
	}
	if r.Format == "mp4" {
		filters = append(filters, strings.Join(joins, "")+fmt.Sprintf("concat=n=%d:v=1:a=1[vout][acat]", len(joins)))
	} else {
		filters = append(filters, strings.Join(joins, "")+fmt.Sprintf("concat=n=%d:v=0:a=1[acat]", len(joins)))
	}
	if r.Normalize {
		filters = append(filters, fmt.Sprintf("[acat]loudnorm=I=%.2f:TP=-1.5:LRA=11,aresample=%d[aout]", r.TargetLUFS, r.SampleRate))
	} else {
		filters = append(filters, "[acat]anull[aout]")
	}
	args = append(args, "-filter_complex", strings.Join(filters, ";"))
	if r.Format == "mp4" {
		args = append(args, "-map", "[vout]", "-map", "[aout]", "-c:v", "libx264", "-preset", "veryfast", "-crf", "23", "-threads", "1", "-c:a", "aac", "-b:a", "160k", "-movflags", "+faststart")
	} else {
		args = append(args, "-map", "[aout]", "-c:a", "pcm_s16le")
	}
	return append(args, "-ar", fmt.Sprint(r.SampleRate), "-t", seconds(render.Plan.DurationMS), "-map_metadata", "-1", output), nil
}
