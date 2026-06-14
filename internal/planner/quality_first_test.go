// quality_first_test.go covers the hevc_vaapi quality-maximization plan:
// the Change 1 QP-ceiling push, the Change 5 stacked-bias invariant + per-vendor
// gating, and --size-priority restoring the legacy unbounded push.
package planner

import (
	"fmt"
	"testing"

	"github.com/backmassage/muxmaster/internal/config"
	"github.com/backmassage/muxmaster/internal/probe"
)

// TestBoundedPushQP_EdgeCases proves the Change 1 formula
// finalQP = max(baseQP, min(pushTargetQP, qpCeiling)) at the three edge cases
// the plan calls out: base below, at, and above the ceiling.
func TestBoundedPushQP_EdgeCases(t *testing.T) {
	const ceiling = 16
	cases := []struct {
		name               string
		base, target, want int
	}{
		// base < ceiling: push raises QP, but only to the ceiling (not base+3),
		// recovering quality while staying past the visual knee.
		{"base below ceiling", 14, 23, 16},
		// base ≈ ceiling: the push is effectively neutralized.
		{"base at ceiling", 16, 23, 16},
		// base > ceiling: max() wins, the push is fully disabled, base stands.
		{"base above ceiling", 18, 23, 18},
		// target between base and ceiling: the target is honored.
		{"target inside band", 14, 15, 15},
		// target below base (push wants a lower QP / bigger file): never lower QP.
		{"target below base", 18, 14, 18},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := boundedPushQP(c.base, c.target, ceiling); got != c.want {
				t.Errorf("boundedPushQP(%d, %d, %d) = %d, want %d",
					c.base, c.target, ceiling, got, c.want)
			}
		})
	}
}

// amdVAAPICfg returns a VAAPI config gated to the AMD/VCN quality-first profile.
func amdVAAPICfg() *config.Config {
	cfg := defaultCfg()
	cfg.Encoder.Mode = config.EncoderVAAPI
	cfg.Encoder.VaapiVendor = config.VaapiVendorAMD
	cfg.Encoder.VaapiProfile = "main10"
	cfg.Encoder.VaapiSwFormat = "p010"
	return cfg
}

// TestVaapiUseQualityFirstPush_VendorGating: the bounded push activates only on
// AMD and only when --size-priority is off.
func TestVaapiUseQualityFirstPush_VendorGating(t *testing.T) {
	cases := []struct {
		vendor       config.VaapiVendor
		sizePriority bool
		want         bool
	}{
		{config.VaapiVendorAMD, false, true},
		{config.VaapiVendorAMD, true, false},
		{config.VaapiVendorIntel, false, false},
		{config.VaapiVendorOther, false, false},
		{config.VaapiVendorUnknown, false, false},
	}
	for _, c := range cases {
		cfg := defaultCfg()
		cfg.Encoder.Mode = config.EncoderVAAPI
		cfg.Encoder.VaapiVendor = c.vendor
		cfg.Encoder.SizePriority = c.sizePriority
		if got := vaapiUseQualityFirstPush(cfg); got != c.want {
			t.Errorf("vendor=%s size-priority=%v: got %v, want %v",
				c.vendor, c.sizePriority, got, c.want)
		}
	}
}

// TestBuildPlan_QualityFirstPushApplied: on AMD the optimal-bitrate push is
// bounded by the absolute ceiling (not base+MaxOptimalOverride). The push result
// is recorded in plan.Notes; the post-encode preflight safety net may then raise
// QP further on pessimistic estimates (it stays untouched), so the note — not
// the final QP — is what proves the ceiling was applied.
func TestBuildPlan_QualityFirstPushApplied(t *testing.T) {
	pr := h264SDR()
	pr.PrimaryVideo.BitRate = 25_000_000 // dense Blu-ray-grade source
	pr.Format.BitRate = 26_000_000

	qf := amdVAAPICfg() // tune=auto → clean ceiling
	qfPlan := BuildPlan(qf, pr)

	// SmartQuality base is 14 here; the push target is well above the ceiling, so
	// the bounded push lands exactly on the clean ceiling.
	wantNote := fmt.Sprintf("ceiling %d", vcnQPCeilingClean)
	if !hasNote(qfPlan.Notes, wantNote) {
		t.Errorf("expected a quality-first push note mentioning %q, got %v", wantNote, qfPlan.Notes)
	}
}

