package planner

import (
	"github.com/backmassage/muxmaster/internal/config"
	"github.com/backmassage/muxmaster/internal/probe"
)

// BuildSubtitlePlan decides subtitle handling. MKV gets a straight copy,
// MP4 gets mov_text for text subs and skips bitmap subs.
// Mirrors the legacy build_subtitle_opts and describe_subtitle_plan functions.
func BuildSubtitlePlan(cfg *config.Config, pr *probe.ProbeResult) SubtitlePlan {
	if !cfg.KeepSubtitles || len(pr.SubtitleStreams) == 0 {
		return SubtitlePlan{Include: false}
	}

	if cfg.OutputContainer == config.ContainerMP4 {
		// Collect text (non-bitmap) subtitle stream indices.
		var textIdxs []int
		for _, s := range pr.SubtitleStreams {
			if !s.IsBitmap {
				textIdxs = append(textIdxs, s.Index)
			}
		}
		if len(textIdxs) == 0 {
			// All subs are bitmap — MP4 can't carry any of them.
			return SubtitlePlan{Include: false}
		}
		return SubtitlePlan{
			Include:    true,
			Codec:      "mov_text",
			SkipBitmap: pr.HasBitmapSubs,
			TextIdxs:   textIdxs,
		}
	}

	return SubtitlePlan{Include: true, Codec: "copy"}
}

// BuildAttachmentPlan decides whether to carry font/image attachments.
// Only MKV supports attachments; MP4 always skips them.
func BuildAttachmentPlan(cfg *config.Config) AttachmentPlan {
	if cfg.KeepAttachments && cfg.OutputContainer == config.ContainerMKV {
		return AttachmentPlan{Include: true}
	}
	return AttachmentPlan{Include: false}
}
