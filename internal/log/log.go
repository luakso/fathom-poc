// Package log builds an *slog.Logger from BasicConfig.Log.
package log

import (
	"log/slog"
	"os"

	"github.com/lukostrobl/fathom/internal/config"
)

// New returns a slog.Logger configured per BasicConfig.Log and also installs it
// as the slog package default so plain `slog.Info(...)` calls inherit the
// chosen handler.
func New(b config.BasicConfig) *slog.Logger {
	level := parseLevel(b.Log.Level)
	opts := &slog.HandlerOptions{Level: level}

	var handler slog.Handler
	if b.Log.IsPretty {
		handler = slog.NewTextHandler(os.Stderr, opts)
	} else {
		handler = slog.NewJSONHandler(os.Stderr, opts)
	}

	logger := slog.New(handler).With("binary", b.Name, "version", b.Version, "env", b.Env)
	slog.SetDefault(logger)
	return logger
}

func parseLevel(s string) slog.Level {
	switch s {
	case "debug":
		return slog.LevelDebug
	case "warn":
		return slog.LevelWarn
	case "error":
		return slog.LevelError
	default:
		return slog.LevelInfo
	}
}
