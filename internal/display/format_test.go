package display

import (
	"testing"
)

func TestFormatBytes(t *testing.T) {
	tests := []struct {
		n    int64
		want string
	}{
		{0, "0 B"},
		{500, "500 B"},
		{1024, "1.0 KiB"},
		{1536, "1.5 KiB"},
		{1024 * 1024, "1.0 MiB"},
		{1024 * 1024 * 1024, "1.0 GiB"},
		{1500 * 1024 * 1024, "1.5 GiB"},
	}
	for _, tt := range tests {
		got := FormatBytes(tt.n)
		if got != tt.want {
			t.Errorf("FormatBytes(%d) = %q, want %q", tt.n, got, tt.want)
		}
	}
}

func TestFormatBytesWithSign(t *testing.T) {
	if got := FormatBytesWithSign(-1024); got != "- 1.0 KiB" {
		t.Errorf("FormatBytesWithSign(-1024) = %q", got)
	}
	if got := FormatBytesWithSign(1024); got != "+ 1.0 KiB" {
		t.Errorf("FormatBytesWithSign(1024) = %q", got)
	}
	if got := FormatBytesWithSign(0); got != "0 B" {
		t.Errorf("FormatBytesWithSign(0) = %q", got)
	}
}

func TestFormatBitrateLabel(t *testing.T) {
	if got := FormatBitrateLabel(800); got != "800 kbps" {
		t.Errorf("FormatBitrateLabel(800) = %q", got)
	}
	if got := FormatBitrateLabel(1200); got != "1.2 Mbps" {
		t.Errorf("FormatBitrateLabel(1200) = %q", got)
	}
}
