package planner

import (
	"fmt"

	"github.com/backmassage/muxmaster/internal/config"
	"github.com/backmassage/muxmaster/internal/probe"
)

// BuildAudioPlan produces the audio handling strategy for a file. It mirrors
// the legacy build_audio_opts function:
//
//   - No audio streams → NoAudio (produces -an).
//   - All streams are AAC → CopyAll (produces -map 0:a -c:a copy).
//   - Mixed → per-stream plan: copy AAC, transcode others to AAC with
//     optional MATCH_AUDIO_LAYOUT filter chains.
func BuildAudioPlan(cfg *config.Config, pr *probe.ProbeResult) AudioPlan {
	if len(pr.AudioStreams) == 0 {
		return AudioPlan{NoAudio: true}
	}

	allAAC := true
	for _, a := range pr.AudioStreams {
		if a.Codec != "aac" {
			allAAC = false
			break
		}
	}
	if allAAC {
		return AudioPlan{CopyAll: true}
	}

	var streams []AudioStreamPlan
	for i, a := range pr.AudioStreams {
		asp := AudioStreamPlan{
			StreamIndex: i,
			Channels:    clampChannels(a.Channels, cfg.AudioChannels),
			Bitrate:     cfg.AudioBitrate,
			SampleRate:  cfg.AudioSampleRate,
		}

		if a.Codec == "aac" {
			asp.Copy = true
			streams = append(streams, asp)
			continue
		}

		if cfg.MatchAudioLayout {
			asp.NeedsFilter = true
			asp.FilterStr = buildAudioFilter(asp.Channels)
			asp.Layout = layoutForChannels(asp.Channels)
		}

		streams = append(streams, asp)
	}
	return AudioPlan{Streams: streams}
}

func clampChannels(source, max int) int {
	if source < 1 {
		return 1
	}
	if source > max {
		return max
	}
	return source
}

// buildAudioFilter constructs the aresample+aformat chain used when
// MATCH_AUDIO_LAYOUT is enabled. This matches the legacy filter string:
//
//	aresample=async=1:first_pts=0:min_hard_comp=0.100,aformat=sample_rates=48000:channel_layouts=stereo
func buildAudioFilter(channels int) string {
	base := "aresample=async=1:first_pts=0:min_hard_comp=0.100,aformat=sample_rates=48000"
	layout := layoutForChannels(channels)
	if layout != "" {
		return fmt.Sprintf("%s:channel_layouts=%s", base, layout)
	}
	return base
}

func layoutForChannels(ch int) string {
	switch ch {
	case 1:
		return "mono"
	case 2:
		return "stereo"
	default:
		return ""
	}
}
