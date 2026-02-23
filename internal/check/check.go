package check

import (
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/backmassage/muxmaster/internal/config"
)

var (
	ErrFfmpegNotFound   = errors.New("ffmpeg not found")
	ErrFfprobeNotFound  = errors.New("ffprobe not found")
	ErrNoVAAPIDevice    = errors.New("no VAAPI device")
	ErrVAAPITestFailed  = errors.New("VAAPI test failed")
	ErrCPUEncodeFailed  = errors.New("CPU mode selected but libx265 is unavailable")
)

// Logger interface used by RunCheck to avoid circular import from pipeline.
type Logger interface {
	Info(string, ...interface{})
	Success(string, ...interface{})
	Warn(string, ...interface{})
	Error(string, ...interface{})
	Debug(bool, string, ...interface{})
}

// RunCheck runs system diagnostics: ffmpeg, ffprobe, HEVC encoders, VAAPI, x265, AAC.
func RunCheck(cfg *config.Config, log Logger) {
	log.Info("=== System Check ===")

	// ffmpeg
	if _, err := exec.LookPath("ffmpeg"); err != nil {
		log.Error("ffmpeg not found")
	} else {
		cmd := exec.Command("ffmpeg", "-version")
		out, _ := cmd.Output()
		first := strings.TrimSpace(string(out))
		if idx := strings.Index(first, "\n"); idx > 0 {
			first = first[:idx]
		}
		log.Success("ffmpeg: %s", first)
	}

	// HEVC encoders
	log.Info("HEVC encoders:")
	cmd := exec.Command("ffmpeg", "-hide_banner", "-encoders")
	out, _ := cmd.Output()
	for _, line := range strings.Split(string(out), "\n") {
		if strings.Contains(strings.ToLower(line), "hevc") || strings.Contains(line, "265") {
			log.Info("  %s", strings.TrimSpace(line))
		}
	}

	// VAAPI device and test
	dev := getFirstRenderDevice()
	if dev != "" {
		log.Info("Testing VAAPI on %s...", dev)
		if testVAAPI(dev, "p010", "main10") {
			log.Success("VAAPI works (main10)")
		} else if testVAAPI(dev, "nv12", "main") {
			log.Success("VAAPI works (main/8-bit only)")
		} else {
			log.Error("VAAPI failed")
		}
	} else {
		log.Warn("No VAAPI device found")
	}

	// CPU x265
	log.Info("Testing CPU x265...")
	if runSilent("ffmpeg", "-hide_banner", "-nostdin", "-f", "lavfi", "-i", "color=black:s=256x256:d=0.1", "-c:v", "libx265", "-f", "null", "-") {
		log.Success("CPU x265 works")
	} else {
		log.Error("CPU x265 failed")
	}

	// AAC
	log.Info("Testing AAC encoder...")
	if runSilent("ffmpeg", "-hide_banner", "-nostdin", "-f", "lavfi", "-i", "sine=frequency=1000:duration=0.1", "-c:a", "aac", "-f", "null", "-") {
		log.Success("AAC encoder works")
	} else {
		log.Error("AAC encoder failed")
	}
}

func getFirstRenderDevice() string {
	matches, _ := filepath.Glob("/dev/dri/renderD*")
	for _, m := range matches {
		if _, err := os.Stat(m); err == nil {
			return m
		}
	}
	return ""
}

func testVAAPI(device, swFormat, profile string) bool {
	return runSilent("ffmpeg", "-hide_banner", "-nostdin", "-loglevel", "error",
		"-init_hw_device", "vaapi=va:"+device,
		"-filter_hw_device", "va",
		"-f", "lavfi", "-i", "color=black:s=256x256:d=0.1",
		"-vf", "format="+swFormat+",hwupload",
		"-c:v", "hevc_vaapi", "-profile:v", profile, "-f", "null", "-")
}

func runSilent(name string, args ...string) bool {
	cmd := exec.Command(name, args...)
	cmd.Stdout = nil
	cmd.Stderr = nil
	return cmd.Run() == nil
}

// CheckDeps verifies ffmpeg/ffprobe exist and (in VAAPI mode) device and profile. Returns error on failure.
func CheckDeps(cfg *config.Config) error {
	if _, err := exec.LookPath("ffmpeg"); err != nil {
		return ErrFfmpegNotFound
	}
	if _, err := exec.LookPath("ffprobe"); err != nil {
		return ErrFfprobeNotFound
	}
	if cfg.EncoderMode != config.EncoderVAAPI {
		if !runSilent("ffmpeg", "-hide_banner", "-nostdin", "-loglevel", "error",
			"-f", "lavfi", "-i", "color=black:s=256x256:d=0.1", "-c:v", "libx265", "-f", "null", "-") {
			return ErrCPUEncodeFailed
		}
		return nil
	}
	dev := getFirstRenderDevice()
	if dev == "" {
		return ErrNoVAAPIDevice
	}
	if testVAAPI(dev, "p010", "main10") || testVAAPI(dev, "nv12", "main") {
		return nil
	}
	return ErrVAAPITestFailed
}
