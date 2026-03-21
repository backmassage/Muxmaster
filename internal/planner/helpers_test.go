// helpers_test.go provides probe-result builders and test utilities for planner tests.
package planner

import (
	"github.com/backmassage/muxmaster/internal/config"
	"github.com/backmassage/muxmaster/internal/probe"
)

func defaultCfg() *config.Config {
	cfg := config.DefaultConfig()
	return &cfg
}

func h264SDR() *probe.ProbeResult {
	return &probe.ProbeResult{
		PrimaryVideo: &probe.VideoStream{
			Codec: "h264", Profile: "High", PixFmt: "yuv420p",
			Width: 1920, Height: 1080, BitRate: 8000000,
			FieldOrder: "progressive",
		},
		AudioStreams:    []probe.AudioStream{{Codec: "ac3", Channels: 6, SampleRate: 48000}},
		SubtitleStreams: []probe.SubtitleStream{{Codec: "ass", Language: "eng"}},
		Format:          probe.FormatInfo{BitRate: 9000000},
	}
}

func hevcEdgeSafe() *probe.ProbeResult {
	return &probe.ProbeResult{
		PrimaryVideo: &probe.VideoStream{
			Codec: "hevc", Profile: "Main 10", PixFmt: "yuv420p10le",
			Width: 1920, Height: 1080, BitRate: 5000000,
		},
		AudioStreams: []probe.AudioStream{{Codec: "aac", Channels: 2, SampleRate: 48000}},
		Format:       probe.FormatInfo{BitRate: 6000000},
	}
}

func hevcUnsafe() *probe.ProbeResult {
	return &probe.ProbeResult{
		PrimaryVideo: &probe.VideoStream{
			Codec: "hevc", Profile: "Rext", PixFmt: "yuv444p10le",
			Width: 1920, Height: 1080, BitRate: 5000000,
		},
		AudioStreams: []probe.AudioStream{{Codec: "flac", Channels: 2, SampleRate: 48000}},
		Format:       probe.FormatInfo{BitRate: 6000000},
	}
}

func hdr10File() *probe.ProbeResult {
	return &probe.ProbeResult{
		PrimaryVideo: &probe.VideoStream{
			Codec: "hevc", Profile: "Main 10", PixFmt: "yuv420p10le",
			Width: 3840, Height: 2160, BitRate: 30000000,
			ColorTransfer: "smpte2084", ColorPrimaries: "bt2020", ColorSpace: "bt2020nc",
			FieldOrder: "progressive",
		},
		AudioStreams: []probe.AudioStream{{Codec: "eac3", Channels: 6, SampleRate: 48000}},
		Format:       probe.FormatInfo{BitRate: 35000000},
	}
}

func interlacedFile() *probe.ProbeResult {
	return &probe.ProbeResult{
		PrimaryVideo: &probe.VideoStream{
			Codec: "mpeg2video", Profile: "Main", PixFmt: "yuv420p",
			Width: 720, Height: 480, BitRate: 3500000,
			FieldOrder: "tt",
		},
		AudioStreams: []probe.AudioStream{{Codec: "mp2", Channels: 2, SampleRate: 48000}},
		Format:       probe.FormatInfo{BitRate: 4000000},
	}
}

func safeEstLow(est BitrateEstimate) int {
	if !est.Known {
		return 0
	}
	return est.LowPct
}

func safeEstHigh(est BitrateEstimate) int {
	if !est.Known {
		return 0
	}
	return est.HighPct
}
