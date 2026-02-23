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

var (
	version = "2.0.0-dev"
	commit  = "unknown"
)

func main() {
	cfg := config.DefaultConfig()
	if err := config.ParseFlags(&cfg); err != nil {
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

	display.PrintBanner()

	if cfg.CheckOnly {
		check.RunCheck(&cfg, log)
		os.Exit(0)
	}

	// Validate paths: input exists, output creatable, output not inside input
	inputAbs, err := absPath(cfg.InputDir)
	if err != nil {
		log.Error("Input not found: %s", cfg.InputDir)
		os.Exit(1)
	}
	if err := os.MkdirAll(cfg.OutputDir, 0755); err != nil {
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

	log.Info("=== Muxmaster v%s ===", version)
	log.Info("In:  %s", cfg.InputDir)
	log.Info("Out: %s", cfg.OutputDir)
	if cfg.DryRun {
		log.Warn("DRY RUN")
	}
	log.Info("")

	if err := check.CheckDeps(&cfg); err != nil {
		log.Error("%v", err)
		os.Exit(1)
	}

	// Phase 1: no pipeline yet; log and exit 0
	log.Info("Ready. (Pipeline not yet implemented.)")
}

func absPath(path string) (string, error) {
	abs, err := filepath.Abs(path)
	if err != nil {
		return "", err
	}
	return filepath.EvalSymlinks(abs)
}
