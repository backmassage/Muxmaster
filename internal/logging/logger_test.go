package logging

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"

	"github.com/backmassage/muxmaster/internal/config"
)

func TestNewLogger_NoFile(t *testing.T) {
	cfg := config.DefaultConfig()
	cfg.LogFile = ""
	l, err := NewLogger(&cfg)
	if err != nil {
		t.Fatal(err)
	}
	defer l.Close()
	l.Info("test message")
}

func TestNewLogger_WithFile(t *testing.T) {
	dir := t.TempDir()
	cfg := config.DefaultConfig()
	cfg.LogFile = filepath.Join(dir, "muxmaster.log")
	l, err := NewLogger(&cfg)
	if err != nil {
		t.Fatal(err)
	}
	l.Info("to file")
	if err := l.Close(); err != nil {
		t.Fatal(err)
	}
	b, _ := os.ReadFile(cfg.LogFile)
	if !bytes.Contains(b, []byte("INFO")) || !bytes.Contains(b, []byte("to file")) {
		t.Errorf("log file content: %s", string(b))
	}
}
