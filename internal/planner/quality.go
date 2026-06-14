// SmartQuality: per-file QP/CRF from resolution, bitrate, and density curves.
package planner

import (
	"fmt"

	"github.com/backmassage/muxmaster/internal/config"
	"github.com/backmassage/muxmaster/internal/probe"
)

// QualityResult holds the resolved per-file quality settings.
type QualityResult struct {
	VaapiQP int
	CpuCRF  int
	Note    string
}

// SmartQuality computes per-file QP (VAAPI) and CRF (CPU) values by applying
// resolution and bitrate curves to the config defaults, then adding the
// configurable SmartQualityBias. This mirrors the legacy
// compute_smart_quality_settings function.
//
// When a manual quality override is active, the override values are returned
// unchanged. When smart quality is disabled, config defaults are returned.
func SmartQuality(cfg *config.Config, pr *probe.ProbeResult) QualityResult {
	if cfg.Encoder.ActiveQualityOverride != "" {
		return QualityResult{
			VaapiQP: cfg.Encoder.VaapiQP,
			CpuCRF:  cfg.Encoder.CpuCRF,
			Note:    fmt.Sprintf("manual fixed override (%s=%s)", modeLabel(cfg), cfg.Encoder.ActiveQualityOverride),
		}
	}

	if !cfg.Encoder.SmartQuality {
		return QualityResult{
			VaapiQP: cfg.Encoder.VaapiQP,
			CpuCRF:  cfg.Encoder.CpuCRF,
			Note:    "smart quality disabled",
		}
	}

	v := pr.PrimaryVideo
	var pixels int
	resLabel := "unknown"
	if v != nil && v.Width > 0 && v.Height > 0 {
		pixels = v.Width * v.Height
		resLabel = fmt.Sprintf("%dx%d", v.Width, v.Height)
	}

	bitrateKbps := VideoBitrateKbps(pr)
	bitrateLabel := "unknown"
	if bitrateKbps > 0 {
		bitrateLabel = fmt.Sprintf("%dkb/s", bitrateKbps)
	}

	cpuAdj := cpuResolutionCurve(pixels) + cpuBitrateCurve(bitrateKbps) + cpuDensityCurve(bitrateKbps, pixels)
	vaapiAdj := vaapiResolutionCurve(pixels) + vaapiBitrateCurve(bitrateKbps) + vaapiDensityCurve(bitrateKbps, pixels)

	// CPU CRF: curve + global bias, then optional per-tune bias, clamped to the
	// CRF range. The CPU path is out of scope for the hevc_vaapi quality plan.
	selectedCRF := Clamp(cfg.Encoder.CpuCRF+cpuAdj+cfg.Encoder.SmartQualityBias, CpuCRFMin, CpuCRFMax)
	if b := tuneCRFBias(cfg); b != 0 {
		selectedCRF = Clamp(selectedCRF+b, CpuCRFMin, CpuCRFMax)
	}

	// VAAPI QP: enforce the Change 5 stacked-bias invariant in a single clamp so
	// no (curve, global bias, tune bias) combination can exit
	// [contentClassMin, VaapiQPMax]. The lower bound is the per-content-class,
	// per-vendor floor (grain ≈ its knee on VCN); other classes use VaapiQPMin.
	qpFloor := vaapiContentClassMin(cfg)
	selectedQP := Clamp(cfg.Encoder.VaapiQP+vaapiAdj+cfg.Encoder.SmartQualityBias+tuneQPBias(cfg), qpFloor, VaapiQPMax)

	densityLabel := "n/a"
	if bitrateKbps > 0 && pixels > 0 {
		densityLabel = fmt.Sprintf("%d kbps/Mpx", Density(bitrateKbps, pixels))
	}

	note := fmt.Sprintf("smart (%s, %s, density=%s, cpu_adj=%d, vaapi_adj=%d, smart_bias=%d, tune=%s, cpu_crf=%d, vaapi_qp=%d, mode=%s)",
		resLabel, bitrateLabel, densityLabel, cpuAdj, vaapiAdj, cfg.Encoder.SmartQualityBias, cfg.Encoder.Tune, selectedCRF, selectedQP, cfg.Encoder.Mode)

	return QualityResult{
		VaapiQP: selectedQP,
		CpuCRF:  selectedCRF,
		Note:    note,
	}
}

