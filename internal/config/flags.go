package config

import (
	"flag"
	"fmt"
	"os"
	"strconv"
	"strings"
)

// ParseFlags parses os.Args into cfg. Usage and version are handled by flags; on error returns non-nil.
func ParseFlags(cfg *Config) error {
	fs := flag.NewFlagSet("muxmaster", flag.ContinueOnError)
	fs.Usage = func() { usage(fs) }

	// Encoding
	fs.Var(&encoderModeValue{&cfg.EncoderMode}, "mode", "Encoder mode (vaapi|cpu)")
	fs.Var(&encoderModeValue{&cfg.EncoderMode}, "m", "Encoder mode (short)")
	fs.StringVar(&cfg.QualityOverride, "quality", "", "Fixed quality for active mode (QP/CRF)")
	fs.StringVar(&cfg.QualityOverride, "q", "", "Fixed quality (short)")
	fs.StringVar(&cfg.CpuCRFFixedOverride, "cpu-crf", "", "Fixed CPU CRF override")
	fs.StringVar(&cfg.VaapiQPFixedOverride, "vaapi-qp", "", "Fixed VAAPI QP override")
	fs.StringVar(&cfg.CpuPreset, "preset", cfg.CpuPreset, "CPU preset")
	fs.StringVar(&cfg.CpuPreset, "p", cfg.CpuPreset, "CPU preset (short)")

	// Container / HDR
	fs.Var(&containerValue{&cfg.OutputContainer}, "container", "Output container (mkv|mp4)")
	fs.Var(&hdrModeValue{&cfg.HandleHDR}, "hdr", "HDR handling (preserve|tonemap)")
	var noDeinterlace bool
	fs.BoolVar(&noDeinterlace, "no-deinterlace", false, "Disable auto deinterlace")

	// Behavior (defaults set in DefaultConfig; negated flags applied after Parse)
	var noSkipHEVC, noFps, noStats, noSubs, noAttachments, noSmartQuality, noCleanTimestamps, noMatchLayout, force bool
	fs.BoolVar(&cfg.DryRun, "dry-run", false, "Preview only")
	fs.BoolVar(&cfg.DryRun, "d", false, "Dry run (short)")
	fs.BoolVar(&noSkipHEVC, "no-skip-hevc", false, "Re-encode HEVC video")
	fs.BoolVar(&noFps, "no-fps", false, "Disable live FPS")
	fs.BoolVar(&noStats, "no-stats", false, "Hide per-file stats")
	fs.BoolVar(&noSubs, "no-subs", false, "Do not process subtitles")
	fs.BoolVar(&noAttachments, "no-attachments", false, "Do not include attachments")
	fs.BoolVar(&cfg.StrictMode, "strict", false, "Disable ffmpeg retry fallbacks")
	fs.BoolVar(&noSmartQuality, "no-smart-quality", false, "Use fixed quality only")
	fs.BoolVar(&noCleanTimestamps, "no-clean-timestamps", false, "Disable timestamp regeneration")
	fs.BoolVar(&noMatchLayout, "no-match-audio-layout", false, "Disable audio layout normalization")
	fs.BoolVar(&force, "force", false, "Overwrite existing output files")
	fs.BoolVar(&force, "f", false, "Force (short)")

	// Display (--color / --no-color are bool flags)
	var forceColor, noColor bool
	fs.BoolVar(&forceColor, "color", false, "Force colored logs")
	fs.BoolVar(&noColor, "no-color", false, "Disable colored logs")
	fs.BoolVar(&cfg.Verbose, "verbose", false, "Verbose output")
	fs.BoolVar(&cfg.Verbose, "v", false, "Verbose (short)")
	fs.BoolVar(&cfg.CheckOnly, "check", false, "System diagnostics")
	fs.BoolVar(&cfg.CheckOnly, "c", false, "Check (short)")
	fs.StringVar(&cfg.LogFile, "log", "", "Write logs to file")
	fs.StringVar(&cfg.LogFile, "l", "", "Log file (short)")

	var showVersion bool
	fs.BoolVar(&showVersion, "version", false, "Print version")
	fs.BoolVar(&showVersion, "V", false, "Version (short)")
	fs.BoolVar(&showHelp, "help", false, "Help")
	fs.BoolVar(&showHelp, "h", false, "Help (short)")

	if err := fs.Parse(os.Args[1:]); err != nil {
		return err
	}
	// Apply negated / override flags
	if noDeinterlace {
		cfg.DeinterlaceAuto = false
	}
	if noSkipHEVC {
		cfg.SkipHEVC = false
	}
	if noFps {
		cfg.ShowFfmpegFPS = false
	}
	if noStats {
		cfg.ShowFileStats = false
	}
	if noSubs {
		cfg.KeepSubtitles = false
	}
	if noAttachments {
		cfg.KeepAttachments = false
	}
	if noSmartQuality {
		cfg.SmartQuality = false
	}
	if noCleanTimestamps {
		cfg.CleanTimestamps = false
	}
	if noMatchLayout {
		cfg.MatchAudioLayout = false
	}
	if force {
		cfg.SkipExisting = false
	}
	if noColor {
		cfg.ColorMode = ColorNever
	} else if forceColor {
		cfg.ColorMode = ColorAlways
	}
	if showHelp {
		usage(fs)
		os.Exit(0)
	}
	if showVersion {
		fmt.Fprintln(os.Stdout, "muxmaster v"+version)
		os.Exit(0)
	}

	// Positional: input_dir output_dir (required unless --check)
	positional := fs.Args()
	if !cfg.CheckOnly {
		if len(positional) != 2 {
			return fmt.Errorf("need exactly input_dir and output_dir")
		}
		cfg.InputDir = NormalizeDirArg(positional[0])
		cfg.OutputDir = NormalizeDirArg(positional[1])
	}

	// Quality precedence: mode-specific override > --quality > defaults
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
	} else {
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
	}

	// Verbose -> ffmpeg loglevel info (handled at run time; no field for it in Config today)
	return nil
}

