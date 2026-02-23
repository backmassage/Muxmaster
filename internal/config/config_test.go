package config

import (
	"os"
	"testing"
)

func TestDefaultConfig(t *testing.T) {
	c := DefaultConfig()
	if c.EncoderMode != EncoderVAAPI {
		t.Errorf("default EncoderMode: got %s", c.EncoderMode)
	}
	if c.VaapiQP != 19 || c.CpuCRF != 19 {
		t.Errorf("default quality: VaapiQP=%d CpuCRF=%d", c.VaapiQP, c.CpuCRF)
	}
	if c.OutputContainer != ContainerMKV {
		t.Errorf("default container: got %s", c.OutputContainer)
	}
	if !c.SkipHEVC || !c.SmartQuality || !c.CleanTimestamps {
		t.Errorf("default behavior flags: SkipHEVC=%v SmartQuality=%v CleanTimestamps=%v", c.SkipHEVC, c.SmartQuality, c.CleanTimestamps)
	}
	if c.SmartQualityBias != -1 || c.SmartQualityRetryStep != 2 {
		t.Errorf("default quality tuning: bias=%d step=%d", c.SmartQualityBias, c.SmartQualityRetryStep)
	}
}

func TestNormalizeDirArg(t *testing.T) {
	tests := []struct{ in, want string }{
		{"/", "/"},
		{"/foo/", "/foo"},
		{"/foo//", "/foo"},
		{"relative/", "relative"},
		{"no-slash", "no-slash"},
	}
	for _, tt := range tests {
		got := NormalizeDirArg(tt.in)
		if got != tt.want {
			t.Errorf("NormalizeDirArg(%q) = %q, want %q", tt.in, got, tt.want)
		}
	}
}

func TestValidate(t *testing.T) {
	valid := DefaultConfig()
	valid.CheckOnly = true
	if err := valid.Validate(); err != nil {
		t.Errorf("Validate(checkOnly): %v", err)
	}
	valid.CheckOnly = false
	valid.InputDir = "/in"
	valid.OutputDir = "/out"
	if err := valid.Validate(); err != nil {
		t.Errorf("Validate(with dirs): %v", err)
	}

	badMode := DefaultConfig()
	badMode.EncoderMode = "invalid"
	if err := badMode.Validate(); err == nil {
		t.Error("Validate(invalid mode): expected error")
	}

	badContainer := DefaultConfig()
	badContainer.OutputContainer = "avi"
	if err := badContainer.Validate(); err == nil {
		t.Error("Validate(invalid container): expected error")
	}

	needDirs := DefaultConfig()
	needDirs.CheckOnly = false
	if err := needDirs.Validate(); err == nil {
		t.Error("Validate(no dirs): expected error")
	}
}

func TestValidatePaths(t *testing.T) {
	cfg := DefaultConfig()
	in := "/home/media"
	outInside := "/home/media/out"
	if err := cfg.ValidatePaths(in, outInside); err == nil {
		t.Error("ValidatePaths(output inside input): expected error")
	}
	outSame := "/home/media"
	if err := cfg.ValidatePaths(in, outSame); err == nil {
		t.Error("ValidatePaths(same path): expected error")
	}
	outOutside := "/home/output"
	if err := cfg.ValidatePaths(in, outOutside); err != nil {
		t.Errorf("ValidatePaths(ok): %v", err)
	}
}

func TestParseFlags_CheckOnly(t *testing.T) {
	orig := os.Args
	defer func() { os.Args = orig }()
	os.Args = []string{"muxmaster", "-c"}
	cfg := DefaultConfig()
	err := ParseFlags(&cfg)
	if err != nil {
		t.Errorf("ParseFlags(-c): %v", err)
	}
	if !cfg.CheckOnly {
		t.Error("expected CheckOnly true")
	}
}

func TestParseFlags_QualityPrecedence(t *testing.T) {
	orig := os.Args
	defer func() { os.Args = orig }()

	// VAAPI: --vaapi-qp overrides --quality
	os.Args = []string{"muxmaster", "-m", "vaapi", "--vaapi-qp", "22", "-q", "20", "/in", "/out"}
	cfg := DefaultConfig()
	if err := ParseFlags(&cfg); err != nil {
		t.Fatalf("ParseFlags: %v", err)
	}
	if cfg.VaapiQP != 22 || cfg.ActiveQualityOverride != "22" {
		t.Errorf("VAAPI: expected QP=22 override=22, got QP=%d override=%q", cfg.VaapiQP, cfg.ActiveQualityOverride)
	}

	// VAAPI: --quality only
	os.Args = []string{"muxmaster", "-m", "vaapi", "-q", "21", "/in", "/out"}
	cfg = DefaultConfig()
	if err := ParseFlags(&cfg); err != nil {
		t.Fatalf("ParseFlags: %v", err)
	}
	if cfg.VaapiQP != 21 || cfg.ActiveQualityOverride != "21" {
		t.Errorf("VAAPI quality only: got QP=%d override=%q", cfg.VaapiQP, cfg.ActiveQualityOverride)
	}

	// CPU: --cpu-crf overrides --quality
	os.Args = []string{"muxmaster", "-m", "cpu", "--cpu-crf", "23", "-q", "20", "/in", "/out"}
	cfg = DefaultConfig()
	if err := ParseFlags(&cfg); err != nil {
		t.Fatalf("ParseFlags: %v", err)
	}
	if cfg.CpuCRF != 23 || cfg.ActiveQualityOverride != "23" {
		t.Errorf("CPU: expected CRF=23 override=23, got CRF=%d override=%q", cfg.CpuCRF, cfg.ActiveQualityOverride)
	}
}
