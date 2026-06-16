// FilePlan, Action, AudioPlan, SubtitlePlan, and other planner domain types.
package planner

import "github.com/backmassage/muxmaster/internal/config"

// Action describes the per-file processing decision.
type Action int

const (
	ActionEncode Action = iota
	ActionRemux
	ActionSkip // File cannot be processed correctly (e.g. Dolby Vision profile 5).
)

// FilePlan holds the complete set of decisions for processing a single media
// file. It is produced by BuildPlan and consumed by the ffmpeg package to
// construct command arguments and by the retry engine for initial state.
type FilePlan struct {
	Action     Action
	SkipReason string   // Why the file is skipped (Action == ActionSkip).
	Notes      []string // One-line informational notes logged before processing.

	// Video encoding.
	VideoCodec       string   // "hevc_vaapi", "libx265", or "copy"
	VideoFilters     string   // comma-joined filter chain (may be empty)
	SWVideoFilters   string   // software-decode fallback chain (set only when HWDecode is true)
	CPUVideoFilters  string   // pure-CPU fallback chain, no hwupload (set in VAAPI mode for the RetryFallbackCPU path)
	ColorOpts        []string // -color_trc, -color_primaries, -colorspace pairs
	BSFOpts          []string // bitstream filter args (e.g. -bsf:v dovi_rpu=strip=1 on remux)
	HWDecode         bool     // Use VAAPI hardware decode (frames stay on GPU)
	VaapiQVBR        bool     // Use QVBR rate control instead of constant-QP (capability-gated)
	VaapiBFrames     bool     // Enable B-frames on the VAAPI encoder (capability-gated)
	KeyframeInterval int      // Per-file GOP length (~2s at source fps); 0 = use config default

	// HDR10 static metadata (empty when not present or not preserving HDR).
	MasterDisplay string // ffmpeg format: G(gx,gy)B(bx,by)R(rx,ry)WP(wpx,wpy)L(maxL,minL)
	MaxCLL        string // ffmpeg format: MaxCLL,MaxFALL

	// Quality (resolved per-file by smart quality).
	VaapiQP            int
	CpuCRF             int
	QualityNote        string
	Estimate           BitrateEstimate // Pre-encode output size prediction.
	PreflightBumps     int             // How many QP/CRF bumps the pre-flight check applied.
	MaxRateKbps        int             // Hard bitrate ceiling for CPU encodes (0 = no cap).
	BufSizeKbps        int             // VBV buffer size (typically 2× maxrate).
	OptimalBitrateKbps int             // Estimated target output bitrate based on input analysis.

	// Audio.
	Audio AudioPlan

	// Subtitles and attachments.
	Subtitles   SubtitlePlan
	Attachments AttachmentPlan

	// Cover-art video streams to carry over (MKV only; absolute indices).
	AttachedPicIdxs []int

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
	InputPath        string
	OutputPath       string
	Container        config.Container
	VideoStreamIdx   int
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
	Bitrate     string // e.g. "320k"
	SampleRate  int    // e.g. 48000
	Layout      string // "mono", "stereo", or "" (passthrough)
	NeedsFilter bool
	FilterStr   string // precomputed aresample/aformat chain
}

// SubtitlePlan describes how subtitles are handled.
type SubtitlePlan struct {
	Include    bool
	Codec      string // "copy", "mov_text", or "" (per-stream codecs in use)
	SkipBitmap bool   // When true, only text subtitle streams are mapped (MP4 with mixed subs).
	TextIdxs   []int  // Absolute stream indices of mapped subtitle streams (indexed-map modes).

	// StreamCodecs holds per-output-stream subtitle codecs parallel to
	// TextIdxs (MKV with mov_text sources that need srt conversion).
	// Empty means Codec applies to all mapped streams.
	StreamCodecs []string
}

// AttachmentPlan describes whether to carry attachments (fonts, etc.).
type AttachmentPlan struct {
	Include bool
}
