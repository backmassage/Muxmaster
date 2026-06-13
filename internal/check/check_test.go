// check_test.go covers pure-logic helpers in the check package.
package check

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/backmassage/muxmaster/internal/config"
)

// TestPictTypesContainB table-drives the ffprobe pict_type parsing that backs
// the B-frame capability probe: a B anywhere → true (Intel iHD); only I/P →
// false (AMD VCN, which accepts -bf but emits no HEVC B-frames).
func TestPictTypesContainB(t *testing.T) {
	cases := []struct {
		name string
		in   string
		want bool
	}{
		{"only I and P (AMD VCN)", "I\nP\nP\nP\nI\nP\n", false},
		{"B present (Intel iHD)", "I\nP\nB\nP\nB\nP\n", true},
		{"single B", "B", true},
		{"empty", "", false},
		{"trailing whitespace around B", "I\n  B  \nP\n", true},
		{"lowercase b", "i\np\nb\n", true},
		{"no frames, blank lines", "\n\n\n", false},
		{"crlf line endings with B", "I\r\nB\r\n", true},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := pictTypesContainB(c.in); got != c.want {
				t.Errorf("pictTypesContainB(%q) = %v, want %v", c.in, got, c.want)
			}
		})
	}
}

func TestSelectVaapiDevicePrefersConfiguredExistingPath(t *testing.T) {
	dir := t.TempDir()
	dev := filepath.Join(dir, "renderD999")
	if err := os.WriteFile(dev, []byte{}, 0o644); err != nil {
		t.Fatalf("write fake device: %v", err)
	}

	cfg := config.DefaultConfig()
	cfg.Encoder.VaapiDevice = dev

	if got := selectVaapiDevice(&cfg); got != dev {
		t.Fatalf("selectVaapiDevice = %q, want configured device %q", got, dev)
	}
}
