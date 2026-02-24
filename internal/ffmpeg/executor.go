package ffmpeg

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"

	"github.com/backmassage/muxmaster/internal/config"
)

// ExecResult holds the outcome of a single ffmpeg invocation.
type ExecResult struct {
	Stderr string // captured stderr (always populated)
	Err    error  // non-nil when ffmpeg exits non-zero or fails to start
}

// Execute runs ffmpeg with the supplied arguments, captures stderr into a
// buffer, and optionally tees it to os.Stderr when verbose mode or FPS
// display is active. This matches the legacy run_ffmpeg_logged behavior
// without requiring temporary error files.
func Execute(ctx context.Context, args []string, cfg *config.Config) ExecResult {
	if len(args) == 0 {
		return ExecResult{Err: fmt.Errorf("ffmpeg: empty argument list")}
	}

	cmd := exec.CommandContext(ctx, args[0], args[1:]...)
	cmd.Stdin = nil

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
