package display

import (
	"testing"
)

func TestFormatBytes(t *testing.T) {
	tests := []struct {
		name  string
		bytes int64
		want  string
	}{
		{"zero", 0, "0 B"},
		{"small bytes", 512, "512 B"},
		{"exactly 1 KiB", 1024, "1.0 KiB"},
		{"1.5 KiB", 1536, "1.5 KiB"},
		{"1 MiB", 1024 * 1024, "1.0 MiB"},
		{"1 GiB", 1024 * 1024 * 1024, "1.0 GiB"},
		{"typical file 700 MiB", 734003200, "700.0 MiB"},
		{"4.7 GiB", 5046586572, "4.7 GiB"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := FormatBytes(tt.bytes)
			if got != tt.want {
				t.Errorf("FormatBytes(%d) = %q, want %q", tt.bytes, got, tt.want)
			}
		})
	}
}

func TestFormatBytesWithSign(t *testing.T) {
	tests := []struct {
		name  string
		bytes int64
		want  string
	}{
		{"positive", 1024 * 1024, "+ 1.0 MiB"},
		{"negative", -1024 * 1024, "- 1.0 MiB"},
		{"zero", 0, "0 B"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := FormatBytesWithSign(tt.bytes)
			if got != tt.want {
				t.Errorf("FormatBytesWithSign(%d) = %q, want %q", tt.bytes, got, tt.want)
			}
		})
	}
}

func TestFormatBitrateLabel(t *testing.T) {
	tests := []struct {
		name string
		kbps int64
		want string
	}{
		{"sub-megabit", 800, "800 kbps"},
		{"exactly 1 Mbps", 1000, "1.0 Mbps"},
		{"typical video", 5000, "5.0 Mbps"},
		{"high bitrate", 25000, "25.0 Mbps"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := FormatBitrateLabel(tt.kbps)
			if got != tt.want {
				t.Errorf("FormatBitrateLabel(%d) = %q, want %q", tt.kbps, got, tt.want)
			}
		})
	}
}
