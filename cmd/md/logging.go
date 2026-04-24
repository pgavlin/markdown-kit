package main

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"runtime"
)

// logDir returns the directory for md log files, following XDG conventions.
//
// Resolution order:
//  1. $XDG_STATE_HOME/md/
//  2. Platform default:
//     - macOS: ~/Library/Logs/md/
//     - Windows: %LOCALAPPDATA%\md\logs\
//     - Linux/other: ~/.local/state/md/
func logDir() (string, error) {
	if xdg := os.Getenv("XDG_STATE_HOME"); xdg != "" {
		return filepath.Join(xdg, "md"), nil
	}

	switch runtime.GOOS {
	case "darwin":
		home, err := os.UserHomeDir()
		if err != nil {
			return "", err
		}
		return filepath.Join(home, "Library", "Logs", "md"), nil
	case "windows":
		local := os.Getenv("LOCALAPPDATA")
		if local == "" {
			home, err := os.UserHomeDir()
			if err != nil {
				return "", err
			}
			local = filepath.Join(home, "AppData", "Local")
		}
		return filepath.Join(local, "md", "logs"), nil
	default:
		home, err := os.UserHomeDir()
		if err != nil {
			return "", err
		}
		return filepath.Join(home, ".local", "state", "md"), nil
	}
}

// openLogger creates a JSON logger that writes to md.log in the log directory.
// It returns the logger, the open file (for deferred close), and any error.
// On failure the returned logger is a no-op (discard) logger and the file is nil.
func openLogger() (*slog.Logger, *os.File, error) {
	dir, err := logDir()
	if err != nil {
		return slog.New(discardHandler{}), nil, err
	}

	if err := os.MkdirAll(dir, 0o755); err != nil {
		return slog.New(discardHandler{}), nil, err
	}

	f, err := os.Create(filepath.Join(dir, "md.log"))
	if err != nil {
		return slog.New(discardHandler{}), nil, err
	}

	logger := slog.New(slog.NewJSONHandler(f, nil))
	return logger, f, nil
}

// writePanicReport logs the panic with stack trace and writes a human-
// readable crash message to stderr. It is separated from handlePanic so
// tests can exercise the formatting and logger wiring without killing
// the test process.
func writePanicReport(logger *slog.Logger, logFile *os.File, stderr io.Writer, r any, stack []byte) {
	logger.Error("panic", "value", fmt.Sprint(r), "stack", string(stack))

	if logFile != nil {
		logFile.Sync()
	}

	fmt.Fprintln(stderr, "")
	fmt.Fprintln(stderr, "md crashed unexpectedly.")
	fmt.Fprintln(stderr, "")
	fmt.Fprintf(stderr, "  panic: %v\n", r)
	fmt.Fprintln(stderr, "")
	if logFile != nil {
		fmt.Fprintf(stderr, "Details have been written to: %s\n", logFile.Name())
		fmt.Fprintln(stderr, "")
	}
	fmt.Fprintln(stderr, "Please report this issue at: https://github.com/pgavlin/markdown-kit/issues")
}

// handlePanic recovers from a panic, logs it, prints a crash report to
// stderr, and exits the process. Intended for use with `defer`.
func handlePanic(logger *slog.Logger, logFile *os.File) {
	r := recover()
	if r == nil {
		return
	}

	// Capture the stack trace (skip the recover/handlePanic frames).
	buf := make([]byte, 64<<10)
	n := runtime.Stack(buf, false)

	writePanicReport(logger, logFile, os.Stderr, r, buf[:n])
	os.Exit(2)
}

// discardHandler is a slog.Handler that discards all records.
type discardHandler struct{}

func (discardHandler) Enabled(_ context.Context, _ slog.Level) bool  { return false }
func (discardHandler) Handle(_ context.Context, _ slog.Record) error { return nil }
func (discardHandler) WithAttrs(_ []slog.Attr) slog.Handler          { return discardHandler{} }
func (discardHandler) WithGroup(_ string) slog.Handler               { return discardHandler{} }
