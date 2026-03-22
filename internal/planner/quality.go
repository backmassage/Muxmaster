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

	bitrateKbps := int(pr.VideoBitRate() / 1000)
	bitrateLabel := "unknown"
	if bitrateKbps > 0 {
		bitrateLabel = fmt.Sprintf("%dkb/s", bitrateKbps)
	}

	cpuAdj := cpuResolutionCurve(pixels) + cpuBitrateCurve(bitrateKbps) + cpuDensityCurve(bitrateKbps, pixels)
	vaapiAdj := vaapiResolutionCurve(pixels) + vaapiBitrateCurve(bitrateKbps) + vaapiDensityCurve(bitrateKbps, pixels)

	selectedCRF := Clamp(cfg.Encoder.CpuCRF+cpuAdj+cfg.Encoder.SmartQualityBias, CpuCRFMin, CpuCRFMax)
	selectedQP := Clamp(cfg.Encoder.VaapiQP+vaapiAdj+cfg.Encoder.SmartQualityBias, VaapiQPMin, VaapiQPMax)

	densityLabel := "n/a"
	if bitrateKbps > 0 && pixels > 0 {
		densityLabel = fmt.Sprintf("%d kbps/Mpx", Density(bitrateKbps, pixels))
	}

	note := fmt.Sprintf("smart (%s, %s, density=%s, cpu_adj=%d, vaapi_adj=%d, smart_bias=%d, cpu_crf=%d, vaapi_qp=%d, mode=%s)",
		resLabel, bitrateLabel, densityLabel, cpuAdj, vaapiAdj, cfg.Encoder.SmartQualityBias, selectedCRF, selectedQP, cfg.Encoder.Mode)

	return QualityResult{
		VaapiQP: selectedQP,
		CpuCRF:  selectedCRF,
		Note:    note,
	}
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

// Density computes bitrate density in kbps per megapixel.
func Density(kbps, pixels int) int {
	if pixels <= 0 {
		return 0
	}
	return kbps * 1_000_000 / pixels
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