// tuneQPBias and tuneCRFBias return the per-tune additive bias folded into the
// SmartQuality-selected QP/CRF.
//
//   - anime: 0 (Change 2). Was +1 (biased flat cels toward smaller files); a
//     quality-first default keeps cels at the curve QP so the deband prefilter's
//     gradients aren't re-quantized harder.
//   - film/grain: −1 only when DenoiseQPBias is set (Change 4, validation-gated,
//     default off) — spend the bits denoise frees on fidelity. The Change 5
//     invariant clamps the full stack to [contentClassMin, VaapiQPMax], so this
//     can never drive grain below its knee.
//
// The two functions are kept separate so the VAAPI and CPU paths can diverge.
func tuneQPBias(cfg *config.Config) int {
	switch cfg.Encoder.Tune {
	case config.TuneFilm, config.TuneGrain:
		if cfg.Encoder.DenoiseQPBias {
			return -1
		}
	}
	return 0
}

func tuneCRFBias(cfg *config.Config) int {
	switch cfg.Encoder.Tune {
	case config.TuneFilm, config.TuneGrain:
		if cfg.Encoder.DenoiseQPBias {
			return -1
		}
	}
	return 0
}

func cpuResolutionCurve(pixels int) int {
	if pixels <= 0 {
		return 0
	}
	return tierLookup(cpuResTiers, pixels, -2)
}

func vaapiResolutionCurve(pixels int) int {
	if pixels <= 0 {
		return 0
	}
	return tierLookup(vaapiResTiers, pixels, -2)
}

func cpuBitrateCurve(kbps int) int {
	if kbps <= 0 {
		return 0
	}
	return tierLookup(cpuBitrateTiers, kbps, -2)
}

func vaapiBitrateCurve(kbps int) int {
	if kbps <= 0 {
		return 0
	}
	return tierLookup(vaapiBitrateTiers, kbps, -2)
}

func modeLabel(cfg *config.Config) string {
	if cfg.Encoder.Mode == config.EncoderVAAPI {
		return "VAAPI_QP"
	}
	return "CPU_CRF"
}

func vaapiDensityCurve(kbps, pixels int) int {
	if kbps <= 0 || pixels <= 0 {
		return 0
	}
	return tierLookup(vaapiDensityTiers, Density(kbps, pixels), -2)
}

func cpuDensityCurve(kbps, pixels int) int {
	if kbps <= 0 || pixels <= 0 {
		return 0
	}
	return tierLookup(cpuDensityTiers, Density(kbps, pixels), -1)
}

// Quality clamp ranges from the legacy script. Exported for reuse by the
// retry engine in package ffmpeg.
const (
	CpuCRFMin  = 16
	CpuCRFMax  = 30
	VaapiQPMin = 14
	VaapiQPMax = 30
)

// Density thresholds in kbps per megapixel. Used by both quality curves
// and estimation biases. See _docs/design/quality-system.md.
const (
	DensityUltraLow = 1000  // Heavily compressed (streaming rips, web-dl).
	DensityLow      = 1500  // Below average for resolution.
	DensityBelowAvg = 2500  // Slightly below typical.
	DensityMedium   = 3500  // Average for resolution.
	DensityHigh     = 8000  // High quality source (Blu-ray).
	DensityVeryHigh = 10000 // Premium quality (remux grade).
)

// --- Change 1 & 5: per-content-class, per-vendor QP bounds (hevc_vaapi) ---
//
// These gate the quality-first default. The absolute QP ceiling bounds how high
// the optimal-bitrate push may raise QP (Change 1); the content-class floor
// bounds how low the stacked bias may drive it (Change 5 invariant). Both are
// VCN-3.1-calibrated and apply only to AMD devices — other vendors use the
// conservative fallback ceiling and the plain VaapiQPMin floor until measured.
//
// These are intentional production values for VCN-3.1-class AMD (the only
// calibrated vendor). They are reasoned estimates, not placeholders: the grain
// values track the measured grain knee from the vcn31-vaapi-encode-facts notes
// (VMAF 99 at qp25; below qp22 spends huge bits for invisible gains), and the
// clean ceiling is a deliberate quality-first floor on how high the push may go.
const (
	// vcnQPCeilingClean caps the optimal-bitrate push for clean/film/anime on
	// VCN. Reasoned estimate (no clean-content rate–QP sweep exists yet): 16
	// keeps the dominant h264/hevc path well inside the visually-transparent
	// band rather than letting the size-driven push climb toward QP ~20.
	vcnQPCeilingClean = 16
	// vcnQPCeilingGrain caps the push for grain content at the measured knee.
	// The knee sits at ~24–26 (qp25 ≈ VMAF 99); 24 is the low edge of the knee,
	// so the push can recover bits down to it without crossing into the
	// invisible-gain region below.
	vcnQPCeilingGrain = 24

	// vcnContentClassMinGrain is the measured grain knee (≈24 on VCN; below
	// qp22 = huge bits, invisible gains — vcn31-vaapi-encode-facts). The stacked
	// bias can never drive grain below this floor (Change 5 invariant).
	vcnContentClassMinGrain = 24

	// fallbackQPCeiling is the conservative ceiling for non-AMD vendors until a
	// per-device sweep calibrates them. Higher than the VCN value → allows more
	// push (closer to the legacy size-discipline behavior) rather than asserting
	// an unvalidated quality-first magnitude off-VCN.
	fallbackQPCeiling = 21
)

