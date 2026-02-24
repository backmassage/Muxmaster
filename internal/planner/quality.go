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

	cpuAdj := cpuResolutionCurve(pixels) + cpuBitrateCurve(bitrateKbps)
	vaapiAdj := vaapiResolutionCurve(pixels) + vaapiBitrateCurve(bitrateKbps)

	selectedCRF := clamp(cfg.CpuCRF+cpuAdj+cfg.SmartQualityBias, cpuCRFMin, cpuCRFMax)
	selectedQP := clamp(cfg.VaapiQP+vaapiAdj+cfg.SmartQualityBias, vaapiQPMin, vaapiQPMax)

	note := fmt.Sprintf("smart (%s, %s, cpu_adj=%d, vaapi_adj=%d, smart_bias=%d, cpu_crf=%d, vaapi_qp=%d, mode=%s)",
		resLabel, bitrateLabel, cpuAdj, vaapiAdj, cfg.SmartQualityBias, selectedCRF, selectedQP, cfg.EncoderMode)

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

// Quality clamp ranges from the legacy script.
const (
	cpuCRFMin  = 16
	cpuCRFMax  = 30
	vaapiQPMin = 14
	vaapiQPMax = 36
)

func clamp(v, lo, hi int) int {
	if v < lo {
		return lo
	}
	if v > hi {
		return hi
	}
	return v
}
