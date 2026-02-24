package ffmpeg

import (
	"bytes"
	"context"
	"io"
	"os"
	"os/exec"

	"github.com/backmassage/muxmaster/internal/config"
	"github.com/backmassage/muxmaster/internal/planner"
)

// ExecResult holds the outcome of a single ffmpeg invocation.
type ExecResult struct {
	Stderr string
	Err    error
}

// Execute builds and runs the ffmpeg command for a file. When verbose or
// show-fps is enabled, stderr is tee'd to os.Stderr in real time; otherwise
// it is captured silently for retry classification.
//
// This mirrors the legacy run_ffmpeg_logged helper and the stderr capture
// strategy documented in foundation-plan.md §7.3.
func Execute(ctx context.Context, cfg *config.Config, plan *planner.FilePlan, rs *RetryState) ExecResult {
	args := Build(cfg, plan, rs)

	cmd := exec.CommandContext(ctx, args[0], args[1:]...)

	var stderrBuf bytes.Buffer
	if cfg.Verbose || cfg.ShowFfmpegFPS {
		cmd.Stderr = io.MultiWriter(&stderrBuf, os.Stderr)
	} else {
		cmd.Stderr = &stderrBuf
	}

	err := cmd.Run()
	return ExecResult{
		Stderr: stderrBuf.String(),
		Err:    err,
	}
}
