// Package check provides system diagnostics (--check mode) and pre-pipeline
// dependency validation (CheckDeps) for ffmpeg, ffprobe, VAAPI, x265, and AAC.
package check

import (
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/backmassage/muxmaster/internal/config"
)

// Sentinel errors returned by CheckDeps when a required tool or encoder is missing.
var (
	ErrFfmpegNotFound  = errors.New("ffmpeg not found on PATH")
	ErrFfprobeNotFound = errors.New("ffprobe not found on PATH")
	ErrNoVAAPIDevice   = errors.New("no VAAPI render device found in /dev/dri/")
	ErrVAAPITestFailed = errors.New("VAAPI test encode failed (device exists but hevc_vaapi unusable)")
	ErrCPUEncodeFailed = errors.New("CPU mode selected but libx265 test encode failed")
)

// Logger is the minimal logging interface needed by RunCheck.
// Defined here (rather than importing the logging package) so that check
// remains dependency-light and testable with a mock logger.
type Logger interface {
	Info(string, ...interface{})
	Success(string, ...interface{})
	Warn(string, ...interface{})
	Error(string, ...interface{})
	Debug(bool, string, ...interface{})
}

// RunCheck runs the interactive --check flow: prints availability of ffmpeg,
// ffprobe, HEVC encoders, VAAPI device/test, CPU x265, and AAC encoder.
// This is informational only — it does not stop on failure.
func RunCheck(cfg *config.Config, log Logger) {
	log.Info("=== System Check ===")

	checkFfmpeg(log)
	checkHEVCEncoders(log)
	checkVAAPI(log)
	checkCPUx265(log)
	checkAAC(log)
}

// checkFfmpeg verifies ffmpeg is on PATH and logs its version string.
func checkFfmpeg(log Logger) {
	if _, err := exec.LookPath("ffmpeg"); err != nil {
		log.Error("ffmpeg not found")
		return
	}
	cmd := exec.Command("ffmpeg", "-version")
	out, err := cmd.Output()
	if err != nil {
		log.Warn("ffmpeg found but -version failed: %v", err)
		return
	}
	firstLine := strings.TrimSpace(string(out))
	if idx := strings.Index(firstLine, "\n"); idx > 0 {
		firstLine = firstLine[:idx]
	}
	log.Success("ffmpeg: %s", firstLine)
}

// checkHEVCEncoders lists all HEVC-related encoders reported by ffmpeg.
func checkHEVCEncoders(log Logger) {
	log.Info("HEVC encoders:")
	cmd := exec.Command("ffmpeg", "-hide_banner", "-encoders")
	out, err := cmd.Output()
	if err != nil {
		log.Warn("Could not list encoders: %v", err)
		return
	}
	for _, line := range strings.Split(string(out), "\n") {
		lower := strings.ToLower(line)
		if strings.Contains(lower, "hevc") || strings.Contains(lower, "265") {
			log.Info("  %s", strings.TrimSpace(line))
		}
	}
}

// checkVAAPI finds the first render device and runs a minimal VAAPI encode test.
func checkVAAPI(log Logger) {
	dev := getFirstRenderDevice()
	if dev == "" {
		log.Warn("No VAAPI device found")
		return
	}
	log.Info("Testing VAAPI on %s...", dev)
	if testVAAPI(dev, "p010", "main10") {
		log.Success("VAAPI works (main10)")
	} else if testVAAPI(dev, "nv12", "main") {
		log.Success("VAAPI works (main/8-bit only)")
	} else {
		log.Error("VAAPI test encode failed on %s", dev)
	}
}

// checkCPUx265 runs a minimal libx265 encode to verify CPU encoding works.
func checkCPUx265(log Logger) {
	log.Info("Testing CPU x265...")
	if runSilent("ffmpeg", cpuTestArgs()...) {
		log.Success("CPU x265 works")
	} else {
		log.Error("CPU x265 test encode failed")
	}
}

// checkAAC runs a minimal AAC encode to verify the audio encoder works.
func checkAAC(log Logger) {
	log.Info("Testing AAC encoder...")
	if runSilent("ffmpeg",
		"-hide_banner", "-nostdin",
		"-f", "lavfi", "-i", "sine=frequency=1000:duration=0.1",
		"-c:a", "aac", "-f", "null", "-",
	) {
		log.Success("AAC encoder works")
	} else {
		log.Error("AAC encoder test failed")
	}
}

// CheckDeps is the pre-pipeline validation: it verifies that ffmpeg and
// ffprobe are on PATH and that the chosen encoder mode actually works.
// In CPU mode a quick libx265 encode is run; in VAAPI mode a render device
// must exist and pass a short encode test. Returns a sentinel error on failure.
func CheckDeps(cfg *config.Config) error {
	if _, err := exec.LookPath("ffmpeg"); err != nil {
		return ErrFfmpegNotFound
	}
	if _, err := exec.LookPath("ffprobe"); err != nil {
		return ErrFfprobeNotFound
	}

	if cfg.EncoderMode == config.EncoderCPU {
		if !runSilent("ffmpeg", cpuTestArgs()...) {
			return ErrCPUEncodeFailed
		}
		return nil
	}

	// VAAPI mode: need a render device that passes an encode test.
	dev := getFirstRenderDevice()
	if dev == "" {
		return ErrNoVAAPIDevice
	}
	if testVAAPI(dev, "p010", "main10") || testVAAPI(dev, "nv12", "main") {
		return nil
	}
	return ErrVAAPITestFailed
}

// --- internal helpers ---

// getFirstRenderDevice returns the first available /dev/dri/renderD* path,
// or empty string if none exist.
func getFirstRenderDevice() string {
	matches, _ := filepath.Glob("/dev/dri/renderD*")
	for _, m := range matches {
		if _, err := os.Stat(m); err == nil {
			return m
		}
	}
	return ""
}

// testVAAPI runs a minimal ffmpeg VAAPI encode to verify the device supports
// the given pixel format and HEVC profile.
func testVAAPI(device, swFormat, profile string) bool {
	return runSilent("ffmpeg",
		"-hide_banner", "-nostdin", "-loglevel", "error",
		"-init_hw_device", "vaapi=va:"+device,
		"-filter_hw_device", "va",
		"-f", "lavfi", "-i", "color=black:s=256x256:d=0.1",
		"-vf", "format="+swFormat+",hwupload",
		"-c:v", "hevc_vaapi", "-profile:v", profile,
		"-f", "null", "-",
	)
}

// cpuTestArgs returns the ffmpeg arguments for a minimal libx265 test encode.
// Shared by checkCPUx265 and CheckDeps to avoid duplicating the argument list.
func cpuTestArgs() []string {
	return []string{
		"-hide_banner", "-nostdin", "-loglevel", "error",
		"-f", "lavfi", "-i", "color=black:s=256x256:d=0.1",
		"-c:v", "libx265",
		"-f", "null", "-",
	}
}

// runSilent runs a command and returns true if it exits with status 0.
// Both stdout and stderr are discarded.
func runSilent(name string, args ...string) bool {
	cmd := exec.Command(name, args...)
	cmd.Stdout = nil
	cmd.Stderr = nil
	return cmd.Run() == nil
}
