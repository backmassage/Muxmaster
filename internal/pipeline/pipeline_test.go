package pipeline

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/backmassage/muxmaster/internal/config"
	"github.com/backmassage/muxmaster/internal/logging"
)

// --- Discover tests ---

func TestDiscover_FiltersExtensions(t *testing.T) {
	dir := t.TempDir()
	touch(t, dir, "movie.mkv")
	touch(t, dir, "show.mp4")
	touch(t, dir, "music.mp3")
	touch(t, dir, "readme.txt")
	touch(t, dir, "anime.avi")
	touch(t, dir, "special.m4v")

	files, err := Discover(dir)
	if err != nil {
		t.Fatalf("Discover: %v", err)
	}

	want := []string{"anime.avi", "movie.mkv", "show.mp4", "special.m4v"}
	got := basenames(files)
	if !sliceEqual(got, want) {
		t.Errorf("got %v, want %v", got, want)
	}
}

func TestDiscover_AllMediaExtensions(t *testing.T) {
	dir := t.TempDir()
	exts := []string{".mkv", ".mp4", ".avi", ".m4v", ".mov", ".wmv",
		".flv", ".webm", ".ts", ".m2ts", ".mpg", ".mpeg", ".vob", ".ogv"}
	for _, ext := range exts {
		touch(t, dir, "file"+ext)
	}
	touch(t, dir, "file.jpg")

	files, err := Discover(dir)
	if err != nil {
		t.Fatalf("Discover: %v", err)
	}
	if len(files) != len(exts) {
		t.Errorf("got %d files, want %d", len(files), len(exts))
	}
}

func TestDiscover_PrunesExtras(t *testing.T) {
	dir := t.TempDir()
	touch(t, dir, "main.mkv")
	os.MkdirAll(filepath.Join(dir, "Extras"), 0o755)
	touch(t, filepath.Join(dir, "Extras"), "bonus.mkv")
	os.MkdirAll(filepath.Join(dir, "extras"), 0o755)
	touch(t, filepath.Join(dir, "extras"), "deleted_scenes.mp4")

	files, err := Discover(dir)
	if err != nil {
		t.Fatalf("Discover: %v", err)
	}
	if len(files) != 1 {
		t.Errorf("got %d files, want 1 (extras should be pruned)", len(files))
	}
}

func TestDiscover_RecursiveAndSorted(t *testing.T) {
	dir := t.TempDir()
	os.MkdirAll(filepath.Join(dir, "Show", "Season 01"), 0o755)
	os.MkdirAll(filepath.Join(dir, "Show", "Season 02"), 0o755)
	touch(t, filepath.Join(dir, "Show", "Season 02"), "ep01.mkv")
	touch(t, filepath.Join(dir, "Show", "Season 01"), "ep02.mkv")
	touch(t, filepath.Join(dir, "Show", "Season 01"), "ep01.mkv")

	files, err := Discover(dir)
	if err != nil {
		t.Fatalf("Discover: %v", err)
	}

	if len(files) != 3 {
		t.Fatalf("got %d files, want 3", len(files))
	}
	// Should be sorted lexicographically.
	for i := 1; i < len(files); i++ {
		if files[i] < files[i-1] {
			t.Errorf("not sorted: %q before %q", files[i-1], files[i])
		}
	}
}

func TestDiscover_EmptyDir(t *testing.T) {
	dir := t.TempDir()
	files, err := Discover(dir)
	if err != nil {
		t.Fatalf("Discover: %v", err)
	}
	if len(files) != 0 {
		t.Errorf("got %d files, want 0", len(files))
	}
}

func TestDiscover_CaseInsensitiveExtension(t *testing.T) {
	dir := t.TempDir()
	touch(t, dir, "MOVIE.MKV")
	touch(t, dir, "Show.Mp4")

	files, err := Discover(dir)
	if err != nil {
		t.Fatalf("Discover: %v", err)
	}
	if len(files) != 2 {
		t.Errorf("got %d files, want 2 (case-insensitive ext matching)", len(files))
	}
}

// --- RunStats tests ---

func TestRunStats_SpaceSaved(t *testing.T) {
	s := RunStats{TotalInputBytes: 1000, TotalOutputBytes: 600}
	if got := s.SpaceSaved(); got != 400 {
		t.Errorf("SpaceSaved: got %d, want 400", got)
	}

	s2 := RunStats{TotalInputBytes: 100, TotalOutputBytes: 150}
	if got := s2.SpaceSaved(); got != -50 {
		t.Errorf("SpaceSaved (negative): got %d, want -50", got)
	}
}

