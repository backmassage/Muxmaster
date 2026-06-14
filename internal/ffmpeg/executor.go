// executor.go runs ffmpeg subprocesses with stderr capture and optional FPS display.
package ffmpeg

import (
	"bytes"
	"context"
	"errors"
	"io"
	"os"
	"os/exec"
	"syscall"

	"github.com/backmassage/muxmaster/internal/config"
	"github.com/backmassage/muxmaster/internal/planner"
)

// ExecResult holds the outcome of a single ffmpeg invocation.
type ExecResult struct {
	Stderr string
	Err    error
	// Signal is the name of the signal that terminated ffmpeg (e.g. "killed",
	// "segmentation fault"), or "" when the process exited normally — including
	// with a non-zero status. A signal kill prints nothing to stderr, so this is
	// the only evidence of an OOM SIGKILL or a crash; the retry classifier and
	// failure reporter use it to avoid the misleading "no error message captured".
	Signal string
}

// signalName returns the terminating signal's name if err is an *exec.ExitError
// whose process was killed by a signal, or "" otherwise.
func signalName(err error) string {
	var ee *exec.ExitError
	if errors.As(err, &ee) {
		if ws, ok := ee.ProcessState.Sys().(syscall.WaitStatus); ok && ws.Signaled() {
			return ws.Signal().String()
		}
	}
	return ""
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
			Signal: signalName(err),
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
