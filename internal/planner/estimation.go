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
	if cfg.Encoder.Mode == config.EncoderVAAPI {
		qualityValue = vaapiQP
	} else {
		qualityValue = cpuCRF
	}

	ratio := qualityRatioPerMille(cfg.Encoder.Mode, qualityValue)

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

		if cfg.Encoder.Mode == config.EncoderVAAPI {
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

func vaapiRatio(qp int) int {
	i := Clamp(qp-vaapiRatioBase, 0, len(vaapiRatios)-1)
	return vaapiRatios[i]
}

func cpuRatio(crf int) int {
	i := Clamp(crf-cpuRatioBase, 0, len(cpuRatios)-1)
	return cpuRatios[i]
}

func codecBias(codec string) int {
	return codecBiases[strings.ToLower(codec)]
}

func resolutionBias(pixels int) int {
	return tierLookup(resBiasTiers, pixels, -40)
}

func bitrateBias(kbps int) int {
	return tierLookup(bitrateBiasTiers, kbps, -50)
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

	baseRatio := 68 // h264→HEVC default
	if r, ok := optimalCodecRatio[strings.ToLower(v.Codec)]; ok {
		baseRatio = r
	}

	pixels := v.Width * v.Height
	density := Density(inputKbps, pixels)
	baseRatio += tierLookup(optDensityTiers, density, -8)

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
		return cfg.Encoder.VaapiQP
	}

	bestQP := cfg.Encoder.VaapiQP
	bestDist := 1<<31 - 1

	for qp := VaapiQPMin; qp <= VaapiQPMax; qp++ {
		est := EstimateBitrate(cfg, pr, qp, cfg.Encoder.CpuCRF)
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
		return cfg.Encoder.CpuCRF
	}

	bestCRF := cfg.Encoder.CpuCRF
	bestDist := 1<<31 - 1

	for crf := CpuCRFMin; crf <= CpuCRFMax; crf++ {
		est := EstimateBitrate(cfg, pr, cfg.Encoder.VaapiQP, crf)
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

func estimationDensityBias(kbps, pixels int) int {
	if kbps <= 0 || pixels <= 0 {
		return 0
	}
	return tierLookup(estDensityTiers, Density(kbps, pixels), -20)
}
