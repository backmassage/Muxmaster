package tune

import (
	"fmt"

	"github.com/backmassage/muxmaster/internal/config"
)

// Grain thresholds mapping mean LSB bit-plane noise to a denoise profile.
//
// INITIAL ESTIMATES — deep research gives the method but no calibrated cut
// points for real Blu-ray HEVC sources (an open question in the plan). Local
// measurement anchors them: flat 0.00, smooth gradient 0.04, heavy synthetic
// grain ~0.95; real correlated film grain lands between. Re-sweep on real
// titles before treating these as final.
const (
	grainLightThreshold = 0.20 // ≥ this → light denoise (film).
	grainHeavyThreshold = 0.55 // ≥ this → heavy denoise (grain).
)

// Confidence qualifies how much weight to give a suggestion in the prompt.
type Confidence string

const (
	ConfidenceHigh Confidence = "high"
	ConfidenceLow  Confidence = "low" // Detection failed → safe default, user should decide.
)

// Suggestion is the recommended prefilter for a source plus the rationale
// surfaced in the per-series prompt. Anime is never auto-suggested — research
// found no reliable animation-vs-live-action detector — so the user selects it
// in the prompt when they know the content is animation.
type Suggestion struct {
	Tune       config.TuneMode
	Confidence Confidence
	Reason     string
}

// Suggest maps a measured ContentSignal to a denoise profile. With no usable
// sample (Frames==0) it returns a low-confidence `none` so the run stays safe
// and the user makes the call.
func Suggest(sig ContentSignal) Suggestion {
	if sig.Frames == 0 {
		return Suggestion{
			Tune:       config.TuneNone,
			Confidence: ConfidenceLow,
			Reason:     "no sample analyzed",
		}
	}
	switch {
	case sig.Grain >= grainHeavyThreshold:
		return Suggestion{config.TuneGrain, ConfidenceHigh, grainReason("heavy grain/noise", sig.Grain)}
	case sig.Grain >= grainLightThreshold:
		return Suggestion{config.TuneFilm, ConfidenceHigh, grainReason("light grain", sig.Grain)}
	default:
		return Suggestion{config.TuneNone, ConfidenceHigh, grainReason("clean source", sig.Grain)}
	}
}

func grainReason(label string, grain float64) string {
	return fmt.Sprintf("%s (grain=%.2f)", label, grain)
}
