package config

// This file implements CLI flag parsing and help text.
// Flags are grouped into encoding, container/HDR, behavior, display, and utility.
// Negated flags (e.g. --no-skip-hevc) are applied after Parse so Config defaults hold unless set.

import (
	"flag"
	"fmt"
	"os"
	"strconv"
	"strings"
)

// ParseFlags parses os.Args into cfg. On --help or --version it prints and exits.
// On error it returns non-nil (e.g. unknown flag, missing positional args).
// The version parameter is passed from main so the help text reflects the build-time version.
func ParseFlags(cfg *Config, version string) error {
	fs := flag.NewFlagSet("muxmaster", flag.ContinueOnError)
	fs.Usage = func() { printUsage(fs, version) }

	// Negated/override flags: we capture bools then apply to cfg after Parse,
	// so that defaults from DefaultConfig() hold unless the user passes the flag.
	var negated negatedFlags

	defineEncodingFlags(fs, cfg)
	defineContainerAndHDRFlags(fs, cfg, &negated)
	defineBehaviorFlags(fs, cfg, &negated)
	defineDisplayFlags(fs, cfg, &negated)
	defineUtilityFlags(fs, cfg, &negated)

	if err := fs.Parse(os.Args[1:]); err != nil {
		return err
	}

	applyNegatedFlags(cfg, &negated)

	if negated.showHelp {
		printUsage(fs, version)
		os.Exit(0)
	}
	if negated.showVersion {
		fmt.Fprintln(os.Stdout, "muxmaster v"+version)
		os.Exit(0)
	}

	if err := parsePositionalArgs(fs, cfg); err != nil {
		return err
	}
	return applyQualityPrecedence(cfg)
}

// negatedFlags holds boolean flags that are applied after Parse.
// These either invert a default (e.g. noSkipHEVC -> SkipHEVC=false) or trigger exit (showHelp, showVersion).
type negatedFlags struct {
	noDeinterlace     bool
	noSkipHEVC        bool
	noFps             bool
	noStats           bool
	noSubs            bool
	noAttachments     bool
	noSmartQuality    bool
	noCleanTimestamps bool
	noMatchLayout     bool
	force             bool
	forceColor        bool
	noColor           bool
	showVersion       bool
	showHelp          bool
}

// defineEncodingFlags registers -m/--mode, -q/--quality, --cpu-crf, --vaapi-qp, -p/--preset, --audio-bitrate.
func defineEncodingFlags(fs *flag.FlagSet, cfg *Config) {
	fs.Var(&encoderModeValue{&cfg.EncoderMode}, "mode", "Encoder mode: vaapi | cpu")
	fs.Var(&encoderModeValue{&cfg.EncoderMode}, "m", "Same as --mode")
	fs.StringVar(&cfg.QualityOverride, "quality", "", "Fixed quality for active mode (QP or CRF)")
	fs.StringVar(&cfg.QualityOverride, "q", "", "Same as --quality")
	fs.StringVar(&cfg.CpuCRFFixedOverride, "cpu-crf", "", "Fixed CPU CRF (overrides --quality in CPU mode)")
	fs.StringVar(&cfg.VaapiQPFixedOverride, "vaapi-qp", "", "Fixed VAAPI QP (overrides --quality in VAAPI mode)")
	fs.StringVar(&cfg.CpuPreset, "preset", cfg.CpuPreset, "x265 preset (e.g. slow, medium)")
	fs.StringVar(&cfg.CpuPreset, "p", cfg.CpuPreset, "Same as --preset")
	fs.StringVar(&cfg.AudioBitrate, "audio-bitrate", cfg.AudioBitrate, "Audio bitrate in Kbps (e.g. 128k, 256k)")
}

// defineContainerAndHDRFlags registers --container, --hdr, --no-deinterlace.
func defineContainerAndHDRFlags(fs *flag.FlagSet, cfg *Config, n *negatedFlags) {
	fs.Var(&containerValue{&cfg.OutputContainer}, "container", "Output container: mkv | mp4")
	fs.Var(&hdrModeValue{&cfg.HandleHDR}, "hdr", "HDR handling: preserve | tonemap")
	fs.BoolVar(&n.noDeinterlace, "no-deinterlace", false, "Disable automatic deinterlace")
}

