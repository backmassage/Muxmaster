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
		TimestampFix:  cfg.CleanTimestamps,
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

	// --- 2. Smart quality ---
	q := SmartQuality(cfg, pr)
	plan.VaapiQP = q.VaapiQP
	plan.CpuCRF = q.CpuCRF
	if plan.QualityNote == "" {
		plan.QualityNote = q.Note
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
		plan.VideoFilters = BuildVideoFilter(cfg, pr)
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

	plan.AudioStreamCount = len(pr.AudioStreams)
	if v != nil {
		plan.VideoStreamIdx = v.Index
	}
	return plan
}