// --- Bitrate outlier tests ---

func TestBitrateOutlierTiers(t *testing.T) {
	cases := []struct {
		w, h    int
		kbps    int64
		outlier bool
		dir     string
	}{
		{1920, 1080, 5000, false, ""},
		{1920, 1080, 500, true, "low"},
		{1920, 1080, 20000, true, "high"},
		{1280, 720, 3000, false, ""},
		{3840, 2160, 50000, true, "high"},
		{640, 360, 100, true, "low"},
	}
	for _, tc := range cases {
		pixels := tc.w * tc.h
		var low, high int64
		var label string
		for _, tier := range bitrateTiers {
			if pixels <= tier.maxPixels {
				low, high, label = tier.lowKbps, tier.highKbps, tier.label
				break
			}
		}
		if label == "" {
			low, high = 15000, 65000
		}
		isOutlier := tc.kbps < low || tc.kbps > high
		if isOutlier != tc.outlier {
			t.Errorf("%dx%d@%dkbps: outlier=%v, want %v", tc.w, tc.h, tc.kbps, isOutlier, tc.outlier)
		}
	}
}

// --- Dry-run integration test ---

func TestDryRunPipeline(t *testing.T) {
	if _, err := exec.LookPath("ffmpeg"); err != nil {
		t.Skip("ffmpeg not available")
	}
	if _, err := exec.LookPath("ffprobe"); err != nil {
		t.Skip("ffprobe not available")
	}

	inputDir := t.TempDir()
	outputDir := t.TempDir()

	// Generate two 1-second synthetic video files.
	for _, name := range []string{"Show S01E01.mp4", "Movie (2023).mp4"} {
		path := filepath.Join(inputDir, name)
		gen := exec.Command("ffmpeg",
			"-f", "lavfi", "-i", "testsrc=duration=1:size=1280x720:rate=24",
			"-f", "lavfi", "-i", "sine=frequency=440:duration=1:sample_rate=48000",
			"-c:v", "libx264", "-profile:v", "high", "-pix_fmt", "yuv420p",
			"-c:a", "aac", "-ac", "2",
			"-y", path,
		)
		gen.Stderr = os.Stderr
		if err := gen.Run(); err != nil {
			t.Fatalf("generate %s: %v", name, err)
		}
	}

	// Also create an extras dir that should be excluded.
	os.MkdirAll(filepath.Join(inputDir, "Extras"), 0o755)
	genExtras := exec.Command("ffmpeg",
		"-f", "lavfi", "-i", "testsrc=duration=1:size=320x240:rate=24",
		"-c:v", "libx264", "-pix_fmt", "yuv420p",
		"-y", filepath.Join(inputDir, "Extras", "bonus.mp4"),
	)
	genExtras.Stderr = os.Stderr
	genExtras.Run()

	cfg := config.DefaultConfig()
	cfg.InputDir = inputDir
	cfg.OutputDir = outputDir
	cfg.DryRun = true
	cfg.ColorMode = config.ColorNever

	log, err := logging.NewLogger(&cfg)
	if err != nil {
		t.Fatalf("NewLogger: %v", err)
	}
	defer log.Close()

	stats := Run(context.Background(), &cfg, log)

	t.Logf("Total=%d Encoded=%d Skipped=%d Failed=%d",
		stats.Total, stats.Encoded, stats.Skipped, stats.Failed)

	if stats.Total != 2 {
		t.Errorf("Total: got %d, want 2 (extras should be excluded)", stats.Total)
	}
	if stats.Encoded != 2 {
		t.Errorf("Encoded: got %d, want 2 (dry-run should count as encoded)", stats.Encoded)
	}
	if stats.Failed != 0 {
		t.Errorf("Failed: got %d, want 0", stats.Failed)
	}
}

// --- Helpers ---

func touch(t *testing.T, dir, name string) {
	t.Helper()
	path := filepath.Join(dir, name)
	if err := os.WriteFile(path, []byte{}, 0o644); err != nil {
		t.Fatalf("touch %s: %v", path, err)
	}
}

func basenames(paths []string) []string {
	out := make([]string, len(paths))
	for i, p := range paths {
		out[i] = filepath.Base(p)
	}
	return out
}

func sliceEqual(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if !strings.EqualFold(a[i], b[i]) {
			return false
		}
	}
	return true
}
