package planner

import (
	"fmt"
	"strings"

	"github.com/backmassage/muxmaster/internal/config"
	"github.com/backmassage/muxmaster/internal/probe"
)

// BuildPlan produces a FilePlan from config and probe data. This is the
// central decision matrix: HEVC edge-safe check determines remux vs encode,
// then codec, quality, audio, subtitle, and container settings are resolved.
//
// TODO: the following are deferred to full planner implementation:
//   - Smart per-file quality adaptation (quality.go)
//   - Bitrate estimation model (estimation.go)
//   - HDR tonemap filter chain (filter.go)
//   - Advanced audio layout normalization (audio.go)
func BuildPlan(cfg *config.Config, pr *probe.ProbeResult) *FilePlan {
	plan := &FilePlan{
		MuxQueueSize:  4096,
		TimestampFix:  cfg.CleanTimestamps,
		IncludeSubs:   cfg.KeepSubtitles,
		IncludeAttach: cfg.KeepAttachments,
		VaapiQP:       cfg.VaapiQP,
		CpuCRF:        cfg.CpuCRF,
	}

	v := pr.PrimaryVideo

	// --- Action decision ---
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

	// --- Video codec and filters ---
	switch plan.Action {
	case ActionRemux:
		plan.VideoCodec = "copy"
	case ActionEncode:
		buildVideoOpts(cfg, pr, plan)
	}

	// --- Audio ---
	buildAudioPlan(cfg, pr, plan)

	// --- Subtitles ---
	buildSubtitlePlan(cfg, pr, plan)

	// --- Attachments (MKV only) ---
	if cfg.KeepAttachments && cfg.OutputContainer == config.ContainerMKV {
		plan.Attachments = AttachmentPlan{Include: true}
	}

	// --- Container opts ---
	if cfg.OutputContainer == config.ContainerMP4 {
		plan.ContainerOpts = []string{"-movflags", "+faststart"}
		plan.TagOpts = []string{"-tag:v", "hvc1"}
	}

	// --- Stream dispositions ---
	buildDispositions(pr, plan)

	plan.AudioStreamCount = len(pr.AudioStreams)
	return plan
}

func buildVideoOpts(cfg *config.Config, pr *probe.ProbeResult, plan *FilePlan) {
	var filters []string

	// Deinterlace.
	if cfg.DeinterlaceAuto && pr.IsInterlaced() {
		filters = append(filters, "yadif")
	}

	switch cfg.EncoderMode {
	case config.EncoderVAAPI:
		plan.VideoCodec = "hevc_vaapi"
		swFormat := cfg.VaapiSwFormat
		if swFormat == "" {
			swFormat = "p010"
		}
		filters = append(filters, "format="+swFormat, "hwupload")
	case config.EncoderCPU:
		plan.VideoCodec = "libx265"
	}

	if len(filters) > 0 {
		plan.VideoFilters = strings.Join(filters, ",")
	}

	// HDR color metadata preservation on encode path.
	if cfg.HandleHDR == config.HDRPreserve && pr.HDRType() == "hdr10" {
		v := pr.PrimaryVideo
		if v.ColorTransfer != "" {
			plan.ColorOpts = append(plan.ColorOpts, "-color_trc", v.ColorTransfer)
		}
		if v.ColorPrimaries != "" {
			plan.ColorOpts = append(plan.ColorOpts, "-color_primaries", v.ColorPrimaries)
		}
		if v.ColorSpace != "" {
			plan.ColorOpts = append(plan.ColorOpts, "-colorspace", v.ColorSpace)
		}
	}
}

func buildAudioPlan(cfg *config.Config, pr *probe.ProbeResult, plan *FilePlan) {
	if len(pr.AudioStreams) == 0 {
		plan.Audio = AudioPlan{NoAudio: true}
		return
	}

	var streams []AudioStreamPlan
	for i, a := range pr.AudioStreams {
		asp := AudioStreamPlan{
			StreamIndex: i,
			Channels:    min(a.Channels, cfg.AudioChannels),
			Bitrate:     cfg.AudioBitrate,
			SampleRate:  cfg.AudioSampleRate,
		}
		if a.Codec == "aac" {
			asp.Copy = true
		}
		streams = append(streams, asp)
	}
	plan.Audio = AudioPlan{Streams: streams}
}

func buildSubtitlePlan(cfg *config.Config, pr *probe.ProbeResult, plan *FilePlan) {
	if !cfg.KeepSubtitles || len(pr.SubtitleStreams) == 0 {
		plan.Subtitles = SubtitlePlan{Include: false}
		return
	}

	if cfg.OutputContainer == config.ContainerMP4 {
		if pr.HasBitmapSubs {
			plan.Subtitles = SubtitlePlan{Include: false}
			return
		}
		plan.Subtitles = SubtitlePlan{Include: true, Codec: "mov_text"}
		return
	}

	plan.Subtitles = SubtitlePlan{Include: true, Codec: "copy"}
}

func buildDispositions(pr *probe.ProbeResult, plan *FilePlan) {
	plan.DispositionOpts = []string{"-disposition:v:0", "default"}
	if len(pr.AudioStreams) > 0 {
		plan.DispositionOpts = append(plan.DispositionOpts, "-disposition:a:0", "default")
		for i := 1; i < len(pr.AudioStreams); i++ {
			plan.DispositionOpts = append(plan.DispositionOpts,
				fmt.Sprintf("-disposition:a:%d", i), "0")
		}
	}
}