// TestBuildPlan_SizePriorityRestoresLegacy: --size-priority on AMD must produce
// exactly the legacy unbounded-push plan — identical to the conservative path a
// non-AMD vendor takes by default — and must NOT emit the quality-first note.
func TestBuildPlan_SizePriorityRestoresLegacy(t *testing.T) {
	mk := func() *probe.ProbeResult {
		pr := h264SDR()
		pr.PrimaryVideo.BitRate = 25_000_000
		pr.Format.BitRate = 26_000_000
		return pr
	}

	sp := amdVAAPICfg()
	sp.Encoder.SizePriority = true
	spPlan := BuildPlan(sp, mk())

	legacy := defaultCfg() // unknown vendor → legacy push (conservative fallback)
	legacy.Encoder.Mode = config.EncoderVAAPI
	legacy.Encoder.VaapiProfile = "main10"
	legacy.Encoder.VaapiSwFormat = "p010"
	legacyPlan := BuildPlan(legacy, mk())

	if spPlan.VaapiQP != legacyPlan.VaapiQP {
		t.Errorf("size-priority QP %d should equal legacy-path QP %d", spPlan.VaapiQP, legacyPlan.VaapiQP)
	}
	if hasNote(spPlan.Notes, "quality-first push") {
		t.Errorf("size-priority must not emit a quality-first note, got %v", spPlan.Notes)
	}
}

// TestBuildPlan_QualityFirstNote_Disabled: a non-AMD vendor uses the legacy push
// and emits no quality-first note (the conservative fallback).
func TestBuildPlan_QualityFirstNote_Disabled(t *testing.T) {
	pr := h264SDR()
	pr.PrimaryVideo.BitRate = 25_000_000
	pr.Format.BitRate = 26_000_000

	cfg := defaultCfg()
	cfg.Encoder.Mode = config.EncoderVAAPI // vendor stays unknown
	plan := BuildPlan(cfg, pr)
	if hasNote(plan.Notes, "quality-first push") {
		t.Errorf("non-AMD vendor should not emit a quality-first note, got %v", plan.Notes)
	}
}

// TestGrainContentClassMin: on AMD, grain QP is floored at its knee and the
// quality-first push cannot drive it below that floor.
func TestGrainContentClassMin(t *testing.T) {
	cfg := amdVAAPICfg()
	cfg.Encoder.Tune = config.TuneGrain
	if got := vaapiContentClassMin(cfg); got != vcnContentClassMinGrain {
		t.Errorf("grain contentClassMin on AMD: got %d, want %d", got, vcnContentClassMinGrain)
	}

	// Non-grain and non-AMD keep the plain floor.
	clean := amdVAAPICfg()
	if got := vaapiContentClassMin(clean); got != VaapiQPMin {
		t.Errorf("clean contentClassMin on AMD: got %d, want %d", got, VaapiQPMin)
	}
	off := defaultCfg()
	off.Encoder.Mode = config.EncoderVAAPI
	off.Encoder.Tune = config.TuneGrain
	if got := vaapiContentClassMin(off); got != VaapiQPMin {
		t.Errorf("grain contentClassMin off-AMD: got %d, want %d (no calibrated floor)", got, VaapiQPMin)
	}
}

// TestQPInvariant_NoCombinationExitsRange is the Change 5 hard gate: across
// every (vendor, tune, denoise-bias, size-priority, source) combination, the
// planned VAAPI QP must stay within [contentClassMin, VaapiQPMax] — grain never
// below its knee, nothing above the clamp ceiling.
func TestQPInvariant_NoCombinationExitsRange(t *testing.T) {
	sources := []struct {
		name  string
		w, h  int
		kbps  int
		codec string
	}{
		{"480p 1000k h264", 854, 480, 1000, "h264"},
		{"1080p 2000k h264", 1920, 1080, 2000, "h264"},
		{"1080p 8000k h264", 1920, 1080, 8000, "h264"},
		{"1080p 25000k h264", 1920, 1080, 25000, "h264"},
		{"4K 40000k h264", 3840, 2160, 40000, "h264"},
		{"1080p 5000k hevc", 1920, 1080, 5000, "hevc"},
	}
	vendors := []config.VaapiVendor{
		config.VaapiVendorAMD, config.VaapiVendorIntel, config.VaapiVendorUnknown,
	}
	tunes := []config.TuneMode{
		config.TuneNone, config.TuneFilm, config.TuneGrain, config.TuneAnime,
	}

	for _, src := range sources {
		for _, vendor := range vendors {
			for _, tune := range tunes {
				for _, denoise := range []bool{false, true} {
					for _, sizePri := range []bool{false, true} {
						cfg := defaultCfg()
						cfg.Encoder.Mode = config.EncoderVAAPI
						cfg.Encoder.VaapiVendor = vendor
						cfg.Encoder.VaapiProfile = "main10"
						cfg.Encoder.VaapiSwFormat = "p010"
						cfg.Encoder.Tune = tune
						cfg.Encoder.DenoiseQPBias = denoise
						cfg.Encoder.SizePriority = sizePri
						cfg.SkipHEVC = false // force encode even for hevc sources

						pr := &probe.ProbeResult{
							PrimaryVideo: &probe.VideoStream{
								Codec: src.codec, Width: src.w, Height: src.h,
								BitRate: int64(src.kbps) * 1000,
							},
							Format: probe.FormatInfo{BitRate: int64(src.kbps)*1000 + 500_000},
						}

						plan := BuildPlan(cfg, pr)
						floor := vaapiContentClassMin(cfg)
						if plan.VaapiQP < floor || plan.VaapiQP > VaapiQPMax {
							t.Errorf("src=%s vendor=%s tune=%s denoise=%v size=%v: QP %d exits [%d, %d]",
								src.name, vendor, tune, denoise, sizePri,
								plan.VaapiQP, floor, VaapiQPMax)
						}
						// Grain on AMD must never fall below its knee.
						if vendor == config.VaapiVendorAMD && tune == config.TuneGrain &&
							plan.VaapiQP < vcnContentClassMinGrain {
							t.Errorf("src=%s denoise=%v size=%v: AMD grain QP %d below knee %d",
								src.name, denoise, sizePri, plan.VaapiQP, vcnContentClassMinGrain)
						}
					}
				}
			}
		}
	}
}

