package planner

import "github.com/backmassage/muxmaster/internal/config"

// Action describes the per-file processing decision.
type Action int

const (
	ActionEncode Action = iota
	ActionRemux
	ActionSkip
)

// FilePlan holds the complete set of decisions for processing a single media
// file. It is produced by BuildPlan and consumed by the ffmpeg package to
// construct command arguments and by the retry engine for initial state.
type FilePlan struct {
	Action     Action
	SkipReason string

	// Video encoding.
	VideoCodec   string   // "hevc_vaapi", "libx265", or "copy"
	VideoFilters string   // comma-joined filter chain (may be empty)
	ColorOpts    []string // -color_trc, -color_primaries, -colorspace pairs

	// Quality (resolved per-file by smart quality).
	VaapiQP     int
	CpuCRF      int
	QualityNote string

	// Audio.
	Audio AudioPlan

	// Subtitles and attachments.
	Subtitles   SubtitlePlan
	Attachments AttachmentPlan

	// Stream dispositions.
	DispositionOpts []string

	// Container-specific flags.
	ContainerOpts []string // e.g. -movflags +faststart
	TagOpts       []string // e.g. -tag:v hvc1

	// Retry initial state (seeded from config and probe data).
	MuxQueueSize  int
	TimestampFix  bool
	IncludeSubs   bool
	IncludeAttach bool

	// Output.
	InputPath       string
	OutputPath      string
	Container       config.Container
	VideoStreamIdx  int
	AudioStreamCount int
}

// AudioPlan describes the audio handling strategy for a file.
type AudioPlan struct {
	NoAudio bool
	CopyAll bool
	Streams []AudioStreamPlan
}

// AudioStreamPlan describes the processing for one audio stream.
type AudioStreamPlan struct {
	StreamIndex int
	Copy        bool   // true for AAC passthrough
	Channels    int    // target channel count (capped at Config.AudioChannels)
	Bitrate     string // e.g. "256k"
	SampleRate  int    // e.g. 48000
	Layout      string // "mono", "stereo", or "" (passthrough)
	NeedsFilter bool
	FilterStr   string // precomputed aresample/aformat chain
}

// SubtitlePlan describes how subtitles are handled.
type SubtitlePlan struct {
	Include    bool
	Codec      string // "copy", "mov_text", or ""
	SkipBitmap bool   // When true, only text subtitle streams are mapped (MP4 with mixed subs).
	TextIdxs   []int  // Absolute stream indices of text subtitle streams (used when SkipBitmap is true).
}

// AttachmentPlan describes whether to carry attachments (fonts, etc.).
type AttachmentPlan struct {
	Include bool
}
