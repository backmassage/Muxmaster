package tune

import (
	"math"
	"testing"

	"github.com/backmassage/muxmaster/internal/config"
)

func TestParseMetadataFloats(t *testing.T) {
	// Representative ffmpeg `metadata=mode=print` output: a frame header line,
	// the target key, an unrelated key, a malformed value, and whitespace.
	out := "frame:0    pts:0       pts_time:0\n" +
		"lavfi.bitplanenoise.0.1=0.984000\n" +
		"lavfi.signalstats.YAVG=128.0\n" +
		"frame:1    pts:512     pts_time:0.5\n" +
		"  lavfi.bitplanenoise.0.1=0.512  \n" +
		"lavfi.bitplanenoise.0.1=not_a_number\n"
	got := parseMetadataFloats(out, "lavfi.bitplanenoise.0.1")
	want := []float64{0.984, 0.512}
	if len(got) != len(want) {
		t.Fatalf("got %d values %v, want %d %v", len(got), got, len(want), want)
	}
	for i := range want {
		if math.Abs(got[i]-want[i]) > 1e-9 {
			t.Errorf("value %d: got %g, want %g", i, got[i], want[i])
		}
	}
}

func TestParseMetadataFloats_Empty(t *testing.T) {
	if got := parseMetadataFloats("", "k"); got != nil {
		t.Errorf("empty input: got %v, want nil", got)
	}
	if got := parseMetadataFloats("k=1\n", "other"); got != nil {
		t.Errorf("no match: got %v, want nil", got)
	}
}

func TestMean(t *testing.T) {
	cases := []struct {
		in   []float64
		want float64
	}{
		{nil, 0},
		{[]float64{}, 0},
		{[]float64{2}, 2},
		{[]float64{1, 2, 3, 4}, 2.5},
	}
	for _, c := range cases {
		if got := mean(c.in); math.Abs(got-c.want) > 1e-9 {
			t.Errorf("mean(%v) = %g, want %g", c.in, got, c.want)
		}
	}
}

func TestSeekSeconds(t *testing.T) {
	cases := []struct {
		dur  float64
		want int
	}{
		{0, 0},        // unknown
		{60, 0},       // short → no seek
		{120, 0},      // at the boundary → no seek
		{1200, 240},   // 20%
		{100000, 600}, // clamped to maxSeekSeconds
	}
	for _, c := range cases {
		if got := seekSeconds(c.dur); got != c.want {
			t.Errorf("seekSeconds(%g) = %d, want %d", c.dur, got, c.want)
		}
	}
}

func TestSuggest(t *testing.T) {
	cases := []struct {
		name     string
		sig      ContentSignal
		wantTune config.TuneMode
		wantConf Confidence
	}{
		{"no sample", ContentSignal{Grain: 0, Frames: 0}, config.TuneNone, ConfidenceLow},
		{"clean", ContentSignal{Grain: 0.09, Frames: 48}, config.TuneNone, ConfidenceHigh},
		{"just-under-light", ContentSignal{Grain: 0.19, Frames: 48}, config.TuneNone, ConfidenceHigh},
		{"light boundary", ContentSignal{Grain: 0.20, Frames: 48}, config.TuneFilm, ConfidenceHigh},
		{"light grain", ContentSignal{Grain: 0.40, Frames: 48}, config.TuneFilm, ConfidenceHigh},
		{"just-under-heavy", ContentSignal{Grain: 0.54, Frames: 48}, config.TuneFilm, ConfidenceHigh},
		{"heavy boundary", ContentSignal{Grain: 0.55, Frames: 48}, config.TuneGrain, ConfidenceHigh},
		{"heavy grain", ContentSignal{Grain: 0.99, Frames: 48}, config.TuneGrain, ConfidenceHigh},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := Suggest(c.sig)
			if got.Tune != c.wantTune {
				t.Errorf("Tune: got %s, want %s", got.Tune, c.wantTune)
			}
			if got.Confidence != c.wantConf {
				t.Errorf("Confidence: got %s, want %s", got.Confidence, c.wantConf)
			}
			if got.Reason == "" {
				t.Error("Reason should not be empty")
			}
		})
	}
}
