// Output bitrate estimation, preflight adjustment, and QP/CRF targeting.
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
// source codec, resolution, bitrate, and density.
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
	var pixels int
	if v != nil && v.Width > 0 && v.Height > 0 {
		pixels = v.Width * v.Height
		ratio += resolutionBias(pixels)
	}

	// Bitrate bias.
	ratio += bitrateBias(inputKbps)

	// Density bias: already-compressed sources produce higher ratios
	// (output closer to or exceeding input). This matches the density
	// awareness in the quality curves.
	ratio += estimationDensityBias(inputKbps, pixels)

	ratio = Clamp(ratio, 220, 1050)

	lowRatio := ratio * 75 / 100
	highRatio := ratio * 130 / 100

	return BitrateEstimate{
		LowKbps:  (inputKbps*lowRatio + 500) / 1000,
		HighKbps: (inputKbps*highRatio + 500) / 1000,
		LowPct:   (lowRatio + 5) / 10,
		HighPct:  (highRatio + 5) / 10,
		Known:    true,
	}
}

// PreflightAdjust checks whether the estimated output would overshoot the
// input file size and iteratively bumps QP/CRF until the high estimate is
// within targetPct of the input. Returns the adjusted QP, CRF, and how
// many bumps were applied.
//
// It only adjusts the active encoder mode's value. targetPct is typically
// 105 (5% overshoot tolerance to avoid chasing marginal gains at quality
// cost). The adjustment is capped at 4 steps from the starting values to
// prevent the estimator from over-correcting on unreliable models.
func PreflightAdjust(cfg *config.Config, pr *probe.ProbeResult, vaapiQP, cpuCRF, targetPct int) (adjQP, adjCRF, bumps int) {
	const maxBumps = 4
	adjQP, adjCRF = vaapiQP, cpuCRF

	for i := 0; i < maxBumps; i++ {
		est := EstimateBitrate(cfg, pr, adjQP, adjCRF)
		if !est.Known || est.HighPct <= targetPct {
			return adjQP, adjCRF, i
		}

		if cfg.EncoderMode == config.EncoderVAAPI {
			if adjQP >= VaapiQPMax {
				return adjQP, adjCRF, i
			}
			adjQP++
		} else {
			if adjCRF >= CpuCRFMax {
				return adjQP, adjCRF, i
			}
			adjCRF++
		}
	}
	return adjQP, adjCRF, maxBumps
}

// qualityRatioPerMille returns the estimated output/input ratio (in per-mille)
// for a given encoder mode and quality setting.
func qualityRatioPerMille(mode config.EncoderMode, q int) int {
	if mode == config.EncoderVAAPI {
		return vaapiRatio(q)
	}
	return cpuRatio(q)
}

// vaapiRatio maps QP to an estimated output-to-input percentage (permille).
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
	case qp == 27:
		return 395
	case qp == 28:
		return 360
	case qp == 29:
		return 330
	case qp == 30:
		return 300
	case qp == 31:
		return 275
	case qp == 32:
		return 250
	case qp == 33:
		return 230
	case qp == 34:
		return 210
	case qp == 35:
		return 195
	default: // 36+
		return 180
	}
}

// cpuRatio covers CRF 16–30 (the full CpuCRFMin–CpuCRFMax range).
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
	case crf == 28:
		return 235
	case crf == 29:
		return 215
	default: // 30+
		return 200
	}
}

