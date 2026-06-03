// Owner module: language-go-http
//
// Package config is the service's configuration. It is yours (scaffolded): add
// fields and sources here as the service grows. Values come from the
// environment with command-line flags taking precedence — no third-party
// configuration library.
package config

import (
	"flag"
	"os"
)

// Config holds the runtime configuration for the service.
type Config struct {
	// Addr is the HTTP listen address (e.g. ":8080").
	Addr string
	// LogLevel is the minimum slog level: debug, info, warn, or error.
	LogLevel string
}

// Load builds a Config from environment variables, then applies any
// command-line flag overrides. Defaults apply when neither is set.
//
// It parses into a private FlagSet rather than the global flag.CommandLine so
// it never interferes with test harness flags, and so calling it more than
// once is harmless.
func Load() *Config {
	cfg := &Config{Addr: ":8080", LogLevel: "info"}
	if v := os.Getenv("ADDR"); v != "" {
		cfg.Addr = v
	}
	if v := os.Getenv("LOG_LEVEL"); v != "" {
		cfg.LogLevel = v
	}

	fs := flag.NewFlagSet("server", flag.ContinueOnError)
	fs.StringVar(&cfg.Addr, "addr", cfg.Addr, "HTTP listen address")
	fs.StringVar(&cfg.LogLevel, "log-level", cfg.LogLevel, "log level: debug, info, warn, or error")
	// Unknown flags are ignored (ContinueOnError) so the binary stays usable
	// under wrappers that pass their own flags.
	_ = fs.Parse(os.Args[1:])

	return cfg
}
