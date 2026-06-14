package pipeline

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/backmassage/muxmaster/internal/config"
	"github.com/backmassage/muxmaster/internal/ffmpeg"
	"github.com/backmassage/muxmaster/internal/logging"
	"github.com/backmassage/muxmaster/internal/naming"
	"github.com/backmassage/muxmaster/internal/planner"
	"github.com/backmassage/muxmaster/internal/tune"
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

	// All extras-category folders should be pruned.
	for _, name := range []string{"Extras", "extras", "Extra", "Bonus", "Featurettes"} {
		sub := filepath.Join(dir, name)
		os.MkdirAll(sub, 0o755)
		touch(t, sub, "bonus.mkv")
	}

	// Specials-category folders should NOT be pruned (they contain real media).
	for _, name := range []string{"Specials", "NCOP", "NCED"} {
		sub := filepath.Join(dir, name)
		os.MkdirAll(sub, 0o755)
		touch(t, sub, "ep.mkv")
	}

	files, err := Discover(dir)
	if err != nil {
		t.Fatalf("Discover: %v", err)
	}

	// Should find main.mkv + 3 files from specials-category folders = 4
	if len(files) != 4 {
		names := basenames(files)
		t.Errorf("got %d files %v, want 4 (extras pruned, specials kept)", len(files), names)
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
	cfg.Display.ColorMode = config.ColorNever

	log, err := logging.NewLogger(&cfg)
	if err != nil {
		t.Fatalf("NewLogger: %v", err)
	}
	defer log.Close()

	noExec := ffmpeg.RunFunc(func(_ context.Context, args []string) ffmpeg.ExecResult {
		t.Fatalf("unexpected ffmpeg execution in dry run: %v", args)
		return ffmpeg.ExecResult{}
	})

	stats := Run(context.Background(), &cfg, log, noExec)

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

func TestRunAutoTuneDryRunSkipsPrepassEvenTTY(t *testing.T) {
	requireFfmpegProbe(t)

	inputDir := t.TempDir()
	outputDir := t.TempDir()
	generateSyntheticMP4(t, filepath.Join(inputDir, "Show S01E01.mp4"))

	var detects int
	withAutoTuneDeps(t, autoTuneDeps{
		isTerminal: func() bool { return true },
		detectContent: func(context.Context, string, float64) (tune.ContentSignal, error) {
			detects++
			return tune.ContentSignal{Grain: 0.90, Frames: 4}, nil
		},
	})

	cfg := config.DefaultConfig()
	cfg.InputDir = inputDir
	cfg.OutputDir = outputDir
	cfg.DryRun = true
	cfg.Display.ColorMode = config.ColorNever

	noExec := ffmpeg.RunFunc(func(_ context.Context, args []string) ffmpeg.ExecResult {
		t.Fatalf("unexpected ffmpeg execution in dry run: %v", args)
		return ffmpeg.ExecResult{}
	})

	stats := Run(context.Background(), &cfg, nopLogger{}, noExec)
	if detects != 0 {
		t.Fatalf("dry-run should not run auto-tune pre-pass, got %d calls", detects)
	}
	if stats.Encoded != 1 || stats.Failed != 0 {
		t.Fatalf("stats = %+v, want one dry-run encoded and no failures", stats)
	}
}

func TestRunAutoTuneSkipsExistingOutputCandidate(t *testing.T) {
	requireFfmpegProbe(t)

	inputDir := t.TempDir()
	outputDir := t.TempDir()
	input := filepath.Join(inputDir, "Show S01E01.mp4")
	generateSyntheticMP4(t, input)

	cfg := config.DefaultConfig()
	cfg.InputDir = inputDir
	cfg.OutputDir = outputDir
	cfg.Display.ColorMode = config.ColorNever

	files := []string{input}
	yearIndex := naming.BuildYearVariantIndex(files)
	out := predictedOutputPath(&cfg, input, yearIndex)
	if err := os.MkdirAll(filepath.Dir(out), 0o755); err != nil {
		t.Fatalf("mkdir output: %v", err)
	}
	if err := os.WriteFile(out, []byte("already encoded"), 0o644); err != nil {
		t.Fatalf("write output: %v", err)
	}

	var detects int
	withAutoTuneDeps(t, autoTuneDeps{
		isTerminal: func() bool { return true },
		detectContent: func(context.Context, string, float64) (tune.ContentSignal, error) {
			detects++
			return tune.ContentSignal{Grain: 0.90, Frames: 4}, nil
		},
	})

	noExec := ffmpeg.RunFunc(func(_ context.Context, args []string) ffmpeg.ExecResult {
		t.Fatalf("skip-existing file should not execute ffmpeg: %v", args)
		return ffmpeg.ExecResult{}
	})

	stats := Run(context.Background(), &cfg, nopLogger{}, noExec)
	if detects != 0 {
		t.Fatalf("existing output should not run auto-tune pre-pass, got %d calls", detects)
	}
	if stats.Skipped != 1 || stats.Failed != 0 {
		t.Fatalf("stats = %+v, want one skipped existing output and no failures", stats)
	}
}

func TestRunAutoTuneAppliesResolvedTuneToPlan(t *testing.T) {
	requireFfmpegProbe(t)

	inputDir := t.TempDir()
	outputDir := t.TempDir()
	generateSyntheticMP4(t, filepath.Join(inputDir, "Show S01E01.mp4"))

	var detects int
	var prompt strings.Builder
	withAutoTuneDeps(t, autoTuneDeps{
		isTerminal: func() bool { return true },
		reader:     strings.NewReader("\n"),
		writer:     &prompt,
		probeDuration: func(context.Context, string) (float64, error) {
			return 300, nil
		},
		detectContent: func(context.Context, string, float64) (tune.ContentSignal, error) {
			detects++
			return tune.ContentSignal{Grain: 0.90, Frames: 6}, nil
		},
	})

	cfg := config.DefaultConfig()
	cfg.InputDir = inputDir
	cfg.OutputDir = outputDir
	cfg.Display.ColorMode = config.ColorNever

	var gotArgs []string
	run := ffmpeg.RunFunc(func(_ context.Context, args []string) ffmpeg.ExecResult {
		gotArgs = append([]string(nil), args...)
		out := args[len(args)-1]
		if err := os.MkdirAll(filepath.Dir(out), 0o755); err != nil {
			return ffmpeg.ExecResult{Err: err}
		}
		if err := os.WriteFile(out, []byte("ok"), 0o644); err != nil {
			return ffmpeg.ExecResult{Err: err}
		}
		return ffmpeg.ExecResult{}
	})

	stats := Run(context.Background(), &cfg, nopLogger{}, run)
	if detects != 1 {
		t.Fatalf("detect calls: got %d, want 1", detects)
	}
	if stats.Encoded != 1 || stats.Failed != 0 {
		t.Fatalf("stats = %+v, want one encoded and no failures", stats)
	}
	if !strings.Contains(strings.Join(gotArgs, " "), "hqdn3d=4:4:9:9") {
		t.Fatalf("ffmpeg args did not include grain prefilter:\n%v", gotArgs)
	}
	if !strings.Contains(prompt.String(), "Enter = grain") {
		t.Fatalf("prompt did not default to grain:\n%s", prompt.String())
	}
}

func TestExecuteWithRetryQVBRSizeEscalationShrinksBitrateArgs(t *testing.T) {
	dir := t.TempDir()
	input := filepath.Join(dir, "input.mkv")
	output := filepath.Join(dir, "output.mkv")
	if err := os.WriteFile(input, make([]byte, 1000), 0o644); err != nil {
		t.Fatalf("write input: %v", err)
	}

	cfg := config.DefaultConfig()
	cfg.Encoder.Mode = config.EncoderVAAPI
	cfg.Encoder.VaapiDevice = "/dev/dri/renderD128"
	cfg.Encoder.VaapiProfile = "main10"
	cfg.Display.ColorMode = config.ColorNever

	plan := &planner.FilePlan{
		Action:             planner.ActionEncode,
		InputPath:          input,
		OutputPath:         output,
		VideoStreamIdx:     0,
		Audio:              planner.AudioPlan{NoAudio: true},
		MuxQueueSize:       4096,
		VaapiQP:            20,
		VaapiQVBR:          true,
		OptimalBitrateKbps: 1000,
		MaxRateKbps:        1200,
		BufSizeKbps:        2400,
	}
	rs := ffmpeg.NewRetryState(plan)

	var calls [][]string
	run := ffmpeg.RunFunc(func(_ context.Context, args []string) ffmpeg.ExecResult {
		calls = append(calls, append([]string(nil), args...))
		size := 800
		if len(calls) == 1 {
			size = 1200
		}
		if err := os.WriteFile(output, make([]byte, size), 0o644); err != nil {
			return ffmpeg.ExecResult{Err: err}
		}
		return ffmpeg.ExecResult{}
	})

	if ok := executeWithRetry(context.Background(), &cfg, nopLogger{}, plan, rs, run); !ok {
		t.Fatal("executeWithRetry returned false")
	}
	if len(calls) != 2 {
		t.Fatalf("ffmpeg calls: got %d, want 2", len(calls))
	}
	if got := argValue(calls[0], "-b:v"); got != "1000k" {
		t.Fatalf("first -b:v = %q, want 1000k", got)
	}
	if got := argValue(calls[1], "-global_quality"); got != "21" {
		t.Fatalf("second -global_quality = %q, want 21", got)
	}
	if got := argValue(calls[1], "-b:v"); got != "850k" {
		t.Fatalf("second -b:v = %q, want 850k", got)
	}
	if got := argValue(calls[1], "-maxrate"); got != "1020k" {
		t.Fatalf("second -maxrate = %q, want 1020k", got)
	}
	if got := argValue(calls[1], "-bufsize"); got != "2040k" {
		t.Fatalf("second -bufsize = %q, want 2040k", got)
	}
}

// TestExecuteWithRetrySubOnePercentBloatEscalates guards the size-trigger fix:
// an output that is larger than the input by less than 1% floors to pct==100
// under integer division, but must still trip the anti-bloat escalation (the
// trigger compares raw bytes, not the floored percentage).
func TestExecuteWithRetrySubOnePercentBloatEscalates(t *testing.T) {
	dir := t.TempDir()
	input := filepath.Join(dir, "input.mkv")
	output := filepath.Join(dir, "output.mkv")
	if err := os.WriteFile(input, make([]byte, 1000), 0o644); err != nil {
		t.Fatalf("write input: %v", err)
	}

	cfg := config.DefaultConfig()
	cfg.Encoder.Mode = config.EncoderCPU
	cfg.Display.ColorMode = config.ColorNever

	plan := &planner.FilePlan{
		Action:         planner.ActionEncode,
		InputPath:      input,
		OutputPath:     output,
		VideoStreamIdx: 0,
		Audio:          planner.AudioPlan{NoAudio: true},
		MuxQueueSize:   4096,
		CpuCRF:         22,
	}
	rs := ffmpeg.NewRetryState(plan)

	var calls int
	run := ffmpeg.RunFunc(func(_ context.Context, _ []string) ffmpeg.ExecResult {
		calls++
		size := 900 // second pass: comfortably under input
		if calls == 1 {
			size = 1001 // 1 byte over → floors to 100% but is genuinely larger
		}
		if err := os.WriteFile(output, make([]byte, size), 0o644); err != nil {
			return ffmpeg.ExecResult{Err: err}
		}
		return ffmpeg.ExecResult{}
	})

	if ok := executeWithRetry(context.Background(), &cfg, nopLogger{}, plan, rs, run); !ok {
		t.Fatal("executeWithRetry returned false")
	}
	if calls != 2 {
		t.Fatalf("ffmpeg calls: got %d, want 2 (sub-1%% bloat should escalate once)", calls)
	}
	if rs.CpuCRF != 23 {
		t.Fatalf("CpuCRF after escalation: got %d, want 23", rs.CpuCRF)
	}
}

// --- Helpers ---

type nopLogger struct{}

func (nopLogger) Info(string, ...interface{})    {}
func (nopLogger) Success(string, ...interface{}) {}
func (nopLogger) Warn(string, ...interface{})    {}
func (nopLogger) Error(string, ...interface{})   {}
func (nopLogger) Debug(bool, string, ...interface{}) {
}
func (nopLogger) Outlier(string, ...interface{}) {}
func (nopLogger) Blank()                         {}

func withAutoTuneDeps(t *testing.T, deps autoTuneDeps) {
	t.Helper()
	old := autoTune
	autoTune = deps
	t.Cleanup(func() { autoTune = old })
}

func requireFfmpegProbe(t *testing.T) {
	t.Helper()
	if _, err := exec.LookPath("ffmpeg"); err != nil {
		t.Skip("ffmpeg not available")
	}
	if _, err := exec.LookPath("ffprobe"); err != nil {
		t.Skip("ffprobe not available")
	}
}

func generateSyntheticMP4(t *testing.T, path string) {
	t.Helper()
	gen := exec.Command("ffmpeg",
		"-f", "lavfi", "-i", "testsrc=duration=1:size=320x240:rate=24",
		"-c:v", "libx264", "-profile:v", "high", "-pix_fmt", "yuv420p",
		"-y", path,
	)
	gen.Stderr = os.Stderr
	if err := gen.Run(); err != nil {
		t.Fatalf("generate %s: %v", path, err)
	}
}

func predictedOutputPath(cfg *config.Config, path string, yearIndex naming.YearVariantIndex) string {
	parsed := naming.ParseFilename(filepath.Base(path), filepath.Dir(path))
	if parsed.MediaType == naming.MediaTV {
		parsed.ShowName = naming.HarmonizeShowName(parsed.ShowName, yearIndex)
	}
	resolver := naming.NewCollisionResolver()
	out := naming.GetOutputPath(parsed, cfg.OutputDir, string(cfg.OutputContainer))
	return resolver.Resolve(path, out)
}

func argValue(args []string, flag string) string {
	for i := 0; i+1 < len(args); i++ {
		if args[i] == flag {
			return args[i+1]
		}
	}
	return ""
}

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
