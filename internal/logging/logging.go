// Package logging provides structured logging using slog.
package logging

import (
	"fmt"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"strings"

	"gopkg.in/natefinch/lumberjack.v2"
)

// FileConfig holds log file rotation settings.
type FileConfig struct {
	Path       string // log file path
	MaxSizeMB  int    // max size per file in MB
	MaxBackups int    // max number of old log files to keep
}

// Setup initializes the global logger.
// Logs are always written to stdout. When fc is non-nil and fc.Path is set,
// logs are additionally written to a rotating file via lumberjack.
func Setup(level string, json bool, fc *FileConfig) {
	var w io.Writer = os.Stdout

	if fc != nil && fc.Path != "" {
		// Ensure log directory exists
		if dir := filepath.Dir(fc.Path); dir != "" && dir != "." {
			if err := os.MkdirAll(dir, 0o755); err != nil {
				fmt.Fprintf(os.Stderr, "WARNING: failed to create log dir %s: %v\n", dir, err)
			}
		}

		maxSize := fc.MaxSizeMB
		if maxSize <= 0 {
			maxSize = 50
		}
		maxBackups := fc.MaxBackups
		if maxBackups <= 0 {
			maxBackups = 3
		}

		lj := &lumberjack.Logger{
			Filename:   fc.Path,
			MaxSize:    maxSize,
			MaxBackups: maxBackups,
			LocalTime:  true,
		}
		w = io.MultiWriter(os.Stdout, lj)
	}

	lvl := parseLevel(level)
	opts := &slog.HandlerOptions{Level: lvl}

	var handler slog.Handler
	if json {
		handler = slog.NewJSONHandler(w, opts)
	} else {
		handler = slog.NewTextHandler(w, opts)
	}

	slog.SetDefault(slog.New(handler))
}

// parseLevel converts string level to slog.Level.
func parseLevel(level string) slog.Level {
	switch strings.ToLower(level) {
	case "debug":
		return slog.LevelDebug
	case "warn", "warning":
		return slog.LevelWarn
	case "error":
		return slog.LevelError
	default:
		return slog.LevelInfo
	}
}

// Debug logs at debug level.
func Debug(msg string, args ...any) {
	slog.Debug(msg, args...)
}

// Info logs at info level.
func Info(msg string, args ...any) {
	slog.Info(msg, args...)
}

// Warn logs at warn level.
func Warn(msg string, args ...any) {
	slog.Warn(msg, args...)
}

// Error logs at error level.
func Error(msg string, args ...any) {
	slog.Error(msg, args...)
}
