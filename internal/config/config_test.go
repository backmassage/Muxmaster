package config

import (
	"testing"
)

func TestNormalizeDirArg(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want string
	}{
		{"no trailing slash", "/media/library", "/media/library"},
		{"single trailing slash", "/media/library/", "/media/library"},
		{"multiple trailing slashes", "/media/library///", "/media/library"},
		{"root path", "/", "/"},
		{"relative path", "output", "output"},
		{"relative with slash", "output/", "output"},
		{"empty string", "", ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := NormalizeDirArg(tt.in)
			if got != tt.want {
				t.Errorf("NormalizeDirArg(%q) = %q, want %q", tt.in, got, tt.want)
			}
		})
	}
}

func TestValidate_EncoderMode(t *testing.T) {
	tests := []struct {
		name    string
		mode    EncoderMode
		wantErr bool
	}{
		{"vaapi is valid", EncoderVAAPI, false},
		{"cpu is valid", EncoderCPU, false},
		{"empty is invalid", "", true},
		{"unknown is invalid", "nvenc", true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := DefaultConfig()
			cfg.CheckOnly = true // skip path requirement
			cfg.EncoderMode = tt.mode
			err := cfg.Validate()
			if (err != nil) != tt.wantErr {
				t.Errorf("Validate() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestValidate_Container(t *testing.T) {
	tests := []struct {
		name    string
		ctr     Container
		wantErr bool
	}{
		{"mkv is valid", ContainerMKV, false},
		{"mp4 is valid", ContainerMP4, false},
		{"empty is invalid", "", true},
		{"avi is invalid", "avi", true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := DefaultConfig()
			cfg.CheckOnly = true
			cfg.OutputContainer = tt.ctr
			err := cfg.Validate()
			if (err != nil) != tt.wantErr {
				t.Errorf("Validate() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestValidate_HDRMode(t *testing.T) {
	tests := []struct {
		name    string
		hdr     HDRMode
		wantErr bool
	}{
		{"preserve is valid", HDRPreserve, false},
		{"tonemap is valid", HDRTonemap, false},
		{"empty is invalid", "", true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := DefaultConfig()
			cfg.CheckOnly = true
			cfg.HandleHDR = tt.hdr
			err := cfg.Validate()
			if (err != nil) != tt.wantErr {
				t.Errorf("Validate() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestValidate_RequiresPaths(t *testing.T) {
	cfg := DefaultConfig()
	cfg.CheckOnly = false
	cfg.InputDir = ""
	cfg.OutputDir = ""

	if err := cfg.Validate(); err == nil {
		t.Error("Validate() should fail when paths are empty and CheckOnly is false")
	}

	cfg.InputDir = "/in"
	cfg.OutputDir = "/out"
	if err := cfg.Validate(); err != nil {
		t.Errorf("Validate() unexpected error: %v", err)
	}
}

func TestValidate_CheckOnlySkipsPaths(t *testing.T) {
	cfg := DefaultConfig()
	cfg.CheckOnly = true
	cfg.InputDir = ""
	cfg.OutputDir = ""

	if err := cfg.Validate(); err != nil {
		t.Errorf("Validate() should pass with empty paths when CheckOnly is true, got: %v", err)
	}
}

func TestValidatePaths(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		output  string
		wantErr bool
	}{
		{"separate directories", "/media/in", "/media/out", false},
		{"output equals input", "/media/lib", "/media/lib", true},
		{"output inside input", "/media/lib", "/media/lib/output", true},
		{"output is parent of input", "/media/lib/sub", "/media/lib", false},
		{"similar prefix not nested", "/media/library", "/media/library2", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := DefaultConfig()
			err := cfg.ValidatePaths(tt.input, tt.output)
			if (err != nil) != tt.wantErr {
				t.Errorf("ValidatePaths(%q, %q) error = %v, wantErr %v",
					tt.input, tt.output, err, tt.wantErr)
			}
		})
	}
}

func TestDefaultConfig_SaneDefaults(t *testing.T) {
	cfg := DefaultConfig()

	if cfg.EncoderMode != EncoderVAAPI {
		t.Errorf("default EncoderMode = %q, want %q", cfg.EncoderMode, EncoderVAAPI)
	}
	if cfg.OutputContainer != ContainerMKV {
		t.Errorf("default OutputContainer = %q, want %q", cfg.OutputContainer, ContainerMKV)
	}
	if cfg.HandleHDR != HDRPreserve {
		t.Errorf("default HandleHDR = %q, want %q", cfg.HandleHDR, HDRPreserve)
	}
	if !cfg.SkipExisting {
		t.Error("default SkipExisting should be true")
	}
	if !cfg.SkipHEVC {
		t.Error("default SkipHEVC should be true")
	}
	if !cfg.SmartQuality {
		t.Error("default SmartQuality should be true")
	}
	if cfg.DryRun {
		t.Error("default DryRun should be false")
	}
}
