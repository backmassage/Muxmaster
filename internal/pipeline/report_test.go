package pipeline

import (
	"fmt"
	"strings"
	"testing"
)

// captureLogger records every Error line for assertion.
type captureLogger struct {
	nopLogger
	lines []string
}

func (c *captureLogger) Error(format string, args ...interface{}) {
	c.lines = append(c.lines, fmt.Sprintf(format, args...))
}

func (c *captureLogger) joined() string { return strings.Join(c.lines, "\n") }

// ffmpeg rewrites its progress line in place with carriage returns, so a failed
// run's stderr is one long `\r`-joined string with the real error buried among
// the stats updates. logStderr must split on `\r`, drop progress lines, and
// surface the actual diagnostic.
func TestLogStderr_SurfacesErrorBuriedInProgressNoise(t *testing.T) {
	stderr := "frame=  120 fps=2455 q=-1.0 size=16KiB time=N/A bitrate=N/A speed=N/A elapsed=0:00:01.00\r" +
		"Non-monotonic DTS; previous: 100, current: 90;\n" +
		"frame=120298 fps=2455 q=-1.0 size=16KiB time=N/A bitrate=N/A speed=N/A elapsed=0:00:49.00\r"

	log := &captureLogger{}
	logStderr(log, stderr)
	out := log.joined()

	if !strings.Contains(out, "Non-monotonic DTS") {
		t.Errorf("real error not surfaced; got:\n%s", out)
	}
	if strings.Contains(out, "fps=2455") || strings.Contains(out, "time=N/A") {
		t.Errorf("progress noise should be filtered; got:\n%s", out)
	}
}

// When only progress output was captured (no real error reached stderr),
// logStderr must not go silent — it falls back to the last progress line so the
// failure still produces visible context.
func TestLogStderr_ProgressOnlyFallback(t *testing.T) {
	stderr := "frame=  120 fps=2455 q=-1.0 size=16KiB time=N/A bitrate=N/A speed=N/A\r" +
		"frame=120298 fps=2455 q=-1.0 size=16KiB time=N/A bitrate=N/A speed=N/A\r"

	log := &captureLogger{}
	logStderr(log, stderr)
	out := log.joined()

	if !strings.Contains(out, "progress only") {
		t.Errorf("expected progress-only note; got:\n%s", out)
	}
	if !strings.Contains(out, "frame=120298") {
		t.Errorf("expected last progress line as fallback; got:\n%s", out)
	}
}

func TestLogStderr_EmptyIsSilent(t *testing.T) {
	log := &captureLogger{}
	logStderr(log, "")
	if len(log.lines) != 0 {
		t.Errorf("empty stderr should log nothing; got %v", log.lines)
	}
}

// An OOM SIGKILL prints nothing to stderr; logSignal must name it as a kernel
// kill / likely OOM so the failure isn't mistaken for a contentless error.
func TestLogSignal_KilledReportsOOM(t *testing.T) {
	log := &captureLogger{}
	logSignal(log, "killed")
	out := log.joined()
	if !strings.Contains(out, "SIGKILL") || !strings.Contains(strings.ToLower(out), "memory") {
		t.Errorf("expected SIGKILL/OOM diagnosis; got:\n%s", out)
	}
}

func TestLogSignal_OtherSignalNamed(t *testing.T) {
	log := &captureLogger{}
	logSignal(log, "segmentation fault")
	if !strings.Contains(log.joined(), "segmentation fault") {
		t.Errorf("expected signal name; got:\n%s", log.joined())
	}
}

func TestLogSignal_EmptyIsSilent(t *testing.T) {
	log := &captureLogger{}
	logSignal(log, "")
	if len(log.lines) != 0 {
		t.Errorf("normal exit should log nothing; got %v", log.lines)
	}
}
