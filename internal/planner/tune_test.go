// tune_test.go covers --tune prefilters, sw-decode forcing, QP bias, and --quality-priority.
package planner

import (
	"strings"
	"testing"

	"github.com/backmassage/muxmaster/internal/config"
)

func TestTunePrefilter(t *testing.T) {
	cases := []struct {
		tune config.TuneMode
		want string
	}{
		{config.TuneNone, ""},
		{config.TuneFilm, "hqdn3d=1.5:1.5:6:6"},
		{config.TuneGrain, "hqdn3d=4:4:9:9"},
		{config.TuneAnime, "gradfun=1.2:16"},
	}
	for _, c := range cases {
		cfg := defaultCfg()
		cfg.Encoder.Tune = c.tune
		if got := TunePrefilter(cfg); got != c.want {
			t.Errorf("tune=%s: got %q, want %q", c.tune, got, c.want)
		}
	}
}

// TestBuildPlan_TuneMatrix exercises tune × mode, asserting prefilter presence,
// sw-decode forcing for VAAPI, and that none preserves the GPU path.
func TestBuildPlan_TuneMatrix(t *testing.T) {
	prefilters := map[config.TuneMode]string{
		config.TuneNone:  "",
		config.TuneFilm:  "hqdn3d=1.5:1.5:6:6",
		config.TuneGrain: "hqdn3d=4:4:9:9",
		config.TuneAnime: "gradfun=1.2:16",
	}
	for _, mode := range []config.EncoderMode{config.EncoderVAAPI, config.EncoderCPU} {
		for tune, pre := range prefilters {
			cfg := defaultCfg()
			cfg.Encoder.Mode = mode
			cfg.Encoder.Tune = tune
			// CheckDeps would set these in VAAPI mode; seed them for the test.
			cfg.Encoder.VaapiProfile = "main10"
			cfg.Encoder.VaapiSwFormat = "p010"

			plan := BuildPlan(cfg, h264SDR())

			if pre != "" && !strings.Contains(plan.VideoFilters, pre) {
				t.Errorf("mode=%s tune=%s: VideoFilters %q missing prefilter %q",
					mode, tune, plan.VideoFilters, pre)
			}
			if pre == "" && (strings.Contains(plan.VideoFilters, "hqdn3d") ||
				strings.Contains(plan.VideoFilters, "gradfun")) {
				t.Errorf("mode=%s tune=none: VideoFilters %q should carry no prefilter", mode, plan.VideoFilters)
			}

			if mode == config.EncoderVAAPI {
				if pre != "" {
					if plan.HWDecode {
						t.Errorf("VAAPI tune=%s: HWDecode must be forced false when a prefilter is active", tune)
					}
					if strings.Contains(plan.VideoFilters, "scale_vaapi") {
						t.Errorf("VAAPI tune=%s: sw-decode chain must not use scale_vaapi, got %q", tune, plan.VideoFilters)
					}
					if !strings.Contains(plan.VideoFilters, "hwupload") {
						t.Errorf("VAAPI tune=%s: sw-decode chain must end in hwupload, got %q", tune, plan.VideoFilters)
					}
					if !hasNote(plan.Notes, "GPU decode disabled") {
						t.Errorf("VAAPI tune=%s: expected a GPU-decode-disabled note, got %v", tune, plan.Notes)
					}
				} else if !plan.HWDecode {
					t.Errorf("VAAPI tune=none: HWDecode should remain enabled for an 8-bit SDR source")
				}
			}
		}
	}
}

// TestBuildPlan_TunePrefilterOrder verifies the prefilter sits after deinterlace
// and before hwupload in the VAAPI software-decode chain.
func TestBuildPlan_TunePrefilterOrder(t *testing.T) {
	cfg := defaultCfg()
	cfg.Encoder.Mode = config.EncoderVAAPI
	cfg.Encoder.Tune = config.TuneFilm
	cfg.Encoder.VaapiProfile = "main10"
	cfg.Encoder.VaapiSwFormat = "p010"

	plan := BuildPlan(cfg, interlacedFile())

	yadif := strings.Index(plan.VideoFilters, "yadif")
	denoise := strings.Index(plan.VideoFilters, "hqdn3d")
	hwupload := strings.Index(plan.VideoFilters, "hwupload")
	if yadif < 0 || denoise < 0 || hwupload < 0 {
		t.Fatalf("expected yadif, hqdn3d, and hwupload in %q", plan.VideoFilters)
	}
	if !(yadif < denoise && denoise < hwupload) {
		t.Errorf("filter order wrong (want yadif<hqdn3d<hwupload): %q", plan.VideoFilters)
	}
}

