// Command muxmaster is the CLI entrypoint for the Muxmaster media encoder.
//
// It parses flags, validates configuration and paths, and either runs
// system diagnostics (--check) or the encode/remux pipeline.
package main

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"

	"github.com/backmassage/muxmaster/internal/check"
	"github.com/backmassage/muxmaster/internal/config"
	"github.com/backmassage/muxmaster/internal/display"
	"github.com/backmassage/muxmaster/internal/ffmpeg"
	"github.com/backmassage/muxmaster/internal/logging"
	"github.com/backmassage/muxmaster/internal/pipeline"
)

// version and commit are injected at build time via -ldflags.
// When built with plain "go build" (no make), these retain their defaults.
// The Makefile is the authoritative source for VERSION; see the Makefile for ldflags details.
var (
	version = "2.6.0"
	commit  = "unknown"
)

func main() {
	os.Exit(run())
}

func run() int {
	// Phase 1: Bootstrap — the logger doesn't exist yet, so errors go
	// directly to stderr via fmt. Once NewLogger succeeds, all output
	// goes through the logger for consistent formatting and log-file capture.
	cfg := config.DefaultConfig()
	if err := config.ParseFlags(&cfg, version, commit); err != nil {
		if err == config.ErrExitClean {
			return 0
		}
		fmt.Fprintf(os.Stderr, "muxmaster: %v\n", err)
		return 1
	}

	if err := cfg.Validate(); err != nil {
		fmt.Fprintf(os.Stderr, "muxmaster: %v\n", err)
		return 1
	}

	log, err := logging.NewLogger(&cfg)
	if err != nil {
		fmt.Fprintf(os.Stderr, "muxmaster: %v\n", err)
		return 1
	}
	defer log.Close()

	// Phase 2: Logger available — all output goes through log from here on.
	display.PrintBanner()

	if cfg.CheckOnly {
		if !check.RunCheck(&cfg, log) {
			return 1
		}
		return 0
	}

	if cfg.AnalyzeOnly {
		inputAbs, err := absExistingPath(cfg.InputDir)
		if err != nil {
			log.Error("Input path error: %v", err)
			return 1
		}
		cfg.InputDir = inputAbs

		log.Info("=== Muxmaster v%s (%s) — Analyze ===", version, commit)
		log.Info("In: %s", cfg.InputDir)
		log.Blank()

		ctx, cancel := signalContext(log)
		defer cancel()

		pipeline.Analyze(ctx, &cfg, log)
		return 0
	}

	// Resolve and validate paths: input must exist, output is created if
	// needed, and output must not be inside input (prevents recursive processing).
	inputAbs, err := absExistingPath(cfg.InputDir)
	if err != nil {
		log.Error("Input path error: %v", err)
		return 1
	}
	outputAbs, err := absCreatablePath(cfg.OutputDir)
	if err != nil {
		log.Error("Cannot resolve output path: %v", err)
		return 1
	}
	if err := cfg.ValidatePaths(inputAbs, outputAbs); err != nil {
		log.Error("%v", err)
		log.Error("Choose an output path outside: %s", cfg.InputDir)
		return 1
	}

	log.Info("=== Muxmaster v%s (%s) ===", version, commit)
	log.Info("In:  %s", cfg.InputDir)
	log.Info("Out: %s", cfg.OutputDir)
	if cfg.DryRun {
		log.Warn("DRY RUN — no files will be written")
	}
	log.Info("")

	if !cfg.DryRun {
		if err := os.MkdirAll(cfg.OutputDir, 0o755); err != nil {
			log.Error("Cannot create output directory: %v", err)
			return 1
		}
	}

	// Fail fast if ffmpeg/ffprobe or the chosen encoder are unavailable.
	if err := check.CheckDeps(&cfg); err != nil {
		log.Error("%v", err)
		return 1
	}

	// Phase 3: Signal handling + pipeline execution.
	ctx, cancel := signalContext(log)
	defer cancel()

	run := ffmpeg.NewRunFunc(cfg.Display.Verbose || cfg.Display.FfmpegFPS)
	stats := pipeline.Run(ctx, &cfg, log, run)

	if ctx.Err() != nil {
		return 1
	}
	if stats.Failed > 0 {
		return 1
	}
	return 0
}

// signalContext returns a context that is cancelled on SIGINT/SIGTERM.
// A second signal forces immediate exit.
func signalContext(log *logging.Logger) (context.Context, context.CancelFunc) {
	ctx, cancel := context.WithCancel(context.Background())
	sigCh := make(chan os.Signal, 2)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
	go func() {
		<-sigCh
		log.Warn("Received interrupt, finishing current file…")
		cancel()
		<-sigCh
		log.Error("Forced exit")
		os.Exit(1)
	}()
	return ctx, cancel
}

// absExistingPath returns the absolute, symlink-resolved path for paths that
// must already exist, such as the input library.
func absExistingPath(path string) (string, error) {
	abs, err := filepath.Abs(path)
	if err != nil {
		return "", err
	}
	return filepath.EvalSymlinks(abs)
}

// absCreatablePath returns an absolute path suitable for safety checks before
// the output directory exists. Existing path components are symlink-resolved;
// missing trailing components are appended without creating anything.
func absCreatablePath(path string) (string, error) {
	abs, err := filepath.Abs(path)
	if err != nil {
		return "", err
	}
	abs = filepath.Clean(abs)

	if resolved, err := filepath.EvalSymlinks(abs); err == nil {
		return resolved, nil
	} else if !errors.Is(err, os.ErrNotExist) {
		return "", err
	}

	var missing []string
	cur := abs
	for {
		resolved, err := filepath.EvalSymlinks(cur)
		if err == nil {
			for i := len(missing) - 1; i >= 0; i-- {
				resolved = filepath.Join(resolved, missing[i])
			}
			return resolved, nil
		}
		if !errors.Is(err, os.ErrNotExist) {
			return "", err
		}

		parent := filepath.Dir(cur)
		if parent == cur {
			return "", err
		}
		missing = append(missing, filepath.Base(cur))
		cur = parent
	}
}