// defineBehaviorFlags registers dry-run, skip-hevc, subs, attachments, strict, quality, timestamps, force.
func defineBehaviorFlags(fs *flag.FlagSet, cfg *Config, n *negatedFlags) {
	fs.BoolVar(&cfg.DryRun, "dry-run", false, "Preview only; do not encode or remux")
	fs.BoolVar(&cfg.DryRun, "d", false, "Same as --dry-run")
	fs.BoolVar(&n.noSkipHEVC, "no-skip-hevc", false, "Re-encode HEVC instead of remuxing")
	fs.BoolVar(&cfg.SmartQuality, "smart-quality", cfg.SmartQuality, "Per-file quality adaptation")
	fs.BoolVar(&cfg.CleanTimestamps, "clean-timestamps", cfg.CleanTimestamps, "Regenerate timestamps")
	fs.BoolVar(&cfg.MatchAudioLayout, "match-audio-layout", cfg.MatchAudioLayout, "Normalize audio channel layout")
	fs.BoolVar(&n.noFps, "no-fps", false, "Do not show live ffmpeg FPS")
	fs.BoolVar(&n.noStats, "no-stats", false, "Hide per-file source stats")
	fs.BoolVar(&n.noSubs, "no-subs", false, "Do not process subtitle streams")
	fs.BoolVar(&n.noAttachments, "no-attachments", false, "Do not include attachments")
	fs.BoolVar(&cfg.StrictMode, "strict", false, "Disable automatic ffmpeg retry fallbacks")
	fs.BoolVar(&n.noSmartQuality, "no-smart-quality", false, "Use fixed quality only (no per-file adaptation)")
	fs.BoolVar(&n.noCleanTimestamps, "no-clean-timestamps", false, "Disable timestamp regeneration")
	fs.BoolVar(&n.noMatchLayout, "no-match-audio-layout", false, "Disable audio layout normalization")
	fs.BoolVar(&n.force, "force", false, "Overwrite existing output files")
	fs.BoolVar(&n.force, "f", false, "Same as --force")
}

// defineDisplayFlags registers color, verbose, log, and --check flags.
// Note: --check is a utility flag conceptually, but is registered here alongside
// --verbose and --log because it controls what the program outputs rather than
// how it encodes.
func defineDisplayFlags(fs *flag.FlagSet, cfg *Config, n *negatedFlags) {
	fs.BoolVar(&n.forceColor, "color", false, "Force colored logs")
	fs.BoolVar(&n.noColor, "no-color", false, "Disable colored logs")
	fs.BoolVar(&cfg.ShowFfmpegFPS, "show-fps", cfg.ShowFfmpegFPS, "Show live ffmpeg FPS")
	fs.BoolVar(&cfg.Verbose, "verbose", false, "Verbose output")
	fs.BoolVar(&cfg.Verbose, "v", false, "Same as --verbose")
	fs.BoolVar(&cfg.CheckOnly, "check", false, "Run system diagnostics and exit")
	fs.BoolVar(&cfg.CheckOnly, "c", false, "Same as --check")
	fs.BoolVar(&cfg.AnalyzeOnly, "analyze", false, "Probe all files and print codec/bitrate table")
	fs.BoolVar(&cfg.AnalyzeOnly, "a", false, "Same as --analyze")
	fs.StringVar(&cfg.LogFile, "log", "", "Append logs to file")
	fs.StringVar(&cfg.LogFile, "l", "", "Same as --log")
}

// defineUtilityFlags registers --version and --help (both cause exit after printing).
// The cfg parameter is unused but kept for signature consistency with other define* functions.
func defineUtilityFlags(fs *flag.FlagSet, _ *Config, n *negatedFlags) {
	fs.BoolVar(&n.showVersion, "version", false, "Print version and exit")
	fs.BoolVar(&n.showVersion, "V", false, "Same as --version")
	fs.BoolVar(&n.showHelp, "help", false, "Show this help and exit")
	fs.BoolVar(&n.showHelp, "h", false, "Same as --help")
}

