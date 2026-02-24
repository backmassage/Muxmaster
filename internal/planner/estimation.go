package planner

import (
	"strings"

	"github.com/backmassage/muxmaster/internal/config"
	"github.com/backmassage/muxmaster/internal/probe"
)

// BitrateEstimate holds the estimated output bitrate range for display.
type BitrateEstimate struct {
	LowKbps  int
	HighKbps int
	LowPct   int
	HighPct  int
	Known    bool
}

// EstimateBitrate predicts the output video bitrate range for an encode
// operation. It uses the quality-to-ratio tables from the legacy
// estimate_transcode_video_output_range function, with adjustments for
// source codec, resolution, and bitrate.
func EstimateBitrate(cfg *config.Config, pr *probe.ProbeResult, vaapiQP, cpuCRF int) BitrateEstimate {
	inputBps := pr.VideoBitRate()
	if inputBps <= 0 {
		return BitrateEstimate{}
	}
	inputKbps := int((inputBps + 500) / 1000)

	var qualityValue int
	if cfg.EncoderMode == config.EncoderVAAPI {
		qualityValue = vaapiQP
	} else {
		qualityValue = cpuCRF
	}

	ratio := qualityRatioPerMille(cfg.EncoderMode, qualityValue)

	// Source codec bias: modern codecs leave less room for compression.
	v := pr.PrimaryVideo
	if v != nil {
		ratio += codecBias(v.Codec)
	}

	// Resolution bias.
	if v != nil && v.Width > 0 && v.Height > 0 {
		ratio += resolutionBias(v.Width * v.Height)
	}

	// Bitrate bias.
	ratio += bitrateBias(inputKbps)

	ratio = clamp(ratio, 220, 1050)

	lowRatio := ratio * 75 / 100
	highRatio := ratio * 145 / 100

	return BitrateEstimate{
		LowKbps:  (inputKbps*lowRatio + 500) / 1000,
		HighKbps: (inputKbps*highRatio + 500) / 1000,
		LowPct:   (lowRatio + 5) / 10,
		HighPct:  (highRatio + 5) / 10,
		Known:    true,
	}
}

// qualityRatioPerMille returns the estimated output/input ratio (in per-mille)
// for a given encoder mode and quality setting. These tables mirror the legacy
// estimate_quality_ratio_per_mille function exactly.
func qualityRatioPerMille(mode config.EncoderMode, q int) int {
	if mode == config.EncoderVAAPI {
		return vaapiRatio(q)
	}
	return cpuRatio(q)
}

func vaapiRatio(qp int) int {
	switch {
	case qp <= 14:
		return 930
	case qp == 15:
		return 900
	case qp == 16:
		return 860
	case qp == 17:
		return 820
	case qp == 18:
		return 770
	case qp == 19:
		return 730
	case qp == 20:
		return 680
	case qp == 21:
		return 640
	case qp == 22:
		return 590
	case qp == 23:
		return 550
	case qp == 24:
		return 510
	case qp == 25:
		return 470
	case qp == 26:
		return 430
	default:
		return 390
	}
}

func cpuRatio(crf int) int {
	switch {
	case crf <= 16:
		return 900
	case crf == 17:
		return 820
	case crf == 18:
		return 740
	case crf == 19:
		return 660
	case crf == 20:
		return 590
	case crf == 21:
		return 520
	case crf == 22:
		return 460
	case crf == 23:
		return 410
	case crf == 24:
		return 360
	case crf == 25:
		return 320
	case crf == 26:
		return 290
	case crf == 27:
		return 260
	default:
		return 230
	}
}

func codecBias(codec string) int {
	switch strings.ToLower(codec) {
	case "h264", "avc", "avc1", "hevc", "h265", "vp9", "av1":
		return 110
	case "mpeg2video", "mpeg4", "wmv3", "vc1":
		return -60
	default:
		return 0
	}
}

func resolutionBias(pixels int) int {
	switch {
	case pixels <= 854*480:
		return 80
	case pixels <= 1280*720:
		return 40
	case pixels >= 3840*2160:
		return -40
	default:
		return 0
	}
}

func bitrateBias(kbps int) int {
	switch {
	case kbps < 1500:
		return 120
	case kbps < 3000:
		return 70
	case kbps > 30000:
		return -50
	case kbps > 15000:
		return -20
	default:
		return 0
	}
}
