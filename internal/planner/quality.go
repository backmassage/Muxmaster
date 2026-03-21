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
	if cfg.ActiveQualityOverride != "" {
		return QualityResult{
			VaapiQP: cfg.VaapiQP,
			CpuCRF:  cfg.CpuCRF,
			Note:    fmt.Sprintf("manual fixed override (%s=%s)", modeLabel(cfg), cfg.ActiveQualityOverride),
		}
	}

	if !cfg.SmartQuality {
		return QualityResult{
			VaapiQP: cfg.VaapiQP,
			CpuCRF:  cfg.CpuCRF,
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

	selectedCRF := Clamp(cfg.CpuCRF+cpuAdj+cfg.SmartQualityBias, CpuCRFMin, CpuCRFMax)
	selectedQP := Clamp(cfg.VaapiQP+vaapiAdj+cfg.SmartQualityBias, VaapiQPMin, VaapiQPMax)

	densityLabel := "n/a"
	if bitrateKbps > 0 && pixels > 0 {
		densityLabel = fmt.Sprintf("%d kbps/Mpx", bitrateKbps*1_000_000/pixels)
	}

	note := fmt.Sprintf("smart (%s, %s, density=%s, cpu_adj=%d, vaapi_adj=%d, smart_bias=%d, cpu_crf=%d, vaapi_qp=%d, mode=%s)",
		resLabel, bitrateLabel, densityLabel, cpuAdj, vaapiAdj, cfg.SmartQualityBias, selectedCRF, selectedQP, cfg.EncoderMode)

	return QualityResult{
		VaapiQP: selectedQP,
		CpuCRF:  selectedCRF,
		Note:    note,
	}
}

// CPU resolution curve: lower-res content gets a higher CRF (more
// compression), higher-res masters get lower CRF (more quality).
func cpuResolutionCurve(pixels int) int {
	if pixels <= 0 {
		return 0
	}
	switch {
	case pixels <= 640*360:
		return 4
	case pixels <= 854*480:
		return 3
	case pixels <= 1280*720:
		return 2
	case pixels <= 1920*1080:
		return 1
	case pixels >= 3840*2160:
		return -2
	case pixels >= 2560*1440:
		return -1
	default:
		return 0
	}
}

// VAAPI resolution curve: generally needs a slightly different QP ramp.
func vaapiResolutionCurve(pixels int) int {
	if pixels <= 0 {
		return 0
	}
	switch {
	case pixels <= 640*360:
		return 6
	case pixels <= 854*480:
		return 4
	case pixels <= 1280*720:
		return 3
	case pixels <= 1920*1080:
		return 1
	case pixels >= 3840*2160:
		return -1
	case pixels >= 2560*1440:
		return -1
	default:
		return 0
	}
}

// CPU bitrate adaptation.
func cpuBitrateCurve(kbps int) int {
	if kbps <= 0 {
		return 0
	}
	switch {
	case kbps < 1200:
		return 2
	case kbps < 2500:
		return 1
	case kbps > 35000:
		return -2
	case kbps > 18000:
		return -1
	default:
		return 0
	}
}

// VAAPI bitrate adaptation.
func vaapiBitrateCurve(kbps int) int {
	if kbps <= 0 {
		return 0
	}
	switch {
	case kbps < 1200:
		return 3
	case kbps < 2500:
		return 2
	case kbps > 30000:
		return -2
	case kbps > 16000:
		return -1
	default:
		return 0
	}
}

func modeLabel(cfg *config.Config) string {
	if cfg.EncoderMode == config.EncoderVAAPI {
		return "VAAPI_QP"
	}
	return "CPU_CRF"
}

// --- Bits-per-pixel density curves ---
//
// The resolution and bitrate curves above use absolute thresholds, which miss
// the case where bitrate is low *relative to* resolution (e.g. 3.9 Mbps at
// 1080p vs 3.9 Mbps at 480p). The density curves use kbps per megapixel to
// detect already-compressed sources and apply extra compression to avoid
// producing output that is larger than the input.
//
// Typical density ranges (kbps per megapixel):
//
//   < 1500  — heavily compressed (streaming rips, web-dl at low bitrate)
//   1500-2500 — below average for resolution
//   2500-5000 — typical for the resolution
//   5000-8000 — high quality source
//   > 8000  — very high quality (Blu-ray remux, lossless-adjacent)

// vaapiDensityCurve returns a QP adjustment based on bitrate density.
func vaapiDensityCurve(kbps, pixels int) int {
	if kbps <= 0 || pixels <= 0 {
		return 0
	}
	// kbps per megapixel (multiply by 1M to avoid float).
	density := kbps * 1_000_000 / pixels
	switch {
	case density < 1000:
		// Ultra-low density: source is so compressed that HEVC at any
		// reasonable quality level will produce output LARGER than the
		// input. Aggressive QP bump to minimize bloat.
		return 8
	case density < 1500:
		return 5
	case density < 2500:
		return 3
	case density < 3500:
		return 1
	case density > 8000:
		return -1
	default:
		return 0
	}
}

// cpuDensityCurve returns a CRF adjustment based on bitrate density.
func cpuDensityCurve(kbps, pixels int) int {
	if kbps <= 0 || pixels <= 0 {
		return 0
	}
	density := kbps * 1_000_000 / pixels
	switch {
	case density < 1000:
		// Ultra-low density: source is extremely compressed.
		return 6
	case density < 1500:
		return 4
	case density < 2500:
		return 2
	case density < 3500:
		return 1
	case density > 8000:
		return -1
	default:
		return 0
	}
}

// Quality clamp ranges from the legacy script. Exported for reuse by the
// retry engine in package ffmpeg.
const (
	CpuCRFMin  = 16
	CpuCRFMax  = 30
	VaapiQPMin = 14
	VaapiQPMax = 36
)

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
