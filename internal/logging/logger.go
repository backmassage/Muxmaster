// Package logging provides a leveled logger with optional file sink.
// ANSI colors are managed by [term.Configure]; the logger reads them
// from the [term] package at write time.
package logging

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/backmassage/muxmaster/internal/config"
	"github.com/backmassage/muxmaster/internal/term"
)

// Logger writes leveled messages to stdout/stderr and optionally to a log
// file. All write operations are serialized under a mutex for safe
// concurrent use.
type Logger struct {
	mu   sync.Mutex
	file *os.File
}

// NewLogger initializes terminal colors via [term.Configure] and opens a
// log file if cfg.LogFile is set. The caller must call [Logger.Close] when
// finished.
func NewLogger(cfg *config.Config) (*Logger, error) {
	term.Configure(cfg.ColorMode)

	l := &Logger{}
	if cfg.LogFile != "" {
		dir := filepath.Dir(cfg.LogFile)
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return nil, fmt.Errorf("create log directory: %w", err)
		}
		f, err := os.OpenFile(cfg.LogFile, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o644)
		if err != nil {
			return nil, fmt.Errorf("open log file: %w", err)
		}
		l.file = f
	}
	return l, nil
}

// Close flushes and closes the log file, if one was opened.
func (l *Logger) Close() error {
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.file != nil {
		err := l.file.Close()
		l.file = nil
		return err
	}
	return nil
}

// line writes a single timestamped log entry. ERROR goes to stderr; all
// others go to stdout. When a log file is open, the plain (uncolored) text
// is appended there as well.
func (l *Logger) line(level, ansiColor, text string) {
	ts := time.Now().Format("2006-01-02 15:04:05")
	plain := ts + " [" + level + "] " + text + "\n"

	l.mu.Lock()
	defer l.mu.Unlock()

	out := os.Stdout
	if level == "ERROR" {
		out = os.Stderr
	}

	if ansiColor != "" {
		_, _ = io.WriteString(out, ts+" "+ansiColor+"["+level+"]"+term.NC+" "+text+"\n")
	} else {
		_, _ = io.WriteString(out, plain)
	}

	if l.file != nil {
		_, _ = io.WriteString(l.file, plain)
	}
}

// Info logs an informational message (blue).
func (l *Logger) Info(format string, args ...interface{}) {
	l.line("INFO", term.Blue, fmt.Sprintf(format, args...))
}

// Success logs a success message (green).
func (l *Logger) Success(format string, args ...interface{}) {
	l.line("SUCCESS", term.Green, fmt.Sprintf(format, args...))
}

// Warn logs a warning (yellow).
func (l *Logger) Warn(format string, args ...interface{}) {
	l.line("WARN", term.Yellow, fmt.Sprintf(format, args...))
}

// Error logs an error (red) to stderr.
func (l *Logger) Error(format string, args ...interface{}) {
	l.line("ERROR", term.Red, fmt.Sprintf(format, args...))
}

// Render logs a render-plan message (magenta).
func (l *Logger) Render(format string, args ...interface{}) {
	l.line("RENDER", term.Magenta, fmt.Sprintf(format, args...))
}

// Outlier logs a bitrate-outlier message (orange).
func (l *Logger) Outlier(format string, args ...interface{}) {
	l.line("OUTLIER", term.Orange, fmt.Sprintf(format, args...))
}

// Debug logs a debug message (cyan) only when verbose is true.
func (l *Logger) Debug(verbose bool, format string, args ...interface{}) {
	if !verbose {
		return
	}
	l.line("DEBUG", term.Cyan, fmt.Sprintf(format, args...))
}