// applyNegatedFlags copies negated and override flag values into cfg (e.g. noFps -> ShowFfmpegFPS=false).
func applyNegatedFlags(cfg *Config, n *negatedFlags) {
	if n.noDeinterlace {
		cfg.DeinterlaceAuto = false
	}
	if n.noSkipHEVC {
		cfg.SkipHEVC = false
	}
	if n.noFps {
		cfg.ShowFfmpegFPS = false
	}
	if n.noStats {
		cfg.ShowFileStats = false
	}
	if n.noSubs {
		cfg.KeepSubtitles = false
	}
	if n.noAttachments {
		cfg.KeepAttachments = false
	}
	if n.noSmartQuality {
		cfg.SmartQuality = false
	}
	if n.noCleanTimestamps {
		cfg.CleanTimestamps = false
	}
	if n.noMatchLayout {
		cfg.MatchAudioLayout = false
	}
	if n.force {
		cfg.SkipExisting = false
	}
	if n.noColor {
		cfg.ColorMode = ColorNever
	} else if n.forceColor {
		cfg.ColorMode = ColorAlways
	}
}

// parsePositionalArgs sets InputDir and OutputDir from the two positional args when not in CheckOnly mode.
func parsePositionalArgs(fs *flag.FlagSet, cfg *Config) error {
	args := fs.Args()
	if cfg.CheckOnly {
		return nil
	}
	if cfg.AnalyzeOnly {
		if len(args) < 1 {
			return fmt.Errorf("--analyze requires an input directory")
		}
		cfg.InputDir = NormalizeDirArg(args[0])
		return nil
	}
	if len(args) != 2 {
		return fmt.Errorf("need exactly input_dir and output_dir")
	}
	cfg.InputDir = NormalizeDirArg(args[0])
	cfg.OutputDir = NormalizeDirArg(args[1])
	return nil
}

// applyQualityPrecedence sets VaapiQP/CpuCRF and ActiveQualityOverride.
// Precedence: mode-specific override (--vaapi-qp / --cpu-crf) > --quality > defaults.
func applyQualityPrecedence(cfg *Config) error {
	cfg.ActiveQualityOverride = ""
	if cfg.EncoderMode == EncoderVAAPI {
		if cfg.VaapiQPFixedOverride != "" {
			q, err := parseInt(cfg.VaapiQPFixedOverride, "VAAPI QP")
			if err != nil {
				return err
			}
			cfg.VaapiQP = q
			cfg.ActiveQualityOverride = cfg.VaapiQPFixedOverride
		} else if cfg.QualityOverride != "" {
			q, err := parseInt(cfg.QualityOverride, "quality")
			if err != nil {
				return err
			}
			cfg.VaapiQP = q
			cfg.ActiveQualityOverride = cfg.QualityOverride
		}
		return nil
	}
	if cfg.CpuCRFFixedOverride != "" {
		q, err := parseInt(cfg.CpuCRFFixedOverride, "CPU CRF")
		if err != nil {
			return err
		}
		cfg.CpuCRF = q
		cfg.ActiveQualityOverride = cfg.CpuCRFFixedOverride
	} else if cfg.QualityOverride != "" {
		q, err := parseInt(cfg.QualityOverride, "quality")
		if err != nil {
			return err
		}
		cfg.CpuCRF = q
		cfg.ActiveQualityOverride = cfg.QualityOverride
	}
	return nil
}

// Quality value ranges. These match the clamp ranges used by smart quality
// and the retry engine — values outside this range indicate user error.
const (
	qualityMin = 0
	qualityMax = 51
)

// parseInt parses a string as an integer for quality/CRF/QP flags; returns a clear error on failure.
func parseInt(s, name string) (int, error) {
	n, err := strconv.Atoi(strings.TrimSpace(s))
	if err != nil {
		return 0, fmt.Errorf("%s must be a whole number (got %q)", name, s)
	}
	if n < qualityMin || n > qualityMax {
		return 0, fmt.Errorf("%s must be between %d and %d (got %d)", name, qualityMin, qualityMax, n)
	}
	return n, nil
}

