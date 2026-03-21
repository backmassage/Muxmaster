// BuildPlan entry point: wires quality, filters, audio, and subtitle sub-plans.
package planner

import (
	"fmt"

	"github.com/backmassage/muxmaster/internal/config"
	"github.com/backmassage/muxmaster/internal/probe"
)

// BuildPlan produces a complete FilePlan from config and probe data. This is
// the central decision matrix that the pipeline calls for every file.
//
// Flow:
//  1. Decide action (encode vs remux) via HEVC edge-safe check
//  2. Compute smart quality (resolution/bitrate curves + bias)
//  3. Build video filter chain (deinterlace, HDR tonemap, VAAPI hwupload)
//  4. Build audio plan (copy AAC, transcode others, layout normalization)
//  5. Build subtitle + attachment plans
//  6. Set stream dispositions, container opts, retry initial state
func BuildPlan(cfg *config.Config, pr *probe.ProbeResult) *FilePlan {
	plan := &FilePlan{
		MuxQueueSize:  4096,
		IncludeSubs:   cfg.KeepSubtitles,
		IncludeAttach: cfg.KeepAttachments,
	}

	v := pr.PrimaryVideo

	// --- 1. Action decision ---
	if cfg.SkipHEVC && v != nil && v.Codec == "hevc" {
		if pr.IsEdgeSafeHEVC() {
			plan.Action = ActionRemux
		} else {
			plan.Action = ActionEncode
			plan.QualityNote = fmt.Sprintf("HEVC profile '%s' not browser-safe; re-encoding", v.Profile)
		}
	} else {
		plan.Action = ActionEncode
	}

	// Remux targets are already edge-safe HEVC from clean sources — PTS
	// regeneration (+genpts) adds unnecessary container overhead. Only
	// enable timestamp repair for encodes; the retry engine can still
	// activate it for remuxes if ffmpeg fails with a timestamp error.
	if plan.Action == ActionRemux {
		plan.TimestampFix = false
	} else {
		plan.TimestampFix = cfg.CleanTimestamps
	}

	// --- 2. Smart quality ---
	q := SmartQuality(cfg, pr)
	plan.VaapiQP = q.VaapiQP
	plan.CpuCRF = q.CpuCRF
	if plan.QualityNote == "" {
		plan.QualityNote = q.Note
	}

	// --- 2b. Optimal bitrate selection ---
	// Compute an optimal target output bitrate based on the input's codec,
	// resolution, and density. This drives both the VAAPI QP selection and
	// the CPU maxrate ceiling, avoiding wasteful first-pass encodes that
	// produce output larger than the input.
	if plan.Action == ActionEncode && cfg.ActiveQualityOverride == "" && cfg.SmartQuality {
		optKbps := OptimalBitrate(pr)
		plan.OptimalBitrateKbps = optKbps

		if optKbps > 0 {
			if cfg.EncoderMode == config.EncoderVAAPI {
				targetQP := QPForTargetBitrate(cfg, pr, optKbps)
				if targetQP > plan.VaapiQP {
					ceiling := plan.VaapiQP + MaxOptimalOverride
					if targetQP > ceiling {
						targetQP = ceiling
					}
					plan.VaapiQP = targetQP
				}
			} else {
				targetCRF := CRFForTargetBitrate(cfg, pr, optKbps)
				if targetCRF > plan.CpuCRF {
					ceiling := plan.CpuCRF + MaxOptimalOverride
					if targetCRF > ceiling {
						targetCRF = ceiling
					}
					plan.CpuCRF = targetCRF
				}
			}
		}

		// Safety-net preflight: if the estimate exceeds 105% of input after
		// optimal targeting, bump further. The 5% headroom avoids chasing
		// marginal overshoot at the cost of quality — the post-encode
		// escalation loop handles genuine size blowups.
		adjQP, adjCRF, bumps := PreflightAdjust(cfg, pr, plan.VaapiQP, plan.CpuCRF, 105)
		if bumps > 0 {
			plan.VaapiQP = adjQP
			plan.CpuCRF = adjCRF
			plan.PreflightBumps = bumps
		}
		plan.Estimate = EstimateBitrate(cfg, pr, plan.VaapiQP, plan.CpuCRF)
	}

	// --- 2c. Bitrate ceiling (CPU only) ---
	// Set -maxrate to the optimal bitrate with headroom so the encoder's
	// CRF can target quality but never produce output larger than what we
	// expect. VAAPI constant-QP mode does not support -maxrate; the QP
	// targeting above handles VAAPI instead.
	if plan.Action == ActionEncode && cfg.EncoderMode == config.EncoderCPU {
		inputKbps := int(pr.VideoBitRate() / 1000)
		if inputKbps > 0 {
			// Use optimal bitrate + 15% headroom as ceiling, capped at
			// input bitrate (never exceed the source).
			ceiling := inputKbps
			if plan.OptimalBitrateKbps > 0 {
				ceiling = plan.OptimalBitrateKbps * cpuMaxrateHeadroomPct / 100
				if ceiling > inputKbps {
					ceiling = inputKbps
				}
			}
			plan.MaxRateKbps = ceiling
			plan.BufSizeKbps = ceiling * 2
		}
	}

	// --- 3. Video codec and filters ---
	switch plan.Action {
	case ActionRemux:
		plan.VideoCodec = "copy"
	case ActionEncode:
		switch cfg.EncoderMode {
		case config.EncoderVAAPI:
			plan.VideoCodec = "hevc_vaapi"
		case config.EncoderCPU:
			plan.VideoCodec = "libx265"
		}

		needsHDRTonemap := pr.HDRType() == "hdr10" && cfg.HandleHDR == config.HDRTonemap
		if cfg.EncoderMode == config.EncoderVAAPI && !needsHDRTonemap {
			plan.HWDecode = true
		}

		plan.VideoFilters = BuildVideoFilter(cfg, pr, plan.HWDecode)
		plan.ColorOpts = BuildColorOpts(cfg, pr)
	}

	// --- 4. Audio ---
	plan.Audio = BuildAudioPlan(cfg, pr)

	// --- 5. Subtitles and attachments ---
	plan.Subtitles = BuildSubtitlePlan(cfg, pr)
	plan.Attachments = BuildAttachmentPlan(cfg)

	// --- 6. Container opts ---
	if cfg.OutputContainer == config.ContainerMP4 {
		plan.ContainerOpts = []string{"-movflags", "+faststart"}
		plan.TagOpts = []string{"-tag:v", "hvc1"}
	}

	// --- 7. Stream dispositions ---
	plan.DispositionOpts = BuildDispositions(pr)

	plan.Container = cfg.OutputContainer
	plan.AudioStreamCount = len(pr.AudioStreams)
	if v != nil {
		plan.VideoStreamIdx = v.Index
	}
	return plan
}