// vaapiUseQualityFirstPush reports whether the quality-first bounded push
// (Change 1) applies, versus the legacy unbounded base+MaxOptimalOverride push.
// It is gated to AMD/VCN — the only calibrated vendor (Change 5) — and disabled
// by --size-priority. Non-AMD/unknown vendors fall back to the legacy push,
// which is the conservative behavior until they are measured.
func vaapiUseQualityFirstPush(cfg *config.Config) bool {
	if cfg.Encoder.SizePriority {
		return false
	}
	return cfg.Encoder.VaapiVendor == config.VaapiVendorAMD
}

// boundedPushQP implements the Change 1 quality-first push: the optimal-bitrate
// push may raise QP from baseQP toward targetQP but never above the absolute
// ceiling, and never below baseQP (the push only raises QP).
//
//	finalQP = max(baseQP, min(targetQP, ceiling))
func boundedPushQP(baseQP, targetQP, ceiling int) int {
	return max(baseQP, min(targetQP, ceiling))
}

// vaapiQPCeiling returns the absolute QP ceiling for the optimal-bitrate push
// (Change 1), gated by vendor (Change 5). Only AMD/VCN uses the calibrated
// magnitudes; other vendors get the conservative fallback.
func vaapiQPCeiling(cfg *config.Config) int {
	if cfg.Encoder.VaapiVendor != config.VaapiVendorAMD {
		return fallbackQPCeiling
	}
	if cfg.Encoder.Tune == config.TuneGrain {
		return vcnQPCeilingGrain
	}
	return vcnQPCeilingClean
}

// vaapiContentClassMin returns the per-content-class lower bound on QP enforced
// after the full bias stack (Change 5 invariant). Only AMD/VCN grain raises the
// floor to its knee; every other case uses VaapiQPMin so behavior is unchanged
// where uncalibrated.
func vaapiContentClassMin(cfg *config.Config) int {
	if cfg.Encoder.Mode != config.EncoderVAAPI {
		return VaapiQPMin
	}
	if cfg.Encoder.VaapiVendor == config.VaapiVendorAMD && cfg.Encoder.Tune == config.TuneGrain {
		return vcnContentClassMinGrain
	}
	return VaapiQPMin
}

// Planner-level tuning constants exported for cross-package use.
const (
	// MaxOptimalOverride caps how far the optimal bitrate model can push
	// QP/CRF above the SmartQuality result.
	MaxOptimalOverride = 3

	// cpuMaxrateHeadroomPct is the headroom factor (as a percentage) applied
	// to the optimal bitrate when computing the CPU maxrate ceiling.
	cpuMaxrateHeadroomPct = 115

	// MinOptimalBitrateKbps is the floor for the optimal bitrate target.
	MinOptimalBitrateKbps = 200
)

// VideoBitrateKbps converts the probe result's video bitrate from bps to kbps
// with rounding to the nearest integer. All planner code should use this
// instead of raw division to avoid inconsistent truncation vs rounding.
func VideoBitrateKbps(pr *probe.ProbeResult) int {
	return int((pr.VideoBitRate() + 500) / 1000)
}

// Density computes bitrate density in kbps per megapixel. The multiplication
// is done in int64 so a high-bitrate source (kbps × 1e6 exceeds 2^31) can't
// overflow on 32-bit builds; the quotient always fits back in an int.
func Density(kbps, pixels int) int {
	if pixels <= 0 {
		return 0
	}
	return int(int64(kbps) * 1_000_000 / int64(pixels))
}

// Clamp restricts v to the range [lo, hi].
func Clamp(v, lo, hi int) int {
	if v < lo {
		return lo
	}
	if v > hi {
		return hi
	}
	return v
}
