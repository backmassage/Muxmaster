package tune

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

func TestDetectContentLive(t *testing.T) {
	if os.Getenv("MUXMASTER_LIVE_FFMPEG") != "1" {
		t.Skip("set MUXMASTER_LIVE_FFMPEG=1 to run live ffmpeg grain detection")
	}
	if _, err := exec.LookPath("ffmpeg"); err != nil {
		t.Skip("ffmpeg not available")
	}

	dir := t.TempDir()
	input := filepath.Join(dir, "grain-smoke.mkv")
	gen := exec.Command("ffmpeg",
		"-f", "lavfi", "-i", "testsrc2=duration=2:size=320x180:rate=24",
		"-c:v", "libx264", "-pix_fmt", "yuv420p",
		"-y", input,
	)
	gen.Stderr = os.Stderr
	if err := gen.Run(); err != nil {
		t.Fatalf("generate live sample: %v", err)
	}

	sig, err := DetectContent(context.Background(), input, 0)
	if err != nil {
		t.Fatalf("DetectContent: %v", err)
	}
	if sig.Frames == 0 {
		t.Fatalf("DetectContent returned no frames: %+v", sig)
	}
}
