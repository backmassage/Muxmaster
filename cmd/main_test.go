package main

import (
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/backmassage/muxmaster/internal/config"
)

func TestAbsCreatablePathAllowsMissingLeaf(t *testing.T) {
	root := t.TempDir()
	output := filepath.Join(root, "new", "nested")

	got, err := absCreatablePath(output)
	if err != nil {
		t.Fatalf("absCreatablePath: %v", err)
	}

	want, err := filepath.Abs(output)
	if err != nil {
		t.Fatalf("Abs: %v", err)
	}
	if got != want {
		t.Fatalf("got %q, want %q", got, want)
	}
	if _, err := os.Stat(output); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("absCreatablePath created output path or returned unexpected stat error: %v", err)
	}
}

func TestAbsCreatablePathResolvesSymlinkParent(t *testing.T) {
	root := t.TempDir()
	input := filepath.Join(root, "input")
	realOutputParent := filepath.Join(input, "generated")
	if err := os.MkdirAll(realOutputParent, 0o755); err != nil {
		t.Fatalf("MkdirAll real output parent: %v", err)
	}

	outputLink := filepath.Join(root, "out-link")
	if err := os.Symlink(realOutputParent, outputLink); err != nil {
		t.Fatalf("Symlink: %v", err)
	}

	output := filepath.Join(outputLink, "library")
	got, err := absCreatablePath(output)
	if err != nil {
		t.Fatalf("absCreatablePath: %v", err)
	}

	realOutputParentAbs, err := filepath.EvalSymlinks(realOutputParent)
	if err != nil {
		t.Fatalf("EvalSymlinks real output parent: %v", err)
	}
	want := filepath.Join(realOutputParentAbs, "library")
	if got != want {
		t.Fatalf("got %q, want %q", got, want)
	}
	if _, err := os.Stat(output); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("absCreatablePath created output path or returned unexpected stat error: %v", err)
	}

	inputAbs, err := absExistingPath(input)
	if err != nil {
		t.Fatalf("absExistingPath: %v", err)
	}
	cfg := config.DefaultConfig()
	if err := cfg.ValidatePaths(inputAbs, got); err == nil {
		t.Fatal("expected output under symlinked input directory to be rejected")
	}
}
