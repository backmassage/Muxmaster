package logging

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/backmassage/muxmaster/internal/config"
)

// ANSI colors (empty when disabled)
var (
	Red     = ""
	Green   = ""
	Yellow  = ""
	Orange  = ""
	Blue    = ""
	Cyan    = ""
	Magenta = ""
	NC      = ""
)

// Logger provides leveled, optionally colored logging with optional file sink.
type Logger struct {
	mu       sync.Mutex
	color    bool
	file     *os.File
	filePath string
}

// NewLogger initializes colors from cfg and optionally opens logFile. Call Close() when done if LogFile was set.
func NewLogger(cfg *config.Config) (*Logger, error) {
	l := &Logger{}
	enable := false
	switch cfg.ColorMode {
	case config.ColorAlways:
		enable = true
	case config.ColorNever:
		enable = false
	case config.ColorAuto:
		enable = isTerminal(os.Stdout) && os.Getenv("NO_COLOR") == "" && strings.ToLower(os.Getenv("TERM")) != "dumb"
	}
	if enable {
		Red = "\033[1;91m"
		Green = "\033[1;92m"
		Yellow = "\033[1;93m"
		Orange = "\033[1;38;5;208m"
		Blue = "\033[1;94m"
		Cyan = "\033[1;96m"
		Magenta = "\033[1;95m"
		NC = "\033[0m"
	} else {
		Red, Green, Yellow, Orange, Blue, Cyan, Magenta, NC = "", "", "", "", "", "", "", ""
	}
	l.color = enable

	if cfg.LogFile != "" {
		dir := filepath.Dir(cfg.LogFile)
		if err := os.MkdirAll(dir, 0755); err != nil {
			return nil, err
		}
		f, err := os.OpenFile(cfg.LogFile, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0644)
		if err != nil {
			return nil, err
		}
		l.file = f
		l.filePath = cfg.LogFile
	}
	return l, nil
}

func isTerminal(f *os.File) bool {
	if f == nil {
		return false
	}
	fi, err := f.Stat()
	if err != nil {
		return false
	}
	return (fi.Mode() & os.ModeCharDevice) != 0
}

// Close closes the log file if one was opened.
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

func (l *Logger) line(level, color, text string) {
	ts := time.Now().Format("2006-01-02 15:04:05")
	l.mu.Lock()
	defer l.mu.Unlock()
	plain := ts + " [" + level + "] " + text + "\n"
	out := os.Stdout
	if level == "ERROR" {
		out = os.Stderr
	}
	if color != "" {
		_, _ = io.WriteString(out, ts+" "+color+"["+level+"]"+NC+" "+text+"\n")
	} else {
		_, _ = io.WriteString(out, plain)
	}
	if l.file != nil {
		_, _ = io.WriteString(l.file, plain)
	}
}

// Info logs at INFO level (blue).
func (l *Logger) Info(format string, args ...interface{}) {
	l.line("INFO", Blue, fmt.Sprintf(format, args...))
}

// Success logs at SUCCESS level (green).
func (l *Logger) Success(format string, args ...interface{}) {
	l.line("SUCCESS", Green, fmt.Sprintf(format, args...))
}

// Warn logs at WARN level (yellow).
func (l *Logger) Warn(format string, args ...interface{}) {
	l.line("WARN", Yellow, fmt.Sprintf(format, args...))
}

// Error logs at ERROR level (red), also to stderr.
func (l *Logger) Error(format string, args ...interface{}) {
	l.line("ERROR", Red, fmt.Sprintf(format, args...))
}

// Render logs at RENDER level (magenta).
func (l *Logger) Render(format string, args ...interface{}) {
	l.line("RENDER", Magenta, fmt.Sprintf(format, args...))
}

// Outlier logs at OUTLIER level (orange).
func (l *Logger) Outlier(format string, args ...interface{}) {
	l.line("OUTLIER", Orange, fmt.Sprintf(format, args...))
}

// Debug logs at DEBUG level (cyan) only when verbose; no-op otherwise. Caller should check Verbose before calling if needed.
func (l *Logger) Debug(verbose bool, format string, args ...interface{}) {
	if !verbose {
		return
	}
	l.line("DEBUG", Cyan, fmt.Sprintf(format, args...))
}
