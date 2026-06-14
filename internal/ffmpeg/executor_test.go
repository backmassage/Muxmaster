package ffmpeg

import (
	"context"
	"os/exec"
	"testing"
)

// signalName must extract the terminating signal from a real signal-killed
// process (the OOM SIGKILL case that produces no stderr), and return "" for
// both normal exits and non-zero exits without a signal.
func TestSignalName(t *testing.T) {
	// A process killed by SIGKILL.
	killed := exec.CommandContext(context.Background(), "sh", "-c", "kill -KILL $$")
	if got := signalName(killed.Run()); got != "killed" {
		t.Errorf("SIGKILL: signalName = %q, want %q", got, "killed")
	}

	// A clean exit: no signal.
	ok := exec.CommandContext(context.Background(), "true")
	if got := signalName(ok.Run()); got != "" {
		t.Errorf("clean exit: signalName = %q, want \"\"", got)
	}

	// A non-zero exit without a signal must NOT be reported as signaled.
	fail := exec.CommandContext(context.Background(), "false")
	if got := signalName(fail.Run()); got != "" {
		t.Errorf("non-zero exit: signalName = %q, want \"\"", got)
	}

	// A non-ExitError (e.g. binary not found) is not a signal kill.
	missing := exec.CommandContext(context.Background(), "this-binary-does-not-exist-muxmaster")
	if got := signalName(missing.Run()); got != "" {
		t.Errorf("exec error: signalName = %q, want \"\"", got)
	}
}
