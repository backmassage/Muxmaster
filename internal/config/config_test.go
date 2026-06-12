package config

import (
	"os"
	"strings"
	"testing"
)

func TestNormalizeAudioBitrate(t *testing.T) {
	tests := []struct {
		name    string
		in      string
		want    string
		wantErr bool
	}{
		{name: "plain number", in: "128", want: "128k"},
		{name: "k suffix", in: "256k", want: "256k"},
		{name: "upper K suffix", in: "320K", want: "320k"},
		{name: "kbps suffix", in: "192kbps", want: "192k"},
		{name: "trim spaces", in: "  160k  ", want: "160k"},
		{name: "empty", in: "", wantErr: true},
		{name: "zero", in: "0", wantErr: true},
		{name: "negative", in: "-64k", wantErr: true},
		{name: "nonnumeric", in: "fast", wantErr: true},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got, err := normalizeAudioBitrate(tc.in)
			if tc.wantErr {
				if err == nil {
					t.Fatalf("expected error for input %q, got nil", tc.in)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error for input %q: %v", tc.in, err)
			}
			if got != tc.want {
				t.Fatalf("normalizeAudioBitrate(%q) = %q, want %q", tc.in, got, tc.want)
			}
		})
	}
}

func TestValidateNormalizesAudioBitrate(t *testing.T) {
	cfg := DefaultConfig()
	cfg.Audio.Bitrate = "192"
	cfg.CheckOnly = true

	if err := cfg.Validate(); err != nil {
		t.Fatalf("Validate() returned error: %v", err)
	}
	if cfg.Audio.Bitrate != "192k" {
		t.Fatalf("Validate() did not normalize bitrate, got %q", cfg.Audio.Bitrate)
	}
}

func TestParseFlagsAnalyzeRequiresExactlyOneInput(t *testing.T) {
	tests := []struct {
		name      string
		args      []string
		wantInput string
		wantErr   string
	}{
		{
			name:    "missing input",
			args:    []string{"--analyze"},
			wantErr: "--analyze requires exactly one input directory",
		},
		{
			name:      "one input",
			args:      []string{"--analyze", "/media/library/"},
			wantInput: "/media/library",
		},
		{
			name:    "extra output rejected",
			args:    []string{"--analyze", "/media/library", "/out/library"},
			wantErr: "--analyze requires exactly one input directory",
		},
	}

	oldArgs := os.Args
	defer func() { os.Args = oldArgs }()

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			os.Args = append([]string{"muxmaster"}, tc.args...)
			cfg := DefaultConfig()

			err := ParseFlags(&cfg, "test", "deadbeef")
			if tc.wantErr != "" {
				if err == nil || !strings.Contains(err.Error(), tc.wantErr) {
					t.Fatalf("ParseFlags error = %v, want containing %q", err, tc.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatalf("ParseFlags returned error: %v", err)
			}
			if cfg.InputDir != tc.wantInput {
				t.Fatalf("InputDir = %q, want %q", cfg.InputDir, tc.wantInput)
			}
		})
	}
}

func TestDefaultConfigAudioEncoder(t *testing.T) {
	cfg := DefaultConfig()
	if cfg.Audio.Encoder != "libfdk_aac" {
		t.Fatalf("Audio.Encoder = %q, want libfdk_aac", cfg.Audio.Encoder)
	}
}

func TestValidatePaths(t *testing.T) {
	cfg := DefaultConfig()
	tests := []struct {
		name      string
		inputAbs  string
		outputAbs string
		wantErr   bool
	}{
		{
			name:      "equal",
			inputAbs:  "/media/library",
			outputAbs: "/media/library",
			wantErr:   true,
		},
		{
			name:      "child",
			inputAbs:  "/media/library",
			outputAbs: "/media/library/encoded",
			wantErr:   true,
		},
		{
			name:      "sibling with shared prefix",
			inputAbs:  "/media/library",
			outputAbs: "/media/library-encoded",
			wantErr:   false,
		},
		{
			name:      "root input contains all absolute outputs",
			inputAbs:  "/",
			outputAbs: "/encoded",
			wantErr:   true,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			err := cfg.ValidatePaths(tc.inputAbs, tc.outputAbs)
			if tc.wantErr && err == nil {
				t.Fatal("expected error, got nil")
			}
			if !tc.wantErr && err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
		})
	}
}

func TestValidate_VaapiRC(t *testing.T) {
	cfg := DefaultConfig()
	cfg.CheckOnly = true

	if cfg.Encoder.VaapiRC != VaapiRCCQP {
		t.Errorf("default VaapiRC should be cqp, got %q", cfg.Encoder.VaapiRC)
	}
	if err := cfg.Validate(); err != nil {
		t.Errorf("default config should validate: %v", err)
	}

	cfg.Encoder.VaapiRC = VaapiRCQVBR
	if err := cfg.Validate(); err != nil {
		t.Errorf("qvbr should validate: %v", err)
	}

	cfg.Encoder.VaapiRC = "vbr"
	if err := cfg.Validate(); err == nil {
		t.Error("invalid rate control mode should fail validation")
	}

	cfg.Encoder.VaapiRC = VaapiRCCQP
	cfg.Encoder.VaapiCompressionLevel = -1
	if err := cfg.Validate(); err == nil {
		t.Error("negative compression level should fail validation")
	}
}
