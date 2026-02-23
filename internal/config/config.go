// Package config holds runtime configuration: defaults, CLI flag parsing, and validation.
// All defaults match the legacy shell script (v1.7.0) for parity.
package config

import (
	"errors"
	"path/filepath"
	"strings"
)

// EncoderMode selects the encoding backend: VAAPI (hardware) or CPU (libx265).
type EncoderMode string

const (
	EncoderVAAPI EncoderMode = "vaapi"
	EncoderCPU   EncoderMode = "cpu"
)

// Container is the output container format; MKV is primary, MP4 for compatibility.
type Container string

const (
	ContainerMKV Container = "mkv"
	ContainerMP4 Container = "mp4"
)

// HDRMode controls HDR handling: preserve metadata or tonemap to SDR.
type HDRMode string

const (
	HDRPreserve HDRMode = "preserve"
	HDRTonemap  HDRMode = "tonemap"
)

// ColorMode controls ANSI colors: auto (tty only), always, or never.
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

// DefaultConfig returns a Config with all defaults set.
// Used as the base before ParseFlags; matches legacy Muxmaster.sh v1.7.0 behavior.
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

// NormalizeDirArg strips trailing slashes from a directory path.
// The root path "/" is returned unchanged so we don't produce an empty string.
func NormalizeDirArg(path string) string {
	if path == "/" {
		return "/"
	}
	return strings.TrimRight(path, "/")
}

// Validate checks that enum fields (mode, container, HDR) are valid and,
// when not in CheckOnly mode, that both input and output directories were given.
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

// ValidatePaths ensures the output directory is not inside the input directory
// (to avoid recursive processing). Call with absolute paths after resolving symlinks.
func (c *Config) ValidatePaths(inputAbs, outputAbs string) error {
	if outputAbs == inputAbs || strings.HasPrefix(outputAbs+string(filepath.Separator), inputAbs+string(filepath.Separator)) {
		return errors.New("output directory must not be inside input directory")
	}
	return nil
}
