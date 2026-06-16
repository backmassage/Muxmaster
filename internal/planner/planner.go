// BuildPlan entry point: wires quality, filters, audio, and subtitle sub-plans.
package planner

import (
	"fmt"
	"strings"

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

	// --- 0. Dolby Vision policy ---
	// Profile 5 video carries no HDR10-compatible base layer (IPTPQc2
	// signal); stripping the RPUs leaves unwatchable green/purple output,
	// and the only correct conversion path (libplacebo tonemap) is out of
	// scope. Skip the file instead of producing a broken one.
	if v != nil && v.DoviProfile == 5 {
		plan.Action = ActionSkip
		plan.SkipReason = "Dolby Vision profile 5 (no HDR10 base layer)"
		plan.Container = cfg.OutputContainer
		return plan
	}

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

	// --- 1b. Dolby Vision stripping (profiles with an HDR10 base layer) ---
	// On remux, -c:v copy would carry the DOVI configuration record into the
	// output and DV-capable clients would engage DV mode on a stream we
	// otherwise treat as HDR10; dovi_rpu=strip=1 removes both the config record
	// and the per-frame RPUs. On encode, re-encoding drops the RPUs on both
	// encoder paths (hevc_vaapi never writes them; libx265 only with explicit
	// dolby-vision-rpu config), so only a note is needed.
	if v != nil && v.DoviProfile > 0 {
		if plan.Action == ActionRemux {
			// Scoped to v:0 so the bsf never touches cover-art streams
			// (dovi_rpu rejects non-HEVC codecs at init).
			plan.BSFOpts = []string{"-bsf:v:0", "dovi_rpu=strip=1"}
		}
		plan.Notes = append(plan.Notes, "stripping Dolby Vision, keeping HDR10 base layer")
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
	if plan.Action == ActionEncode && cfg.Encoder.ActiveQualityOverride == "" && cfg.Encoder.SmartQuality {
		optKbps := OptimalBitrate(pr)
		plan.OptimalBitrateKbps = optKbps

		// --quality-priority keeps the SmartQuality value as-is by skipping the
		// upward optimal-bitrate push (which trades quality for smaller files).
		// The preflight/post-encode size safety nets below stay intact — they
		// only fire when output ≳ input, which never happens for Blu-ray.
		if cfg.Encoder.QualityPriority {
			plan.Notes = append(plan.Notes,
				"quality-priority: keeping SmartQuality QP, skipping optimal-bitrate push")
		}
		if optKbps > 0 && !cfg.Encoder.QualityPriority {
			if cfg.Encoder.Mode == config.EncoderVAAPI {
				targetQP := QPForTargetBitrate(cfg, pr, optKbps)
				baseQP := plan.VaapiQP
				// Change 1 + Change 5: quality-first by default *where calibrated*.
				// On VCN-class AMD the push may raise QP only to an absolute
				// per-content ceiling:
				//   finalQP = max(baseQP, min(pushTargetQP, qpCeiling))
				// --size-priority, and any non-AMD/unknown vendor (no calibrated
				// ceiling yet), fall back to the legacy unbounded push
				// (base+MaxOptimalOverride) — the conservative behavior.
				if vaapiUseQualityFirstPush(cfg) {
					ceiling := vaapiQPCeiling(cfg)
					finalQP := boundedPushQP(baseQP, targetQP, ceiling)
					if finalQP != baseQP {
						plan.Notes = append(plan.Notes,
							fmt.Sprintf("quality-first push: QP %d→%d (ceiling %d, target %d)",
								baseQP, finalQP, ceiling, targetQP))
					}
					plan.VaapiQP = finalQP
				} else if targetQP > baseQP {
					ceiling := baseQP + MaxOptimalOverride
					if targetQP > ceiling {
						targetQP = ceiling
					}
					plan.VaapiQP = targetQP
				}
			} else {
				// CPU path is out of scope for the hevc_vaapi quality plan; the
				// legacy bounded push stays.
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

	// --- 2c. Bitrate ceiling (CPU CRF and VAAPI QVBR) ---
	// Set -maxrate to the optimal bitrate with headroom so the encoder can
	// target quality but never produce output larger than what we expect.
	// VAAPI constant-QP mode does not support -maxrate (the QP targeting
	// above handles that case); QVBR does, which is its whole point — it
	// bounds the peak spikes that cause direct-play buffering.
	qvbrRequested := cfg.Encoder.Mode == config.EncoderVAAPI &&
		cfg.Encoder.VaapiRC == config.VaapiRCQVBR && cfg.Encoder.VaapiQVBR
	if plan.Action == ActionEncode && (cfg.Encoder.Mode == config.EncoderCPU || qvbrRequested) {
		inputKbps := VideoBitrateKbps(pr)
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
	// QVBR needs both a target bitrate and a ceiling; without them (smart
	// quality disabled or unknown input bitrate) fall back to constant QP.
	if plan.Action == ActionEncode && qvbrRequested &&
		plan.OptimalBitrateKbps > 0 && plan.MaxRateKbps > 0 {
		plan.VaapiQVBR = true
	}
	if plan.Action == ActionEncode && cfg.Encoder.Mode == config.EncoderVAAPI {
		plan.VaapiBFrames = cfg.Encoder.VaapiBFrames
	}

	// --- 3. Video codec and filters ---
	switch plan.Action {
	case ActionRemux:
		plan.VideoCodec = "copy"
	case ActionEncode:
		switch cfg.Encoder.Mode {
		case config.EncoderVAAPI:
			plan.VideoCodec = "hevc_vaapi"
		case config.EncoderCPU:
			plan.VideoCodec = "libx265"
		}

		needsHDRTonemap := pr.HDRType() == "hdr10" && cfg.Encoder.HandleHDR == config.HDRTonemap
		// A content prefilter is a CPU filter; it cannot enter a VAAPI surface
		// chain (ffmpeg error -22), so its presence forces software decode.
		prefilter := TunePrefilter(cfg)
		if cfg.Encoder.Mode == config.EncoderVAAPI && !needsHDRTonemap && prefilter == "" && vaapiHWDecodeViable(pr) {
			plan.HWDecode = true
		}
		if cfg.Encoder.Mode == config.EncoderVAAPI && prefilter != "" {
			plan.Notes = append(plan.Notes,
				fmt.Sprintf("tune=%s prefilter %q; GPU decode disabled", cfg.Encoder.Tune, prefilter))
		}

		plan.VideoFilters = BuildVideoFilter(cfg, pr, plan.HWDecode)
		if plan.HWDecode {
			// Fallback chain for the retry engine: if hardware decode fails
			// at runtime (codec the driver can't decode), the encode is
			// retried with software decode + hwupload using this chain.
			plan.SWVideoFilters = BuildVideoFilter(cfg, pr, false)
		}
		if cfg.Encoder.Mode == config.EncoderVAAPI {
			// Last-resort fallback chain for the RetryFallbackCPU path: a pure
			// CPU chain with no hwupload, for drivers where the SW-decode +
			// hwupload + hevc_vaapi graph fails negotiation ("Impossible to
			// convert between the formats supported by 'hwupload' and
			// 'auto_scale'"). Built as if in CPU mode so BuildVideoFilter omits
			// the hwupload tail and ends the tonemap chain in yuv420p.
			cpuCfg := *cfg
			cpuCfg.Encoder.Mode = config.EncoderCPU
			plan.CPUVideoFilters = BuildVideoFilter(&cpuCfg, pr, false)
		}
		plan.ColorOpts = BuildColorOpts(cfg, pr)
		BuildHDR10Meta(cfg, pr, plan)

		plan.KeyframeInterval = keyframeInterval(cfg, pr)
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
	// NB: do NOT set -max_interleave_delta 0 for MKV with subtitles. That
	// disables the muxer's bounded interleave flush, so a single sparse
	// stream (e.g. a forced-narrative subtitle track with cues only late in
	// the file) makes the muxer buffer every video packet in RAM waiting to
	// interleave. On a long 4K remux that grows unbounded until the OOM
	// killer sends SIGKILL — which prints nothing, so the run fails silently
	// with "size=16KiB time=N/A" and the retry engine has no error to match.
	// ffmpeg's default 10 s cap flushes A/V correctly; the only cost is
	// slightly less-optimal interleaving around sparse subs, which is
	// harmless for local playback (cues keep their PTS, players use the index).

	// Cover art: the builder maps only the primary video stream, so attached
	// pictures would be silently dropped. MKV carries them as video streams
	// with the attached_pic disposition; MP4 cover art handling differs, so
	// it stays skipped there.
	if cfg.OutputContainer == config.ContainerMKV {
		plan.AttachedPicIdxs = pr.AttachedPicIdxs
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

// keyframeInterval derives the GOP length from the source frame rate,
// targeting a ~2 second keyframe cadence (the standard segment length for
// Jellyfin/HLS streaming). The config value (48, i.e. 2s at 24fps) is the
// fallback when the frame rate is unknown. The result is clamped so corrupt
// frame-rate metadata can't produce degenerate GOPs.
func keyframeInterval(cfg *config.Config, pr *probe.ProbeResult) int {
	g := cfg.Encoder.KeyframeInterval
	if v := pr.PrimaryVideo; v != nil {
		if fps := v.FrameRate(); fps > 0 {
			g = Clamp(int(fps*2+0.5), 24, 300)
		}
	}
	return g
}

// vaapiHWDecodeViable is false when VAAPI hardware decode is known to fail for
// typical drivers while software decode + hwupload still works. Hi10p AVC
// (H.264 + 10-bit pix_fmt) triggers "hwaccel initialisation returned error"
// on many stacks; ffmpeg then feeds CPU frames into a scale_vaapi graph.
func vaapiHWDecodeViable(pr *probe.ProbeResult) bool {
	v := pr.PrimaryVideo
	if v == nil {
		return true
	}
	if isAVCCodec(v.Codec) && probe.PixFmtIs10Bit(v.PixFmt) {
		return false
	}
	return true
}

func isAVCCodec(codec string) bool {
	switch strings.ToLower(strings.TrimSpace(codec)) {
	case "h264", "avc", "avc1":
		return true
	default:
		return false
	}
}
