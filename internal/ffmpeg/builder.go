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
	if cfg.Verbose {
		args = append(args, "-loglevel", "info")
	} else {
		args = append(args, "-loglevel", "error")
	}

	// Stats for FPS display.
	if cfg.Verbose || cfg.ShowFfmpegFPS {
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

	// --- VAAPI hardware device (encode path only) ---
	if plan.Action == planner.ActionEncode && cfg.EncoderMode == config.EncoderVAAPI {
		args = append(args,
			"-init_hw_device", "vaapi=va:"+cfg.VaapiDevice,
			"-filter_hw_device", "va",
		)
	}

	// --- Input ---
	args = append(args, "-i", plan.InputPath)

	// --- Video filter chain (encode path only, before maps) ---
	if plan.Action == planner.ActionEncode && plan.VideoFilters != "" {
		args = append(args, "-vf", plan.VideoFilters)
	}

	// --- Stream maps ---
	args = append(args, "-map", fmt.Sprintf("0:%d", plan.VideoStreamIdx))
	args = appendAudioMaps(args, cfg, plan, rs)
	args = appendSubtitleMaps(args, plan, rs)
	args = appendAttachmentMaps(args, plan, rs)

	// --- Global stream flags ---
	args = append(args,
		"-dn",
		"-max_muxing_queue_size", strconv.Itoa(rs.MuxQueueSize),
		"-max_interleave_delta", "0",
	)

	// --- Video codec ---
	args = appendVideoCodec(args, cfg, plan, rs)

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
		switch cfg.EncoderMode {
		case config.EncoderVAAPI:
			args = append(args,
				"-c:v", "hevc_vaapi",
				"-qp", strconv.Itoa(rs.VaapiQP),
				"-profile:v", cfg.VaapiProfile,
				"-g", strconv.Itoa(cfg.KeyframeInterval),
			)
		case config.EncoderCPU:
			args = append(args,
				"-c:v", "libx265",
				"-crf", strconv.Itoa(rs.CpuCRF),
				"-preset", cfg.CpuPreset,
				"-profile:v", cfg.CpuProfile,
				"-pix_fmt", cfg.CpuPixFmt,
				"-g", strconv.Itoa(cfg.KeyframeInterval),
				"-x265-params", "log-level=error:open-gop=0",
			)
		}
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
			fmt.Sprintf("-c:a:%d", s.StreamIndex), cfg.AudioEncoder,
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
