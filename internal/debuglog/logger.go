// Package debuglog provides opt-in file logging that cannot corrupt the TUI.
package debuglog

import (
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

var querySecret = regexp.MustCompile(`(?i)(api_key|x-emby-token)=([^&\s]+)`)

type Logger struct {
	file    *os.File
	logger  *slog.Logger
	secrets []string
}

func Open(path string, secrets ...string) (*Logger, error) {
	if path == "" {
		return nil, fmt.Errorf("open debug log: path is required")
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return nil, fmt.Errorf("open debug log: %w", err)
	}
	file, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o600)
	if err != nil {
		return nil, fmt.Errorf("open debug log: %w", err)
	}
	if err := file.Chmod(0o600); err != nil {
		_ = file.Close()
		return nil, fmt.Errorf("secure debug log: %w", err)
	}
	filtered := make([]string, 0, len(secrets))
	for _, secret := range secrets {
		if secret != "" {
			filtered = append(filtered, secret)
		}
	}
	return &Logger{file: file, logger: slog.New(slog.NewJSONHandler(file, nil)), secrets: filtered}, nil
}

func (l *Logger) Error(operation string, err error) {
	if l == nil || err == nil {
		return
	}
	message := querySecret.ReplaceAllString(err.Error(), "$1=[REDACTED]")
	for _, secret := range l.secrets {
		message = strings.ReplaceAll(message, secret, "[REDACTED]")
	}
	l.logger.Error("operation failed", "operation", operation, "error", message)
}

func (l *Logger) Close() error {
	if l == nil || l.file == nil {
		return nil
	}
	return l.file.Close()
}
