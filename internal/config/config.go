// Package config holds runtime configuration: defaults, CLI flag parsing, and
// validation. All defaults match the legacy shell script (v1.7.0) for parity.
package config

import (
	"errors"
	"fmt"
	"path/filepath"
	"strconv"
	"strings"
)

// --- Enum types for validated string fields ---

// EncoderMode selects the encoding backend.
type EncoderMode string

const (
	EncoderVAAPI EncoderMode = "vaapi" // Hardware encoding via VAAPI (default).
	EncoderCPU   EncoderMode = "cpu"   // Software encoding via libx265.
)

// Container is the output container format.
type Container string

const (
	ContainerMKV Container = "mkv" // Matroska (default, full feature support).
	ContainerMP4 Container = "mp4" // MP4 (compatibility; limited subtitle support).
)

// HDRMode controls HDR handling during encoding.
type HDRMode string

const (
	HDRPreserve HDRMode = "preserve" // Keep HDR metadata (default).
	HDRTonemap  HDRMode = "tonemap"  // Tonemap to SDR.
)

// ColorMode controls ANSI color output.
type ColorMode string

const (
	ColorAuto   ColorMode = "auto"   // Enable colors when stdout is a TTY (default).
	ColorAlways ColorMode = "always" // Force colors on.
	ColorNever  ColorMode = "never"  // Disable colors entirely.
)

// Config holds all runtime settings. It is populated by [DefaultConfig] and
// then mutated by [ParseFlags] before being passed (by pointer) to packages
// that need it. Fields are grouped by concern with inline documentation of
// defaults and fixed values.
type Config struct {
	// Paths (set from positional args).
	InputDir  string
	OutputDir string

	// Encoder settings.
	EncoderMode      EncoderMode
	VaapiDevice      string // Default: "/dev/dri/renderD128".
	VaapiQP          int    // Default: 19. Overridden by --vaapi-qp or --quality.
	VaapiProfile     string // Derived at runtime: "main10" or "main".
	VaapiSwFormat    string // Derived at runtime: "p010" or "nv12".
	CpuCRF           int    // Default: 19. Overridden by --cpu-crf or --quality.
	CpuPreset        string // Default: "slow".
	CpuProfile       string // Fixed: "main10".
	CpuPixFmt        string // Fixed: "yuv420p10le".
	KeyframeInterval int    // Fixed: 48 frames.

	// Output format.
	OutputContainer Container // Default: "mkv".

	// Audio encoding.
	AudioChannels   int    // Default: 2 (stereo).
	AudioBitrate    string // Default: "256k".
	AudioSampleRate int    // Fixed: 48000 Hz.
	AudioEncoder    string // Fixed default: "libfdk_aac".

	// Behavior flags.
	DryRun           bool
	SkipExisting     bool    // Default: true. Cleared by --force.
	SkipHEVC         bool    // Default: true. Cleared by --no-skip-hevc.
	StrictMode       bool    // Disable retry fallbacks.
	SmartQuality     bool    // Default: true. Per-file quality adaptation.
	CleanTimestamps  bool    // Default: true. Regenerate timestamps.
	MatchAudioLayout bool    // Default: true. Normalize audio channel layout.
	KeepSubtitles    bool    // Default: true.
	KeepAttachments  bool    // Default: true.
	HandleHDR        HDRMode // Default: "preserve".
	DeinterlaceAuto  bool    // Default: true.

	// Quality tuning.
	SmartQualityBias      int // Default: -1 (slightly favor smaller output).
	SmartQualityRetryStep int // Default: 2 (QP/CRF bump per retry).

	// Display and logging.
	Verbose       bool
	ShowFileStats bool      // Default: true.
	ShowFfmpegFPS bool      // Default: true.
	ColorMode     ColorMode // Default: "auto".
	LogFile       string    // Optional log file path.
	CheckOnly     bool      // Run --check diagnostics and exit.

	// Quality overrides (populated during flag parsing).
	QualityOverride       string // --quality value (applies to active mode).
	CpuCRFFixedOverride   string // --cpu-crf value.
	VaapiQPFixedOverride  string // --vaapi-qp value.
	ActiveQualityOverride string // Derived: the override that applies to the active encoder mode.

	// ffmpeg probe constants (not user-configurable).
	FFmpegProbesize       string
	FFmpegAnalyzeDuration string
}

