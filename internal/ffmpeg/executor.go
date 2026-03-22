// executor.go runs ffmpeg subprocesses with stderr capture and optional FPS display.
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

// RunFunc executes a built ffmpeg argument list and returns the result.
// Production code uses a RunFunc created by NewRunFunc; tests substitute
// a mock that inspects arguments and returns controlled results.
type RunFunc func(ctx context.Context, args []string) ExecResult

// NewRunFunc returns a RunFunc that spawns a real OS process. When
// showOutput is true, stderr is tee'd to os.Stderr in real time for
// verbose/FPS display; otherwise it is captured silently for retry
// classification.
func NewRunFunc(showOutput bool) RunFunc {
	return func(ctx context.Context, args []string) ExecResult {
		cmd := exec.CommandContext(ctx, args[0], args[1:]...)

		var stderrBuf bytes.Buffer
		if showOutput {
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
}

// Execute builds and runs the ffmpeg command for a file. The run parameter
// controls how the subprocess is launched — production callers pass a RunFunc
// from NewRunFunc; tests pass a mock.
func Execute(ctx context.Context, cfg *config.Config, plan *planner.FilePlan, rs *RetryState, run RunFunc) ExecResult {
	args := Build(cfg, plan, rs)
	return run(ctx, args)
}