var showHelp bool
var version = "2.0.0-dev"

func usage(fs *flag.FlagSet) {
	// Optional: print to stderr for help, stdout for version
	fmt.Fprintf(os.Stderr, `Muxmaster v%s - Jellyfin-Optimized Media Encoder

Usage: muxmaster [OPTIONS] <input_dir> <output_dir>

Encoding Options:
  -m, --mode <vaapi|cpu>    Encoder mode (default: vaapi)
  -q, --quality <value>     Fixed quality for active mode
  --cpu-crf <value>         Fixed CPU CRF override
  --vaapi-qp <value>        Fixed VAAPI QP override
  -p, --preset <preset>     CPU preset (default: slow)

HDR/Color Options:
  --hdr <preserve|tonemap>  HDR handling (default: preserve)
  --no-deinterlace          Disable automatic deinterlace

Stream Options:
  --skip-hevc               Copy HEVC video (default: on)
  --no-skip-hevc            Re-encode HEVC video
  --no-subs                 Do not process subtitle streams
  --no-attachments          Do not include attachments

Output Options:
  --container <mkv|mp4>     Output container (default: mkv)
  -f, --force               Overwrite existing output files

Behavior Options:
  -d, --dry-run             Preview only
  --strict                  Disable automatic ffmpeg retry fallbacks
  --smart-quality           Adapt quality per file (default: on)
  --no-smart-quality        Use fixed quality only
  --clean-timestamps        Enable timestamp regeneration (default: on)
  --no-clean-timestamps     Disable timestamp regeneration
  --match-audio-layout      Normalize encoded audio layout (default: on)
  --no-match-audio-layout   Disable audio layout normalization

Display Options:
  --show-fps                Show live ffmpeg FPS (default: on)
  --no-fps                  Disable live FPS
  --no-stats                Hide per-file source stats
  --color                   Force colored logs
  --no-color                Disable colored logs
  -v, --verbose             Verbose output

Utility:
  -l, --log <path>          Write logs to file
  -c, --check               System diagnostics
  -V, --version             Print version
  -h, --help                Help
`, version)
}

func parseInt(s, name string) (int, error) {
	n, err := strconv.Atoi(strings.TrimSpace(s))
	if err != nil {
		return 0, fmt.Errorf("%s must be a whole number (got %q)", name, s)
	}
	return n, nil
}

// flag.Value adapters for enum types
type encoderModeValue struct{ p *EncoderMode }
type containerValue struct{ p *Container }
type hdrModeValue struct{ p *HDRMode }

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

