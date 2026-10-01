// Package logging configures structured logging shared by every abm
// command: stderr for interactive use, plus a rotating file so `abm logs`
// has something to show even when journald/Event Log aren't consulted.
package logging

import (
	"fmt"
	"io"
	"log/slog"
	"os"
	"path/filepath"

	"github.com/Humran13/Auto-Backup-Manager/internal/secrets"
)

// redactingWriter runs every log line through secrets.Redact before it
// reaches disk, so a bug elsewhere that logs a raw error string still can't
// leak a password/token/key into a file that isn't root/admin-only forever.
type redactingWriter struct {
	w io.Writer
}

func (r redactingWriter) Write(p []byte) (int, error) {
	redacted := secrets.Redact(string(p))
	if _, err := r.w.Write([]byte(redacted)); err != nil {
		return 0, err
	}
	return len(p), nil
}

// New builds a slog.Logger that writes to both stderr and logDir/abm.log.
// level is one of "debug", "info", "warn", "error".
func New(logDir string, level string) (*slog.Logger, error) {
	if err := os.MkdirAll(logDir, 0o750); err != nil {
		return nil, fmt.Errorf("creating log directory: %w", err)
	}
	logPath := filepath.Join(logDir, "abm.log")
	f, err := os.OpenFile(logPath, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o640)
	if err != nil {
		return nil, fmt.Errorf("opening log file %s: %w", logPath, err)
	}

	var lvl slog.Level
	if err := lvl.UnmarshalText([]byte(level)); err != nil {
		lvl = slog.LevelInfo
	}

	handler := slog.NewJSONHandler(redactingWriter{io.MultiWriter(os.Stderr, f)}, &slog.HandlerOptions{Level: lvl})
	return slog.New(handler), nil
}