func codecBias(codec string) int {
	switch strings.ToLower(codec) {
	case "hevc", "h265":
		// HEVC→HEVC re-encoding gains almost nothing from the codec change;
		// output is very close to or larger than input. Much higher bias
		// than h264 sources which benefit from the h264→HEVC generation jump.
		return 180
	case "vp9", "av1":
		// Already modern/efficient codecs — limited compression gain.
		return 150
	case "h264", "avc", "avc1":
		return 100
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

// OptimalBitrate computes a target output video bitrate (in kbps) based on
// the input bitrate, resolution, and source codec. This represents what a
// well-tuned h264→HEVC encode should produce, and serves as:
//   - the maxrate ceiling for CPU encodes (with headroom)
//   - the QP selection guide for VAAPI encodes
//
// The target is always ≤ the input bitrate. For already-efficient codecs
// (HEVC, VP9, AV1), the target is close to the input since re-encoding
// offers minimal gains.
func OptimalBitrate(pr *probe.ProbeResult) int {
	if pr.PrimaryVideo == nil {
		return 0
	}

	inputKbps := int(pr.VideoBitRate() / 1000)
	if inputKbps <= 0 {
		return 0
	}

	v := pr.PrimaryVideo
	if v.Width <= 0 || v.Height <= 0 {
		return inputKbps
	}

	// Base ratio: expected output/input percentage based on codec generation
	// gain. h264→HEVC typically achieves 50-70% of input; legacy codecs
	// compress much further; modern codecs offer minimal gain.
	baseRatio := 68 // h264→HEVC default (higher = favor quality)
	switch strings.ToLower(v.Codec) {
	case "hevc", "h265":
		baseRatio = 95 // HEVC→HEVC: almost no codec gain
	case "vp9", "av1":
		baseRatio = 90 // already efficient modern codecs
	case "mpeg2video", "mpeg4", "wmv3", "vc1":
		baseRatio = 45 // large generation gain from legacy codecs
	}

	// Density adjustment: low-density (already compressed) sources can't be
	// compressed as aggressively. High-density sources have more room.
	pixels := v.Width * v.Height
	density := Density(inputKbps, pixels)
	switch {
	case density < DensityUltraLow:
		baseRatio += 30
	case density < DensityLow:
		baseRatio += 25
	case density < DensityBelowAvg:
		baseRatio += 12
	case density > DensityVeryHigh:
		baseRatio -= 8
	case density > DensityHigh:
		baseRatio -= 5
	}

	// Clamp ratio to sane range.
	if baseRatio > 100 {
		baseRatio = 100
	}
	if baseRatio < 30 {
		baseRatio = 30
	}

	target := inputKbps * baseRatio / 100

	// Never exceed input bitrate.
	if target > inputKbps {
		target = inputKbps
	}
	if target < MinOptimalBitrateKbps && inputKbps >= MinOptimalBitrateKbps {
		target = MinOptimalBitrateKbps
	}

	return target
}

// QPForTargetBitrate finds the VAAPI QP value that should produce output
// closest to the target bitrate, given the input bitrate and all estimation
// biases. It searches the QP range [VaapiQPMin, VaapiQPMax] and returns the
// QP whose estimated midpoint output is closest to the target.
func QPForTargetBitrate(cfg *config.Config, pr *probe.ProbeResult, targetKbps int) int {
	if pr.VideoBitRate() <= 0 || targetKbps <= 0 {
		return cfg.VaapiQP
	}

	bestQP := cfg.VaapiQP
	bestDist := 1<<31 - 1

	for qp := VaapiQPMin; qp <= VaapiQPMax; qp++ {
		est := EstimateBitrate(cfg, pr, qp, cfg.CpuCRF)
		if !est.Known {
			continue
		}
		mid := (est.LowKbps + est.HighKbps) / 2
		dist := mid - targetKbps
		if dist < 0 {
			dist = -dist
		}
		if dist < bestDist {
			bestDist = dist
			bestQP = qp
		}
	}
	return bestQP
}

// CRFForTargetBitrate finds the CPU CRF value that should produce output
// closest to the target bitrate, given the input bitrate and all estimation
// biases. Analogous to QPForTargetBitrate for the CPU encoder.
func CRFForTargetBitrate(cfg *config.Config, pr *probe.ProbeResult, targetKbps int) int {
	if pr.VideoBitRate() <= 0 || targetKbps <= 0 {
		return cfg.CpuCRF
	}

	bestCRF := cfg.CpuCRF
	bestDist := 1<<31 - 1

	for crf := CpuCRFMin; crf <= CpuCRFMax; crf++ {
		est := EstimateBitrate(cfg, pr, cfg.VaapiQP, crf)
		if !est.Known {
			continue
		}
		mid := (est.LowKbps + est.HighKbps) / 2
		dist := mid - targetKbps
		if dist < 0 {
			dist = -dist
		}
		if dist < bestDist {
			bestDist = dist
			bestCRF = crf
		}
	}
	return bestCRF
}

// estimationDensityBias adjusts the ratio prediction for sources where the
// bitrate is unusually low or high for the resolution. Low-density sources
// (already compressed) produce higher output ratios because the encoder
// can't compress already-degraded content further.
//
// VAAPI constant-QP output is determined by content complexity, not by
// input bitrate. The ratio-based estimation model (output = input × ratio)
// works well for high-bitrate sources with room to compress, but badly
// underestimates output for low-density sources where the encoder produces
// a similar absolute bitrate regardless of input.
//
// These biases correct the ratio prediction so the preflight check can
// bump QP before encoding. They are intentionally aggressive for
// density < 2500 kbps/Mpx because that's the range where VAAPI QP output
// routinely meets or exceeds the input bitrate.
func estimationDensityBias(kbps, pixels int) int {
	if kbps <= 0 || pixels <= 0 {
		return 0
	}
	density := Density(kbps, pixels)
	switch {
	case density < DensityUltraLow:
		return 250
	case density < DensityLow:
		return 150
	case density < DensityBelowAvg:
		return 80
	case density < DensityMedium:
		return 20
	case density > DensityVeryHigh:
		return -20
	case density > DensityHigh:
		return -10
	default:
		return 0
	}
}
