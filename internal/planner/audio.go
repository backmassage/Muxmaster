// Per-stream audio strategy: AAC passthrough, non-AAC transcoding, layout normalization.
package planner

import (
	"fmt"
	"strings"

	"github.com/backmassage/muxmaster/internal/config"
	"github.com/backmassage/muxmaster/internal/probe"
)

// BuildAudioPlan produces the audio handling strategy for a file.
//
//   - No audio streams → NoAudio (produces -an).
//   - All streams are AAC → CopyAll (produces -map 0:a -c:a copy).
//     AAC is already the target codec for Jellyfin direct play; re-encoding
//     it at any bitrate is lossy-to-lossy with no compatibility benefit.
//   - Otherwise → per-stream plan: copy all AAC streams, transcode
//     non-AAC to AAC with optional MATCH_AUDIO_LAYOUT filter chains.
func BuildAudioPlan(cfg *config.Config, pr *probe.ProbeResult) AudioPlan {
	if len(pr.AudioStreams) == 0 {
		return AudioPlan{NoAudio: true}
	}

	copyAll := true
	for _, a := range pr.AudioStreams {
		if !strings.EqualFold(a.Codec, "aac") {
			copyAll = false
			break
		}
	}
	if copyAll {
		return AudioPlan{CopyAll: true}
	}

	var streams []AudioStreamPlan
	for i, a := range pr.AudioStreams {
		asp := AudioStreamPlan{
			StreamIndex: i,
			Channels:    clampChannels(a.Channels, cfg.Audio.Channels),
			Bitrate:     cfg.Audio.Bitrate,
			SampleRate:  cfg.Audio.SampleRate,
		}

		if strings.EqualFold(a.Codec, "aac") {
			asp.Copy = true
			streams = append(streams, asp)
			continue
		}

		if cfg.Audio.MatchLayout {
			asp.NeedsFilter = true
			asp.FilterStr = buildAudioFilterWithRate(&a, asp.Channels, cfg.Audio.SampleRate)
			asp.Layout = layoutForChannels(asp.Channels)
		}

		streams = append(streams, asp)
	}
	return AudioPlan{Streams: streams}
}

func clampChannels(source, max int) int {
	if source < 1 {
		// Unknown channel count: target the configured cap rather than
		// forcing a destructive downmix to mono.
		return max
	}
	if source > max {
		return max
	}
	return source
}

// buildAudioFilterWithRate constructs the audio filter chain used when
// MATCH_AUDIO_LAYOUT is enabled: an optional dialog-forward pan downmix
// followed by the aresample+aformat chain at the configured sample rate.
func buildAudioFilterWithRate(src *probe.AudioStream, channels, sampleRate int) string {
	base := fmt.Sprintf("aresample=async=1:first_pts=0:min_hard_comp=0.100,aformat=sample_rates=%d", sampleRate)
	layout := layoutForChannels(channels)
	if layout != "" {
		base = fmt.Sprintf("%s:channel_layouts=%s", base, layout)
	}
	if pan := downmixSpec(src, channels); pan != "" {
		return pan + "," + base
	}
	return base
}

// downmixSpecs maps probed multichannel layouts to dialog-forward stereo pan
// specs. The `<` gain syntax renormalizes the weights so the sum cannot clip,
// removing any need for a limiter. Center is weighted at full gain relative
// to 0.6 fronts/surrounds and 0.3 LFE — a mix between Jellyfin's "Dave750"
// (dialog too quiet) and "NightmodeDialogue" (effects crushed). A pan spec
// naming channels absent from the input layout errors out, so only exact
// layout matches are downmixed; everything else falls back to plain -ac 2.
var downmixSpecs = map[string]string{
	"5.1": "pan=stereo|FL<FC+0.60*FL+0.60*BL+0.30*LFE|FR<FC+0.60*FR+0.60*BR+0.30*LFE",
	"5.1(side)": "pan=stereo|FL<FC+0.60*FL+0.60*SL+0.30*LFE|" +
		"FR<FC+0.60*FR+0.60*SR+0.30*LFE",
	"7.1": "pan=stereo|FL<FC+0.60*FL+0.60*BL+0.60*SL+0.30*LFE|" +
		"FR<FC+0.60*FR+0.60*BR+0.60*SR+0.30*LFE",
}

// downmixSpec returns the pan filter for a multichannel→stereo transcode,
// or "" when no downmix applies (already ≤2ch target mismatch, or a layout
// we have no verified spec for).
func downmixSpec(src *probe.AudioStream, targetChannels int) string {
	if targetChannels != 2 || src.Channels <= 2 {
		return ""
	}
	return downmixSpecs[src.ChannelLayout]
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