// TestGrainKneePinnedAt24 locks the grain QP knee at its measured value (24) for
// both the push ceiling and the stacked-bias floor. These are intentional
// production values (vcn31-vaapi-encode-facts: VMAF 99 at qp25, invisible gains
// below qp22), not free parameters — moving either off 24 must fail here.
func TestGrainKneePinnedAt24(t *testing.T) {
	if vcnQPCeilingGrain != 24 {
		t.Errorf("vcnQPCeilingGrain = %d, want 24 (measured grain knee)", vcnQPCeilingGrain)
	}
	if vcnContentClassMinGrain != 24 {
		t.Errorf("vcnContentClassMinGrain = %d, want 24 (measured grain knee)", vcnContentClassMinGrain)
	}

	// The full stacked bias (curves + smart bias + denoise tune bias) cannot
	// drive AMD grain below 24, even under maximal downward pressure.
	cfg := amdVAAPICfg()
	cfg.Encoder.Tune = config.TuneGrain
	cfg.Encoder.DenoiseQPBias = true   // the only downward tune bias (−1)
	cfg.Encoder.SmartQualityBias = -10 // force maximal downward pressure
	pr := &probe.ProbeResult{
		PrimaryVideo: &probe.VideoStream{
			Codec: "h264", Width: 1920, Height: 1080, BitRate: 30_000_000,
		},
		Format: probe.FormatInfo{BitRate: 31_000_000},
	}
	if sq := SmartQuality(cfg, pr); sq.VaapiQP < 24 {
		t.Errorf("AMD grain QP %d driven below knee 24 by stacked bias", sq.VaapiQP)
	}
}

// TestQualityFirstCeilingBindsH264_1080pBD is the headline done-criterion of the
// quality-first plan: a 1080p h264 Blu-ray under quality-first (AMD) must encode
// at the clean QP ceiling (16) and must NOT be bumped by preflight. Before the
// Change 6 point-estimate re-key, preflight keyed on the ×1.30 HIGH estimate and
// dragged this case up to QP ~20, defeating the ceiling.
func TestQualityFirstCeilingBindsH264_1080pBD(t *testing.T) {
	cfg := amdVAAPICfg() // tune=auto → clean ceiling, quality-first on
	pr := &probe.ProbeResult{
		PrimaryVideo: &probe.VideoStream{
			Codec: "h264", Width: 1920, Height: 1080, BitRate: 25_000_000,
		},
		Format: probe.FormatInfo{BitRate: 26_000_000},
	}
	plan := BuildPlan(cfg, pr)
	if plan.Action != ActionEncode {
		t.Fatalf("expected ActionEncode, got %v", plan.Action)
	}
	if plan.VaapiQP != vcnQPCeilingClean {
		t.Errorf("quality-first QP = %d, want ceiling %d (the ceiling must bind)",
			plan.VaapiQP, vcnQPCeilingClean)
	}
	if plan.PreflightBumps != 0 {
		t.Errorf("preflight bumped QP by %d; the ceiling must hold (point estimate ≤105%%)",
			plan.PreflightBumps)
	}
	t.Logf("1080p h264 BD quality-first: final QP=%d, preflight bumps=%d, estimate=%d-%d%% (point %d%%)",
		plan.VaapiQP, plan.PreflightBumps, plan.Estimate.LowPct, plan.Estimate.HighPct, plan.Estimate.PointPct)
}
