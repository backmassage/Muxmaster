// Package check provides system diagnostics (--check mode) and pre-pipeline
// dependency validation (CheckDeps) for ffmpeg, ffprobe, video encoders,
// and the configured AAC encoder.
package check

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/backmassage/muxmaster/internal/config"
)

// Sentinel errors returned by CheckDeps when a required tool or encoder is missing.
var (
	ErrFfmpegNotFound    = errors.New("ffmpeg not found on PATH")
	ErrFfprobeNotFound   = errors.New("ffprobe not found on PATH")
	ErrNoVAAPIDevice     = errors.New("no VAAPI render device found in /dev/dri/")
	ErrVAAPITestFailed   = errors.New("VAAPI test encode failed (device exists but hevc_vaapi unusable)")
	ErrCPUEncodeFailed   = errors.New("CPU mode selected but libx265 test encode failed")
	ErrAudioEncodeFailed = errors.New("configured AAC encoder test failed")
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
// Returns true if all critical checks passed (ffmpeg, ffprobe, and at least
// one working encoder), false if any critical check failed.
func RunCheck(cfg *config.Config, log Logger) bool {
	log.Info("=== System Check ===")

	ok := true
	if !checkFfmpeg(log) {
		ok = false
	}
	checkHEVCEncoders(log)
	if !checkVAAPI(log) {
		ok = false
	}
	if !checkCPUx265(log) {
		ok = false
	}
	if !checkAudioEncoder(log, cfg.AudioEncoder) {
		ok = false
	}
	return ok
}

// checkFfmpeg verifies ffmpeg and ffprobe are on PATH and logs the ffmpeg version string.
// Returns true if both are found.
func checkFfmpeg(log Logger) bool {
	ok := true
	if _, err := exec.LookPath("ffmpeg"); err != nil {
		log.Error("ffmpeg not found")
		ok = false
	} else {
		cmd := exec.Command("ffmpeg", "-version")
		out, err := cmd.Output()
		if err != nil {
			log.Warn("ffmpeg found but -version failed: %v", err)
		} else {
			firstLine := strings.TrimSpace(string(out))
			if idx := strings.Index(firstLine, "\n"); idx > 0 {
				firstLine = firstLine[:idx]
			}
			log.Success("ffmpeg: %s", firstLine)
		}
	}
	if _, err := exec.LookPath("ffprobe"); err != nil {
		log.Error("ffprobe not found")
		ok = false
	} else {
		log.Success("ffprobe: found")
	}
	return ok
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
// Returns true if VAAPI works, false otherwise. A missing VAAPI device is not
// fatal (CPU mode may be used instead), so this is logged as a warning.
func checkVAAPI(log Logger) bool {
	dev := getFirstRenderDevice()
	if dev == "" {
		log.Warn("No VAAPI device found")
		return false
	}
	log.Info("Testing VAAPI on %s...", dev)
	if testVAAPI(dev, "p010", "main10") {
		log.Success("VAAPI works (main10)")
		return true
	}
	if testVAAPI(dev, "nv12", "main") {
		log.Success("VAAPI works (main/8-bit only)")
		return true
	}
	log.Error("VAAPI test encode failed on %s", dev)
	return false
}

// checkCPUx265 runs a minimal libx265 encode to verify CPU encoding works.
// Returns true on success.
func checkCPUx265(log Logger) bool {
	log.Info("Testing CPU x265...")
	if runSilent("ffmpeg", cpuTestArgs()...) {
		log.Success("CPU x265 works")
		return true
	}
	log.Error("CPU x265 test encode failed")
	return false
}

// checkAudioEncoder runs a minimal AAC encode to verify the encoder works.
// Returns true on success.
func checkAudioEncoder(log Logger, encoder string) bool {
	log.Info("Testing AAC encoder (%s)...", encoder)
	if runSilent("ffmpeg",
		"-hide_banner", "-nostdin",
		"-f", "lavfi", "-i", "sine=frequency=1000:duration=0.1",
		"-c:a", encoder, "-f", "null", "-",
	) {
		log.Success("AAC encoder works (%s)", encoder)
		return true
	}
	log.Error("AAC encoder test failed (%s)", encoder)
	return false
}

// CheckDeps is the pre-pipeline validation: it verifies that ffmpeg and
// ffprobe are on PATH and that the chosen encoder mode actually works.
// In CPU mode a quick libx265 encode is run; in VAAPI mode a render device
// must exist and pass a short encode test. On success in VAAPI mode, the
// derived profile and software format are written back to cfg so the builder
// and filter chain use the correct values.
func CheckDeps(cfg *config.Config) error {
	if _, err := exec.LookPath("ffmpeg"); err != nil {
		return ErrFfmpegNotFound
	}
	if _, err := exec.LookPath("ffprobe"); err != nil {
		return ErrFfprobeNotFound
	}
	if !testAudioEncoder(cfg.AudioEncoder) {
		return fmt.Errorf("%w: %s", ErrAudioEncodeFailed, cfg.AudioEncoder)
	}

	if cfg.EncoderMode == config.EncoderCPU {
		if !runSilent("ffmpeg", cpuTestArgs()...) {
			return ErrCPUEncodeFailed
		}
		return nil
	}

	// VAAPI mode: need a render device that passes an encode test.
	// Prefer 10-bit (main10/p010); fall back to 8-bit (main/nv12).
	dev := getFirstRenderDevice()
	if dev == "" {
		return ErrNoVAAPIDevice
	}
	if testVAAPI(dev, "p010", "main10") {
		cfg.VaapiProfile = "main10"
		cfg.VaapiSwFormat = "p010"
		return nil
	}
	if testVAAPI(dev, "nv12", "main") {
		cfg.VaapiProfile = "main"
		cfg.VaapiSwFormat = "nv12"
		return nil
	}
	return ErrVAAPITestFailed
}

func testAudioEncoder(encoder string) bool {
	return runSilent("ffmpeg",
		"-hide_banner", "-nostdin", "-loglevel", "error",
		"-f", "lavfi", "-i", "sine=frequency=1000:duration=0.1",
		"-c:a", encoder, "-f", "null", "-",
	)
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
