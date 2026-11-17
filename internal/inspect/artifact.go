package inspect

import (
	"cadenceforge/internal/domain"
	"cadenceforge/internal/media"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"math"
	"os"
)

func Digest(path string) (string, int64, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", 0, err
	}
	defer f.Close()
	h := sha256.New()
	n, err := io.Copy(h, f)
	if err != nil {
		return "", n, err
	}
	return hex.EncodeToString(h.Sum(nil)), n, nil
}
func Artifact(ctx context.Context, prober media.Prober, path string, r domain.Render) (domain.Inspection, error) {
	m, err := prober.Probe(ctx, path)
	if err != nil {
		return domain.Inspection{}, err
	}
	delta := m.DurationMS - r.Plan.DurationMS
	// Every tempo segment quantizes to whole frames (or atempo windows),
	// so the honest duration bound scales with the confirmed map's
	// segment count rather than a single flat constant.
	segments := 0
	for _, p := range r.Plan.Placements {
		segments += len(p.Tempo())
	}
	tolerance := int64(100)
	if r.Recipe.Format == "mp4" {
		frame := int64(math.Ceil(1000 / float64(r.Recipe.FPS)))
		tolerance = max(tolerance, int64(2000/r.Recipe.FPS)) + int64(segments)*frame
	} else {
		tolerance += int64(segments) * 25
	}
	if math.Abs(float64(delta)) > float64(tolerance) {
		return domain.Inspection{}, fmt.Errorf("output duration delta %dms exceeds %dms", delta, tolerance)
	}
	if !m.Audio || m.SampleRate != r.Recipe.SampleRate {
		return domain.Inspection{}, fmt.Errorf("output audio contract mismatch")
	}
	if r.Recipe.Format == "mp4" && (!m.Video || m.Width != r.Recipe.Width || m.Height != r.Recipe.Height) {
		return domain.Inspection{}, fmt.Errorf("output video contract mismatch")
	}
	digest, n, err := Digest(path)
	if err != nil {
		return domain.Inspection{}, err
	}
	return domain.Inspection{Media: m, Digest: digest, Bytes: n, DurationDeltaMS: delta}, nil
}
