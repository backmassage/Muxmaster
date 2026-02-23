package config

import (
	"errors"
	"path/filepath"
	"strings"
)

// EncoderMode is vaapi or cpu.
type EncoderMode string

const (
	EncoderVAAPI EncoderMode = "vaapi"
	EncoderCPU   EncoderMode = "cpu"
)

// Container is mkv or mp4.
type Container string

const (
	ContainerMKV Container = "mkv"
	ContainerMP4 Container = "mp4"
)

// HDRMode is preserve or tonemap.
type HDRMode string

const (
	HDRPreserve HDRMode = "preserve"
	HDRTonemap  HDRMode = "tonemap"
)

// ColorMode controls colored output.
type ColorMode string

const (
	ColorAuto   ColorMode = "auto"
	ColorAlways ColorMode = "always"
	ColorNever  ColorMode = "never"
)

// Config holds all runtime configuration (defaults + parsed flags).
type Config struct {
	// Paths
	InputDir  string
	OutputDir string

	// Encoder
	EncoderMode      EncoderMode
	VaapiDevice      string // default: "/dev/dri/renderD128"
	VaapiQP          int    // default: 19
	VaapiProfile     string // derived at runtime: "main10" or "main"
	VaapiSwFormat    string // derived at runtime: "p010" or "nv12"
	CpuCRF           int    // default: 19
	CpuPreset        string // default: "slow"
	CpuProfile       string // fixed: "main10"
	CpuPixFmt        string // fixed: "yuv420p10le"
	KeyframeInterval int    // fixed: 48

	// Output
	OutputContainer Container // default: "mkv"

	// Audio
	AudioChannels   int    // default: 2
	AudioBitrate    string // default: "256k"
	AudioSampleRate int    // fixed: 48000

	// Behavior
	DryRun           bool
	SkipExisting     bool
	SkipHEVC         bool
	StrictMode       bool
	SmartQuality     bool
	CleanTimestamps  bool
	MatchAudioLayout bool
	KeepSubtitles    bool
	KeepAttachments  bool
	HandleHDR        HDRMode
	DeinterlaceAuto  bool

	// Quality tuning
	SmartQualityBias      int // default: -1
	SmartQualityRetryStep int // default: 2

	// Display
	Verbose       bool
	ShowFileStats bool
	ShowFfmpegFPS bool
	ColorMode     ColorMode
	LogFile       string
	CheckOnly     bool

	// Quality overrides (set during flag parsing)
	QualityOverride      string
	CpuCRFFixedOverride  string
	VaapiQPFixedOverride string
	ActiveQualityOverride string // derived when manual override applies to active mode

	// ffmpeg constants (not user-configurable)
	FFmpegProbesize       string
	FFmpegAnalyzeDuration string
}

// DefaultConfig returns Config with all defaults set (shell v1.7.0 parity).
func DefaultConfig() Config {
	return Config{
		EncoderMode:      EncoderVAAPI,
		VaapiDevice:     "/dev/dri/renderD128",
		VaapiQP:         19,
		CpuCRF:          19,
		CpuPreset:       "slow",
		CpuProfile:      "main10",
		CpuPixFmt:       "yuv420p10le",
		KeyframeInterval: 48,
		OutputContainer:  ContainerMKV,
		AudioChannels:   2,
		AudioBitrate:    "256k",
		AudioSampleRate: 48000,
		DryRun:          false,
		SkipExisting:    true,
		SkipHEVC:        true,
		StrictMode:      false,
		SmartQuality:    true,
		CleanTimestamps: true,
		MatchAudioLayout: true,
		KeepSubtitles:   true,
		KeepAttachments: true,
		HandleHDR:       HDRPreserve,
		DeinterlaceAuto: true,
		SmartQualityBias:      -1,
		SmartQualityRetryStep: 2,
		Verbose:        false,
		ShowFileStats:  true,
		ShowFfmpegFPS:  true,
		ColorMode:      ColorAuto,
		CheckOnly:      false,
		FFmpegProbesize:       "100M",
		FFmpegAnalyzeDuration: "100M",
	}
}

// NormalizeDirArg strips trailing slashes from a path (except for "/").
func NormalizeDirArg(path string) string {
	if path == "/" {
		return "/"
	}
	return strings.TrimRight(path, "/")
}

// Validate checks mode, container, HDR, quality overrides, and (if !CheckOnly) paths.
func (c *Config) Validate() error {
	switch c.EncoderMode {
	case EncoderVAAPI, EncoderCPU:
	default:
		return errors.New("invalid mode (use 'vaapi' or 'cpu')")
	}
	switch c.OutputContainer {
	case ContainerMKV, ContainerMP4:
	default:
		return errors.New("invalid container (use 'mkv' or 'mp4')")
	}
	switch c.HandleHDR {
	case HDRPreserve, HDRTonemap:
	default:
		return errors.New("invalid HDR mode (use 'preserve' or 'tonemap')")
	}
	if c.CheckOnly {
		return nil
	}
	if c.InputDir == "" || c.OutputDir == "" {
		return errors.New("need exactly input_dir and output_dir")
	}
	return nil
}

// ValidatePaths checks that input exists, output can be created, and output is not inside input.
func (c *Config) ValidatePaths(inputAbs, outputAbs string) error {
	if outputAbs == inputAbs || strings.HasPrefix(outputAbs+string(filepath.Separator), inputAbs+string(filepath.Separator)) {
		return errors.New("output directory must not be inside input directory")
	}
	return nil
}
