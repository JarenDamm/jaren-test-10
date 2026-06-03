// Owner module: language-go-http
//
// Package logging builds the service's structured logger. It is yours
// (scaffolded): swap the handler or add attributes as you like. The default is
// a JSON slog handler writing to stdout at the configured level.
package logging

import (
	"log/slog"
	"os"
	"strings"

	"jt-10/internal/config"
)

// New returns a JSON slog.Logger writing to stdout, with the minimum level
// taken from cfg.LogLevel (unrecognized values fall back to info).
func New(cfg *config.Config) *slog.Logger {
	level := slog.LevelInfo
	switch strings.ToLower(cfg.LogLevel) {
	case "debug":
		level = slog.LevelDebug
	case "warn":
		level = slog.LevelWarn
	case "error":
		level = slog.LevelError
	}
	return slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: level}))
}