// printUsage writes the help text to stderr. Column-aligned for readability.
func printUsage(_ *flag.FlagSet, version string) {
	const col1 = 28 // width of "  -x, --long-name <arg>  "
	lines := []struct {
		flags string
		desc  string
	}{
		{"", "Muxmaster v" + version + " — Jellyfin-optimized media encoder"},
		{"", ""},
		{"  muxmaster [OPTIONS] <input_dir> <output_dir>", ""},
		{"", ""},
		{"Encoding", ""},
		{"  -m, --mode <vaapi|cpu>", "Encoder mode (default: vaapi)"},
		{"  -q, --quality <value>", "Fixed QP (VAAPI) or CRF (CPU) for active mode"},
		{"  --cpu-crf <value>", "Fixed CPU CRF (overrides --quality in CPU mode)"},
		{"  --vaapi-qp <value>", "Fixed VAAPI QP (overrides --quality in VAAPI mode)"},
		{"  -p, --preset <name>", "x265 preset (default: slow)"},
		{"  --audio-bitrate <rate>", "Audio bitrate in Kbps (default: 256k)"},
		{"", ""},
		{"Container & HDR", ""},
		{"  --container <mkv|mp4>", "Output container (default: mkv)"},
		{"  --hdr <preserve|tonemap>", "HDR handling (default: preserve)"},
		{"  --no-deinterlace", "Disable automatic deinterlace"},
		{"", ""},
		{"Streams", ""},
		{"  --no-skip-hevc", "Re-encode HEVC video (default: remux)"},
		{"  --no-subs", "Do not process subtitle streams"},
		{"  --no-attachments", "Do not include attachments"},
		{"", ""},
		{"Output & behavior", ""},
		{"  -f, --force", "Overwrite existing output files"},
		{"  -d, --dry-run", "Preview only; do not encode or remux"},
		{"  --strict", "Disable automatic ffmpeg retry fallbacks"},
		{"  --smart-quality", "Per-file quality adaptation (default: on)"},
		{"  --no-smart-quality", "Use fixed quality only"},
		{"  --clean-timestamps", "Regenerate timestamps (default: on)"},
		{"  --no-clean-timestamps", "Disable timestamp regeneration"},
		{"  --match-audio-layout", "Normalize audio layout (default: on)"},
		{"  --no-match-audio-layout", "Disable audio layout normalization"},
		{"", ""},
		{"Display", ""},
		{"  --show-fps", "Show live ffmpeg FPS (default: on)"},
		{"  --no-fps", "Disable live FPS"},
		{"  --no-stats", "Hide per-file source stats"},
		{"  --color", "Force colored logs"},
		{"  --no-color", "Disable colored logs"},
		{"  -v, --verbose", "Verbose output"},
		{"", ""},
		{"Utility", ""},
		{"  -l, --log <path>", "Append logs to file"},
		{"  -a, --analyze", "Probe all files and print codec/bitrate table"},
		{"  -c, --check", "System diagnostics (ffmpeg, VAAPI, x265, libfdk_aac)"},
		{"  -V, --version", "Print version and exit"},
		{"  -h, --help", "Show this help and exit"},
	}

	for _, l := range lines {
		if l.flags == "" && l.desc == "" {
			fmt.Fprintln(os.Stderr)
			continue
		}
		if l.desc == "" {
			fmt.Fprintln(os.Stderr, l.flags)
			continue
		}
		if l.flags == "" {
			fmt.Fprintln(os.Stderr, l.desc)
			continue
		}
		padding := col1 - len(l.flags)
		if padding < 1 {
			padding = 1
		}
		fmt.Fprintf(os.Stderr, "%s%*s%s\n", l.flags, padding, "", l.desc)
	}
}

// flag.Value adapters so we can use enum types (EncoderMode, Container, HDRMode) with flag.Var.

type encoderModeValue struct{ p *EncoderMode }

func (e *encoderModeValue) String() string { return string(*e.p) }
func (e *encoderModeValue) Set(s string) error {
	switch strings.ToLower(s) {
	case "vaapi":
		*e.p = EncoderVAAPI
	case "cpu":
		*e.p = EncoderCPU
	default:
		return fmt.Errorf("invalid mode %q (use 'vaapi' or 'cpu')", s)
	}
	return nil
}

type containerValue struct{ p *Container }

func (c *containerValue) String() string { return string(*c.p) }
func (c *containerValue) Set(s string) error {
	switch strings.ToLower(s) {
	case "mkv":
		*c.p = ContainerMKV
	case "mp4":
		*c.p = ContainerMP4
	default:
		return fmt.Errorf("invalid container %q (use 'mkv' or 'mp4')", s)
	}
	return nil
}

type hdrModeValue struct{ p *HDRMode }

func (h *hdrModeValue) String() string { return string(*h.p) }
func (h *hdrModeValue) Set(s string) error {
	switch strings.ToLower(s) {
	case "preserve":
		*h.p = HDRPreserve
	case "tonemap":
		*h.p = HDRTonemap
	default:
		return fmt.Errorf("invalid HDR mode %q (use 'preserve' or 'tonemap')", s)
	}
	return nil
}
