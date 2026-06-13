// Package tune runs a cheap ffmpeg pre-pass to estimate source grain/noise so
// the pipeline can suggest a content prefilter (--tune) per series. Grain is
// the one axis deep research found reliably auto-detectable; film-vs-anime is
// left to a per-series user prompt (see internal/pipeline/autotune.go).
//
// Dependency note: this package shells out to ffmpeg directly (like
// internal/check), mirroring that diagnostic exception to the
// "all ffmpeg via internal/ffmpeg" rule — it builds no encode command.
package tune

import (
	"bytes"
	"context"
	"os/exec"
	"strconv"
	"strings"
)

// ContentSignal holds the measured per-source analysis metrics.
type ContentSignal struct {
	// Grain is the mean LSB bit-plane noise over the sampled frames, in [0,1].
	// ~0 = flat/clean, ~0.04 = smooth gradient, high = film grain / sensor
	// noise. bitplanenoise on bit-plane 1 is edge-robust (edges live in higher
	// bit-planes), unlike signalstats TOUT which false-positives on detail.
	Grain float64
	// Frames is how many sampled frames contributed; 0 means detection failed
	// (caller should treat the result as "unknown" → no prefilter).
	Frames int
}

// Sampling and analysis parameters. The window is seeked past the intro so
// logos/black frames don't skew the estimate; 2 fps over ~24 s at 480px wide
// is a few dozen downscaled frames — sub-second versus a multi-minute encode.
const (
	seekFraction     = 0.20 // Start the window ~20% into the title.
	minSeekDuration  = 120  // Only seek when the title is longer than this (s).
	maxSeekSeconds   = 600  // Never seek past 10 min (clamps absurd durations).
	analysisWindow   = 24   // Seconds of footage to analyze.
	analysisBitplane = "1"  // LSB bit-plane carries the most grain/noise.

	// analysisFilter downsamples in time (fps) and space (scale), then prints
	// per-frame bit-plane-1 noise as `lavfi.bitplanenoise.0.1=<v>` to stdout.
	analysisFilter = "fps=2,scale=480:-2,format=yuv420p," +
		"bitplanenoise=bitplane=" + analysisBitplane + ",metadata=mode=print:file=-"

	bitplaneKey = "lavfi.bitplanenoise.0.1"
)

// DetectContent runs the grain pre-pass on path and returns the averaged
// signal. durationSec (from a prior probe) positions the sample window; pass 0
// if unknown (analysis starts at the beginning). A non-nil error means ffmpeg
// failed to run; a successful run with Frames==0 means no metadata was parsed.
func DetectContent(ctx context.Context, path string, durationSec float64) (ContentSignal, error) {
	args := []string{"-hide_banner", "-nostdin", "-loglevel", "error"}
	if seek := seekSeconds(durationSec); seek > 0 {
		args = append(args, "-ss", strconv.Itoa(seek))
	}
	args = append(args,
		"-t", strconv.Itoa(analysisWindow),
		"-i", path,
		"-an",
		"-vf", analysisFilter,
		"-f", "null", "-",
	)

	cmd := exec.CommandContext(ctx, "ffmpeg", args...)
	var out bytes.Buffer
	cmd.Stdout = &out // metadata=print:file=- writes here; null muxer writes nothing.
	if err := cmd.Run(); err != nil {
		return ContentSignal{}, err
	}

	grains := parseMetadataFloats(out.String(), bitplaneKey)
	return ContentSignal{Grain: mean(grains), Frames: len(grains)}, nil
}

// seekSeconds returns the seek offset for the sample window, or 0 to start at
// the beginning (short or unknown-duration titles).
func seekSeconds(durationSec float64) int {
	if durationSec <= minSeekDuration {
		return 0
	}
	seek := int(durationSec * seekFraction)
	if seek > maxSeekSeconds {
		seek = maxSeekSeconds
	}
	return seek
}

// parseMetadataFloats extracts every `<key>=<float>` value from ffmpeg's
// metadata=print output (one key per line). Split out for table-driven tests.
func parseMetadataFloats(out, key string) []float64 {
	var vals []float64
	prefix := key + "="
	for _, line := range strings.Split(out, "\n") {
		line = strings.TrimSpace(line)
		if !strings.HasPrefix(line, prefix) {
			continue
		}
		if v, err := strconv.ParseFloat(strings.TrimSpace(line[len(prefix):]), 64); err == nil {
			vals = append(vals, v)
		}
	}
	return vals
}

// mean returns the arithmetic mean, or 0 for an empty slice.
func mean(vals []float64) float64 {
	if len(vals) == 0 {
		return 0
	}
	var sum float64
	for _, v := range vals {
		sum += v
	}
	return sum / float64(len(vals))
}
