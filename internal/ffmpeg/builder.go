// builder.go constructs ffmpeg argument lists from FilePlan and RetryState.
package ffmpeg

import (
	"fmt"
	"strconv"

	"github.com/backmassage/muxmaster/internal/config"
	"github.com/backmassage/muxmaster/internal/planner"
)

// Build constructs the complete ffmpeg argument slice for a file. The
// generated command follows the shared skeleton documented in
// _docs/design/foundation-plan.md §7.2, with codec-specific sections
// injected for encode vs remux.
//
// The retry parameter supplies the current values for mux queue size,
// timestamp fix, subtitle/attachment inclusion, and quality, which may
// differ from the plan's initial values after retry adjustments.
func Build(cfg *config.Config, plan *planner.FilePlan, rs *RetryState) []string {
	args := make([]string, 0, 64)

	// --- Preamble ---
	args = append(args, "ffmpeg", "-hide_banner", "-nostdin", "-y")

	// Loglevel: info when verbose, otherwise error.
	if cfg.Display.Verbose {
		args = append(args, "-loglevel", "info")
	} else {
		args = append(args, "-loglevel", "error")
	}

	// Stats for FPS display.
	if cfg.Display.Verbose || cfg.Display.FfmpegFPS {
		args = append(args, "-stats", "-stats_period", "1")
	}

	// Probe constants.
	args = append(args,
		"-probesize", cfg.FFmpegProbesize,
		"-analyzeduration", cfg.FFmpegAnalyzeDuration,
		"-ignore_unknown",
	)

	// --- Pre-input flags (timestamp fix) ---
	if rs.TimestampFix {
		args = append(args, "-fflags", "+genpts+discardcorrupt")
	}

	// HW decode can be disabled by the retry engine after a hardware
	// decode failure; the plan's software fallback chain is used instead.
	hwDecode := plan.HWDecode && rs.HWDecode

	// --- VAAPI hardware device (encode path only) ---
	if plan.Action == planner.ActionEncode && cfg.Encoder.Mode == config.EncoderVAAPI {
		args = append(args,
			"-init_hw_device", "vaapi=va:"+cfg.Encoder.VaapiDevice,
		)
		if hwDecode {
			args = append(args,
				"-hwaccel", "vaapi",
				"-hwaccel_device", "va",
				"-hwaccel_output_format", "vaapi",
			)
		}
		args = append(args, "-filter_hw_device", "va")
	}

	// --- Input ---
	args = append(args, "-i", plan.InputPath)

	// --- Video filter chain (encode path only, before maps) ---
	videoFilters := plan.VideoFilters
	if plan.HWDecode && !hwDecode {
		videoFilters = plan.SWVideoFilters
	}
	if plan.Action == planner.ActionEncode && videoFilters != "" {
		// Scope the filter graph to the primary output video stream. A global
		// -vf also matches attached-picture video streams, which conflicts with
		// the later -c:v:N copy used to preserve MKV cover art.
		args = append(args, "-filter:v:0", videoFilters)
	}

	// --- Stream maps ---
	args = append(args, "-map", fmt.Sprintf("0:%d", plan.VideoStreamIdx))
	args = appendAudioMaps(args, cfg, plan, rs)
	args = appendSubtitleMaps(args, plan, rs)
	// Cover-art video streams must be mapped BEFORE the font/image
	// attachments: when -map 0:t? precedes an attached_pic video stream in
	// output order, the matroska muxer mis-routes the cover packet to an
	// attachment slot ("Received a packet for an attachment stream" -> EINVAL).
	args = appendAttachedPicMaps(args, plan)
	args = appendAttachmentMaps(args, plan, rs)

	// --- Global stream flags ---
	args = append(args,
		"-dn",
		"-max_muxing_queue_size", strconv.Itoa(rs.MuxQueueSize),
	)

	// --- Video codec ---
	args = appendVideoCodec(args, cfg, plan, rs)

	// --- Bitstream filters (e.g. dovi_rpu=strip=1 on remux) ---
	args = append(args, plan.BSFOpts...)

	// --- Cover art codec (after the video codec section: the per-stream
	// -c:v:N copy must come later than the global -c:v to win for that
	// stream). The maps themselves are emitted earlier, before attachments. ---
	args = appendAttachedPicCodecs(args, plan)

	// --- Tag opts (e.g. -tag:v hvc1 for MP4) ---
	args = append(args, plan.TagOpts...)

	// --- Color metadata (HDR preserve on encode path) ---
	args = append(args, plan.ColorOpts...)

	// --- Stream dispositions ---
	args = append(args, plan.DispositionOpts...)

	// --- Metadata and chapters ---
	args = append(args, "-map_metadata", "0", "-map_chapters", "0")

	// --- Post-input timestamp flag ---
	if rs.TimestampFix {
		args = append(args, "-avoid_negative_ts", "make_zero")
	}

	// --- Container opts (e.g. -movflags +faststart) ---
	args = append(args, plan.ContainerOpts...)

	// --- Output ---
	args = append(args, plan.OutputPath)

	return args
}