// DefaultConfig returns a Config with all defaults matching legacy Muxmaster.sh
// v1.7.0 behavior. Used as the base before [ParseFlags] applies CLI overrides.
func DefaultConfig() Config {
	return Config{
		EncoderMode:           EncoderVAAPI,
		VaapiDevice:           "/dev/dri/renderD128",
		VaapiQP:               19,
		CpuCRF:                19,
		CpuPreset:             "slow",
		CpuProfile:            "main10",
		CpuPixFmt:             "yuv420p10le",
		KeyframeInterval:      48,
		OutputContainer:       ContainerMKV,
		AudioChannels:         2,
		AudioBitrate:          "256k",
		AudioSampleRate:       48000,
		AudioEncoder:          "libfdk_aac",
		DryRun:                false,
		SkipExisting:          true,
		SkipHEVC:              true,
		StrictMode:            false,
		SmartQuality:          true,
		CleanTimestamps:       true,
		MatchAudioLayout:      true,
		KeepSubtitles:         true,
		KeepAttachments:       true,
		HandleHDR:             HDRPreserve,
		DeinterlaceAuto:       true,
		SmartQualityBias:      -1,
		SmartQualityRetryStep: 2,
		Verbose:               false,
		ShowFileStats:         true,
		ShowFfmpegFPS:         true,
		ColorMode:             ColorAuto,
		CheckOnly:             false,
		FFmpegProbesize:       "100M",
		FFmpegAnalyzeDuration: "100M",
	}
}

// NormalizeDirArg strips trailing slashes from a directory path.
// The filesystem root "/" is returned unchanged so we don't produce an empty string.
func NormalizeDirArg(path string) string {
	if path == "/" {
		return "/"
	}
	return strings.TrimRight(path, "/")
}

// Validate checks that enum fields (mode, container, HDR) hold valid values.
// When not in CheckOnly mode, it also requires that both input and output
// directory paths are non-empty.
func (c *Config) Validate() error {
	switch c.EncoderMode {
	case EncoderVAAPI, EncoderCPU:
		// valid
	default:
		return errors.New("invalid mode (use 'vaapi' or 'cpu')")
	}

	switch c.OutputContainer {
	case ContainerMKV, ContainerMP4:
		// valid
	default:
		return errors.New("invalid container (use 'mkv' or 'mp4')")
	}

	switch c.HandleHDR {
	case HDRPreserve, HDRTonemap:
		// valid
	default:
		return errors.New("invalid HDR mode (use 'preserve' or 'tonemap')")
	}
	normalizedBitrate, err := normalizeAudioBitrate(c.AudioBitrate)
	if err != nil {
		return err
	}
	c.AudioBitrate = normalizedBitrate

	if c.CheckOnly {
		return nil
	}
	if c.InputDir == "" || c.OutputDir == "" {
		return errors.New("need exactly input_dir and output_dir")
	}
	return nil
}

// normalizeAudioBitrate validates and canonicalizes user bitrate input.
// Accepted forms: "256", "256k", "256K", "256kbps". Output is "<n>k".
func normalizeAudioBitrate(raw string) (string, error) {
	s := strings.ToLower(strings.TrimSpace(raw))
	if s == "" {
		return "", errors.New("audio bitrate must not be empty")
	}
	if strings.HasSuffix(s, "kbps") {
		s = strings.TrimSpace(strings.TrimSuffix(s, "kbps"))
	} else if strings.HasSuffix(s, "k") {
		s = strings.TrimSpace(strings.TrimSuffix(s, "k"))
	}
	n, err := strconv.Atoi(s)
	if err != nil || n <= 0 {
		return "", fmt.Errorf("invalid audio bitrate %q (use positive Kbps value, e.g. 128k)", raw)
	}
	return fmt.Sprintf("%dk", n), nil
}

// ValidatePaths ensures the resolved output directory is not inside (or equal
// to) the resolved input directory. This prevents the pipeline from
// recursively discovering its own output files. Both arguments must be
// absolute, symlink-resolved paths.
func (c *Config) ValidatePaths(inputAbs, outputAbs string) error {
	sep := string(filepath.Separator)
	if outputAbs == inputAbs || strings.HasPrefix(outputAbs+sep, inputAbs+sep) {
		return errors.New("output directory must not be inside input directory")
	}
	return nil
}
