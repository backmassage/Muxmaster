package config

import "testing"

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
	cfg.AudioBitrate = "192"
	cfg.CheckOnly = true

	if err := cfg.Validate(); err != nil {
		t.Fatalf("Validate() returned error: %v", err)
	}
	if cfg.AudioBitrate != "192k" {
		t.Fatalf("Validate() did not normalize bitrate, got %q", cfg.AudioBitrate)
	}
}

func TestDefaultConfigAudioEncoder(t *testing.T) {
	cfg := DefaultConfig()
	if cfg.AudioEncoder != "libfdk_aac" {
		t.Fatalf("AudioEncoder = %q, want libfdk_aac", cfg.AudioEncoder)
	}
}