// appendVideoCodec adds the codec-specific arguments for the video stream.
func appendVideoCodec(args []string, cfg *config.Config, plan *planner.FilePlan, rs *RetryState) []string {
	switch plan.Action {
	case planner.ActionRemux:
		args = append(args, "-c:v", "copy")

	case planner.ActionEncode:
		gop := plan.KeyframeInterval
		if gop <= 0 {
			gop = cfg.Encoder.KeyframeInterval
		}
		switch cfg.Encoder.Mode {
		case config.EncoderVAAPI:
			args = append(args, "-c:v", "hevc_vaapi")
			if rs.VaapiQVBR {
				// QVBR: opt-in only, for *peak-bitrate bounding* on
				// bandwidth-limited streaming — NOT a quality improvement.
				// Benchmarking on VCN 3.1 showed QVBR loses quality-per-bit
				// to CQP at matched size (95.9 vs 98.9 VMAF) because -b:v binds
				// before the -global_quality target. CQP is the recommended
				// default; QVBR's only real use is bounding the bitrate spikes
				// that cause direct-play buffering, which constant-QP cannot.
				// The retry engine falls back to CQP if the driver rejects the
				// rate-control mode.
				args = append(args,
					"-rc_mode", "QVBR",
					"-global_quality", strconv.Itoa(rs.VaapiQP),
					"-b:v", strconv.Itoa(plan.OptimalBitrateKbps)+"k",
					"-maxrate", strconv.Itoa(plan.MaxRateKbps)+"k",
					"-bufsize", strconv.Itoa(plan.BufSizeKbps)+"k",
				)
			} else {
				args = append(args, "-qp", strconv.Itoa(rs.VaapiQP))
			}
			args = append(args,
				"-profile:v", cfg.Encoder.VaapiProfile,
				"-g", strconv.Itoa(gop),
				// Quality-first VA quality level. Inert for HEVC on AMD VCN
				// (bit-identical across 1/16/32/default on radeonsi); kept for
				// Intel/discrete-AMD portability where it maps to a real
				// speed/quality tradeoff. Driver-clamped, ignored with a
				// warning when unsupported.
				"-compression_level", strconv.Itoa(cfg.Encoder.VaapiCompressionLevel),
				// Deeper submission pipeline than the default 2 —
				// throughput only, no effect on output.
				"-async_depth", "4",
			)
			if rs.VaapiBFrames {
				args = append(args, "-bf", "4")
			}
			// HDR10 static metadata (ST.2086 mastering display + CTA-861.3
			// MaxCLL/MaxFALL) on the VAAPI path. Unlike libx265, hevc_vaapi has
			// no -master_display/-max_cll options: it re-emits the HDR SEI from
			// the decoded frames' AVMasteringDisplayMetadata/AVContentLightMetadata
			// side data, which the decoder parses from the source bitstream and
			// which survives the HW-decode surface chain and the SW prefilter
			// chains (hqdn3d/gradfun + hwupload). That is why plan.MasterDisplay
			// and plan.MaxCLL are not consumed here — the encoder reads the side
			// data directly, not these formatted strings.
			//
			// Emission is gated by the encoder's -sei flag, which defaults to
			// "hdr+a53_cc". We set it explicitly (to the same value, preserving
			// a53_cc caption passthrough) only when the plan carries HDR10
			// metadata, so the HDR SEI no longer depends on an ffmpeg default
			// that a future version could change silently. Verified on ffmpeg
			// 8.1 / Mesa radeonsi (VCN 3.1): ffprobe confirms "Mastering display
			// metadata" + "Content light level metadata" in the output bitstream.
			if plan.MasterDisplay != "" || plan.MaxCLL != "" {
				args = append(args, "-sei", "hdr+a53_cc")
			}
		case config.EncoderCPU:
			// aq-mode=3: auto-variance AQ biased toward dark scenes —
			// fights banding/blocking in dark content, the most visible
			// artifact class on TV playback.
			x265Params := "log-level=error:open-gop=0:aq-mode=3"
			if plan.MasterDisplay != "" {
				x265Params += ":master-display=" + plan.MasterDisplay
			}
			if plan.MaxCLL != "" {
				x265Params += ":max-cll=" + plan.MaxCLL
			}
			if plan.MasterDisplay != "" || plan.MaxCLL != "" {
				// repeat-headers puts the HDR10 SEI on every keyframe so
				// playback works from any seek point; hdr10-opt enables
				// x265's PQ-aware rate-distortion tuning.
				x265Params += ":hdr10=1:hdr10-opt=1:repeat-headers=1"
			}
			args = append(args,
				"-c:v", "libx265",
				"-crf", strconv.Itoa(rs.CpuCRF),
				"-preset", cfg.Encoder.CpuPreset,
				"-profile:v", cfg.Encoder.CpuProfile,
				"-pix_fmt", cfg.Encoder.CpuPixFmt,
				"-g", strconv.Itoa(gop),
				"-x265-params", x265Params,
			)
			// VBV-constrained CRF: cap output at the input video bitrate
			// so the encoder never produces output larger than the source.
			if plan.MaxRateKbps > 0 {
				args = append(args,
					"-maxrate", strconv.Itoa(plan.MaxRateKbps)+"k",
					"-bufsize", strconv.Itoa(plan.BufSizeKbps)+"k",
				)
			}
		default:
			panic(fmt.Sprintf("appendVideoCodec: unknown encoder mode %q", cfg.Encoder.Mode))
		}

	default:
		panic(fmt.Sprintf("appendVideoCodec: unknown action %d", plan.Action))
	}
	return args
}

