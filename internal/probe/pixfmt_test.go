package probe

import "testing"

func TestPixFmtIs10Bit(t *testing.T) {
	tests := []struct {
		pixFmt string
		want   bool
	}{
		{"yuv420p", false},
		{"yuv420p10le", true},
		{"yuv420p10be", true},
		{"yuv444p10le", true},
		{"p010le", true},
		{"p010", true},
		{"", false},
		{"  yuv422p10le  ", true},
	}
	for _, tt := range tests {
		if got := PixFmtIs10Bit(tt.pixFmt); got != tt.want {
			t.Errorf("PixFmtIs10Bit(%q) = %v, want %v", tt.pixFmt, got, tt.want)
		}
	}
}