// TestSmartQuality_TuneQPBias covers Change 2 (anime +1 → 0) and Change 4
// (film/grain −1 only when DenoiseQPBias is set; off by default).
func TestSmartQuality_TuneQPBias(t *testing.T) {
	pr := h264SDR()

	base := defaultCfg()
	base.Encoder.Mode = config.EncoderVAAPI
	none := SmartQuality(base, pr)

	// Change 2: anime now carries no bias — quality-first keeps cels at the
	// curve QP rather than biasing them toward smaller files.
	anime := defaultCfg()
	anime.Encoder.Mode = config.EncoderVAAPI
	anime.Encoder.Tune = config.TuneAnime
	withAnime := SmartQuality(anime, pr)
	if withAnime.VaapiQP != none.VaapiQP {
		t.Errorf("anime VaapiQP: got %d, want %d (Change 2: anime bias is 0)", withAnime.VaapiQP, none.VaapiQP)
	}
	if withAnime.CpuCRF != none.CpuCRF {
		t.Errorf("anime CpuCRF: got %d, want %d (Change 2: anime bias is 0)", withAnime.CpuCRF, none.CpuCRF)
	}

	// film/grain stay neutral by default (Change 4 gated off).
	for _, tune := range []config.TuneMode{config.TuneFilm, config.TuneGrain} {
		cfg := defaultCfg()
		cfg.Encoder.Mode = config.EncoderVAAPI
		cfg.Encoder.Tune = tune
		if got := SmartQuality(cfg, pr); got.VaapiQP != none.VaapiQP {
			t.Errorf("tune=%s VaapiQP: got %d, want %d (no bias by default)", tune, got.VaapiQP, none.VaapiQP)
		}
	}

	// Change 4: with DenoiseQPBias enabled, film/grain bias QP/CRF down by 1
	// (bounded below by the Change 5 invariant, not tested here).
	for _, tune := range []config.TuneMode{config.TuneFilm, config.TuneGrain} {
		cfg := defaultCfg()
		cfg.Encoder.Mode = config.EncoderVAAPI
		cfg.Encoder.Tune = tune
		cfg.Encoder.DenoiseQPBias = true
		got := SmartQuality(cfg, pr)
		wantQP := Clamp(none.VaapiQP-1, VaapiQPMin, VaapiQPMax)
		if got.VaapiQP != wantQP {
			t.Errorf("tune=%s DenoiseQPBias VaapiQP: got %d, want %d (−1)", tune, got.VaapiQP, wantQP)
		}
		wantCRF := Clamp(none.CpuCRF-1, CpuCRFMin, CpuCRFMax)
		if got.CpuCRF != wantCRF {
			t.Errorf("tune=%s DenoiseQPBias CpuCRF: got %d, want %d (−1)", tune, got.CpuCRF, wantCRF)
		}
	}
}

// TestBuildPlan_QualityPriority asserts the optimal-bitrate upward push is
// skipped while the preflight safety net stays intact.
func TestBuildPlan_QualityPriority(t *testing.T) {
	// A high-density source: the optimal-bitrate model pushes QP up by default.
	pr := h264SDR()
	pr.PrimaryVideo.BitRate = 25000000
	pr.Format.BitRate = 26000000

	cfg := defaultCfg()
	cfg.Encoder.Mode = config.EncoderVAAPI
	def := BuildPlan(cfg, pr)

	qpCfg := defaultCfg()
	qpCfg.Encoder.Mode = config.EncoderVAAPI
	qpCfg.Encoder.QualityPriority = true
	qp := BuildPlan(qpCfg, pr)

	// With the push skipped, the emitted QP is exactly SmartQuality run through
	// the preflight safety net — no optimal-bitrate override.
	sq := SmartQuality(qpCfg, pr)
	wantQP, _, _ := PreflightAdjust(qpCfg, pr, sq.VaapiQP, sq.CpuCRF, 105)
	if qp.VaapiQP != wantQP {
		t.Errorf("quality-priority VaapiQP: got %d, want %d (SmartQuality+preflight, no push)", qp.VaapiQP, wantQP)
	}
	if qp.VaapiQP > def.VaapiQP {
		t.Errorf("quality-priority QP %d should be ≤ default QP %d (push only raises QP)", qp.VaapiQP, def.VaapiQP)
	}
	if def.VaapiQP <= qp.VaapiQP {
		t.Errorf("expected the default run to push QP above quality-priority (got default=%d priority=%d)",
			def.VaapiQP, qp.VaapiQP)
	}
	if !hasNote(qp.Notes, "quality-priority") {
		t.Errorf("expected a quality-priority note, got %v", qp.Notes)
	}
}

func hasNote(notes []string, substr string) bool {
	for _, n := range notes {
		if strings.Contains(n, substr) {
			return true
		}
	}
	return false
}
