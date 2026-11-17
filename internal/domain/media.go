package domain

import "time"

type Media struct {
	DurationMS int64  `json:"duration_ms"`
	Video      bool   `json:"video"`
	Audio      bool   `json:"audio"`
	Width      int    `json:"width"`
	Height     int    `json:"height"`
	SampleRate int    `json:"sample_rate"`
	Channels   int    `json:"channels"`
	VideoCodec string `json:"video_codec"`
	AudioCodec string `json:"audio_codec"`
}
type Source struct {
	ID        string    `json:"id"`
	Name      string    `json:"name"`
	Digest    string    `json:"digest"`
	Bytes     int64     `json:"bytes"`
	Media     Media     `json:"media"`
	CreatedAt time.Time `json:"created_at"`
}

// TempoZone is one explicit constant-rate piece of a span, in source
// milliseconds. Zones of a span must exactly partition [InMS, OutMS).
type TempoZone struct {
	InMS  int64   `json:"in_ms"`
	OutMS int64   `json:"out_ms"`
	Rate  float64 `json:"rate"`
}
type Span struct {
	SourceID string      `json:"source_id"`
	InMS     int64       `json:"in_ms"`
	OutMS    int64       `json:"out_ms"`
	GainDB   float64     `json:"gain_db"`
	Tempo    []TempoZone `json:"tempo,omitempty"`
}
type Cue struct {
	SourceID string `json:"source_id"`
	StartMS  int64  `json:"start_ms"`
	EndMS    int64  `json:"end_ms"`
	Text     string `json:"text"`
	// PaceLock marks designated dialogue whose intelligible pace the
	// delivery fit must preserve; only meaningful with Sheet.Delivery.
	PaceLock bool `json:"pace_lock,omitempty"`
}

// Delivery declares a fixed delivery-duration intent. TargetMS is a
// maximum: shorter natural compositions are never stretched.
type Delivery struct {
	TargetMS        int64   `json:"target_ms"`
	MaxDialogueRate float64 `json:"max_dialogue_rate,omitempty"`
	MaxRate         float64 `json:"max_rate,omitempty"`
}
type Sheet struct {
	ID       string    `json:"id"`
	Revision int       `json:"revision"`
	Name     string    `json:"name"`
	Spans    []Span    `json:"spans"`
	Cues     []Cue     `json:"cues"`
	Delivery *Delivery `json:"delivery,omitempty"`
	// Resolution is service-managed: it records the fit choice the user
	// confirmed via the resolve endpoint. Sheet saves always clear it.
	Resolution string    `json:"resolution,omitempty"`
	CreatedAt  time.Time `json:"created_at"`
}
type Recipe struct {
	ID         string  `json:"id"`
	Revision   int     `json:"revision"`
	Name       string  `json:"name"`
	Format     string  `json:"format"`
	Width      int     `json:"width"`
	Height     int     `json:"height"`
	FPS        int     `json:"fps"`
	SampleRate int     `json:"sample_rate"`
	Normalize  bool    `json:"normalize"`
	TargetLUFS float64 `json:"target_lufs"`
}

// TempoSegment is one resolved constant-rate piece of a placement. The
// ordered segments of all placements are the single confirmed
// source-to-output time relationship shared by picture and captions.
type TempoSegment struct {
	SourceInMS  int64   `json:"source_in_ms"`
	SourceOutMS int64   `json:"source_out_ms"`
	OutputInMS  int64   `json:"output_in_ms"`
	OutputOutMS int64   `json:"output_out_ms"`
	Rate        float64 `json:"rate"`
}
type Placement struct {
	SourceID    string         `json:"source_id"`
	SourceInMS  int64          `json:"source_in_ms"`
	SourceOutMS int64          `json:"source_out_ms"`
	OutputInMS  int64          `json:"output_in_ms"`
	OutputOutMS int64          `json:"output_out_ms"`
	GainDB      float64        `json:"gain_db"`
	Segments    []TempoSegment `json:"segments,omitempty"`
}

// Tempo returns the placement's confirmed piecewise map. Plans frozen
// before tempo editing carry no segments and mean identity rate.
func (p Placement) Tempo() []TempoSegment {
	if len(p.Segments) > 0 {
		return p.Segments
	}
	return []TempoSegment{{SourceInMS: p.SourceInMS, SourceOutMS: p.SourceOutMS, OutputInMS: p.OutputInMS, OutputOutMS: p.OutputOutMS, Rate: 1}}
}

// TempoInfo records how the plan's tempo map was resolved. It is frozen
// with the plan so a render's time semantics stay auditable.
type TempoInfo struct {
	Strategy     string  `json:"strategy"` // natural | explicit | fit | pace-first | deadline-first
	TargetMS     int64   `json:"target_ms,omitempty"`
	NaturalMS    int64   `json:"natural_ms,omitempty"`
	DialogueRate float64 `json:"dialogue_rate,omitempty"`
	OtherRate    float64 `json:"other_rate,omitempty"`
}
type Plan struct {
	DurationMS int64       `json:"duration_ms"`
	Placements []Placement `json:"placements"`
	Recipe     Recipe      `json:"recipe"`
	VTT        string      `json:"vtt"`
	Tempo      *TempoInfo  `json:"tempo,omitempty"`
}

// Candidate is one reviewable resolution of a delivery-fit conflict.
// Confirming it stores its ID as the sheet revision's Resolution.
type Candidate struct {
	ID           string  `json:"id"` // pace-first | deadline-first
	Summary      string  `json:"summary"`
	DurationMS   int64   `json:"duration_ms"`
	DialogueRate float64 `json:"dialogue_rate"`
	OtherRate    float64 `json:"other_rate"`
	TargetMet    bool    `json:"target_met"`
}

// FitReport describes how a sheet revision's delivery intent resolves.
type FitReport struct {
	SheetID    string      `json:"sheet_id"`
	Revision   int         `json:"revision"`
	Status     string      `json:"status"` // resolved | confirmation_required
	Strategy   string      `json:"strategy"`
	NaturalMS  int64       `json:"natural_ms"`
	TargetMS   int64       `json:"target_ms,omitempty"`
	LockedMS   int64       `json:"locked_ms"`
	UnlockedMS int64       `json:"unlocked_ms"`
	Candidates []Candidate `json:"candidates,omitempty"`
}
