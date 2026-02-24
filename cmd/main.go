// Command muxmaster is the CLI entrypoint for the Muxmaster media encoder.
//
// It parses flags, validates configuration and paths, and either runs
// system diagnostics (--check) or the encode/remux pipeline.
package main

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/backmassage/muxmaster/internal/check"
	"github.com/backmassage/muxmaster/internal/config"
	"github.com/backmassage/muxmaster/internal/display"
	"github.com/backmassage/muxmaster/internal/logging"
)

// version and commit are injected at build time via -ldflags.
// When built with plain "go build" (no make), these retain their defaults.
// The Makefile is the authoritative source for VERSION; see the Makefile for ldflags details.
var (
	version = "2.0.0-dev"
	commit  = "unknown"
)

func main() {
	// Phase 1: Bootstrap — the logger doesn't exist yet, so errors go
	// directly to stderr via fmt. Once NewLogger succeeds, all output
	// goes through the logger for consistent formatting and log-file capture.
	cfg := config.DefaultConfig()
	if err := config.ParseFlags(&cfg, version); err != nil {
		fmt.Fprintf(os.Stderr, "muxmaster: %v\n", err)
		os.Exit(1)
	}

	if err := cfg.Validate(); err != nil {
		fmt.Fprintf(os.Stderr, "muxmaster: %v\n", err)
		os.Exit(1)
	}

	log, err := logging.NewLogger(&cfg)
	if err != nil {
		fmt.Fprintf(os.Stderr, "muxmaster: %v\n", err)
		os.Exit(1)
	}
	defer log.Close()

	// Phase 2: Logger available — all output goes through log from here on.
	display.PrintBanner()

	if cfg.CheckOnly {
		check.RunCheck(&cfg, log)
		os.Exit(0)
	}

	// Resolve and validate paths: input must exist, output is created if
	// needed, and output must not be inside input (prevents recursive processing).
	inputAbs, err := absPath(cfg.InputDir)
	if err != nil {
		log.Error("Input not found: %s", cfg.InputDir)
		os.Exit(1)
	}
	if err := os.MkdirAll(cfg.OutputDir, 0o755); err != nil {
		log.Error("Cannot create output directory: %s", cfg.OutputDir)
		os.Exit(1)
	}
	outputAbs, err := absPath(cfg.OutputDir)
	if err != nil {
		log.Error("Cannot resolve output path: %s", cfg.OutputDir)
		os.Exit(1)
	}
	if err := cfg.ValidatePaths(inputAbs, outputAbs); err != nil {
		log.Error("%v", err)
		log.Error("Choose an output path outside: %s", cfg.InputDir)
		os.Exit(1)
	}

	log.Info("=== Muxmaster v%s (%s) ===", version, commit)
	log.Info("In:  %s", cfg.InputDir)
	log.Info("Out: %s", cfg.OutputDir)
	if cfg.DryRun {
		log.Warn("DRY RUN — no files will be written")
	}
	log.Info("")

	// Fail fast if ffmpeg/ffprobe or the chosen encoder are unavailable.
	if err := check.CheckDeps(&cfg); err != nil {
		log.Error("%v", err)
		os.Exit(1)
	}

	// TODO: Run pipeline (discover → probe → plan → execute).
	log.Info("Ready. (Pipeline not yet implemented.)")
}

// absPath returns the absolute, symlink-resolved path for safe comparison
// of input vs output directory hierarchies.
func absPath(path string) (string, error) {
	abs, err := filepath.Abs(path)
	if err != nil {
		return "", err
	}
	return filepath.EvalSymlinks(abs)
}