// appendAudioMaps adds audio mapping and codec arguments.
func appendAudioMaps(args []string, cfg *config.Config, plan *planner.FilePlan, _ *RetryState) []string {
	ap := &plan.Audio

	if ap.NoAudio {
		return append(args, "-an")
	}

	if ap.CopyAll {
		return append(args, "-map", "0:a", "-c:a", "copy")
	}

	for _, s := range ap.Streams {
		args = append(args, "-map", fmt.Sprintf("0:a:%d", s.StreamIndex))

		if s.Copy {
			args = append(args, fmt.Sprintf("-c:a:%d", s.StreamIndex), "copy")
			continue
		}

		args = append(args,
			fmt.Sprintf("-c:a:%d", s.StreamIndex), cfg.Audio.Encoder,
			fmt.Sprintf("-ac:a:%d", s.StreamIndex), strconv.Itoa(s.Channels),
			fmt.Sprintf("-ar:a:%d", s.StreamIndex), strconv.Itoa(s.SampleRate),
			fmt.Sprintf("-b:a:%d", s.StreamIndex), s.Bitrate,
		)

		if s.NeedsFilter && s.FilterStr != "" {
			args = append(args,
				fmt.Sprintf("-filter:a:%d", s.StreamIndex), s.FilterStr,
			)
		}
	}
	return args
}

// appendSubtitleMaps adds subtitle mapping arguments, respecting the retry
// state's IncludeSubs flag. When SkipBitmap is set (MP4 with mixed text+bitmap
// subs), individual text streams are mapped instead of all subtitle streams.
func appendSubtitleMaps(args []string, plan *planner.FilePlan, rs *RetryState) []string {
	if !plan.Subtitles.Include || !rs.IncludeSubs {
		return args
	}

	if len(plan.Subtitles.StreamCodecs) > 0 {
		// Per-stream codecs (MKV with mov_text sources): map each subtitle
		// stream by absolute index and pair it with its output codec by
		// output-subtitle ordinal.
		for i, idx := range plan.Subtitles.TextIdxs {
			args = append(args, "-map", fmt.Sprintf("0:%d", idx))
			args = append(args, fmt.Sprintf("-c:s:%d", i), plan.Subtitles.StreamCodecs[i])
		}
		return args
	}

	if plan.Subtitles.SkipBitmap && len(plan.Subtitles.TextIdxs) > 0 {
		// Map only text subtitle streams by absolute index.
		for _, idx := range plan.Subtitles.TextIdxs {
			args = append(args, "-map", fmt.Sprintf("0:%d", idx))
		}
	} else {
		args = append(args, "-map", "0:s?")
	}

	if plan.Subtitles.Codec != "" {
		args = append(args, "-c:s", plan.Subtitles.Codec)
	}
	return args
}

// appendAttachedPicMaps maps cover-art video streams (MKV only) and tags them
// with the attached_pic disposition. These maps must precede the font/image
// attachment maps (0:t?) in output stream order; otherwise the matroska muxer
// mis-routes the cover packet to an attachment slot and fails with EINVAL. The
// stream-copy codec spec is emitted separately by appendAttachedPicCodecs.
func appendAttachedPicMaps(args []string, plan *planner.FilePlan) []string {
	for i, idx := range plan.AttachedPicIdxs {
		ord := i + 1
		args = append(args,
			"-map", fmt.Sprintf("0:%d", idx),
			fmt.Sprintf("-disposition:v:%d", ord), "attached_pic",
		)
	}
	return args
}

// appendAttachedPicCodecs stream-copies the cover-art video streams. The codec
// spec is emitted by video-stream ordinal (primary video is v:0, covers
// follow), which overrides the global encode codec because it is the later,
// more specific option — hence this is emitted after the video codec section,
// separately from appendAttachedPicMaps.
func appendAttachedPicCodecs(args []string, plan *planner.FilePlan) []string {
	for i := range plan.AttachedPicIdxs {
		ord := i + 1
		args = append(args, fmt.Sprintf("-c:v:%d", ord), "copy")
	}
	return args
}

// appendAttachmentMaps adds attachment mapping arguments (MKV only),
// respecting the retry state's IncludeAttach flag and MP4 constraints.
func appendAttachmentMaps(args []string, plan *planner.FilePlan, rs *RetryState) []string {
	if !plan.Attachments.Include || !rs.IncludeAttach {
		return args
	}
	if plan.Container == config.ContainerMP4 {
		return args
	}
	return append(args, "-map", "0:t?", "-c:t", "copy")
}
