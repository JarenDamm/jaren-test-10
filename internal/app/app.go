// Owner module: language-go-http
//
// Package app is the composition seam for this service's HTTP Foundation. The
// language module owns this file (it is managed: re-apply overwrites it, and
// the platform evolves its shape over time). Capability modules extend the
// service by dropping a self-registering file into this package whose init()
// calls Register — they never edit the developer's cmd/server entrypoint.
package app

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"jt-10/internal/config"
)

// App is the contract a capability registers against. It carries the shared
// HTTP router plus the configuration and logger every handler needs. The
// platform owns its shape (this file is managed) and grows it over time via
// required updates, so add-ons compile against a stable target.
type App struct {
	// Mux is the shared HTTP router. Capability registrations mount their
	// routes here alongside the Foundation's own.
	Mux *http.ServeMux
	// Log is the structured logger; handlers and capabilities log through it.
	Log *slog.Logger
	// Cfg is the loaded runtime configuration.
	Cfg *config.Config
	// OnStart hooks acquire resources after the App is built and before the
	// server accepts traffic; Run invokes them in registration order. A
	// capability appends to OnStart during its registration.
	OnStart []func(context.Context) error
	// OnStop hooks release resources during graceful shutdown; Run invokes
	// them in reverse (LIFO) order, best-effort. The OnStart/OnStop slices are
	// independent (not index-paired), so on a failed start every registered
	// OnStop runs — an OnStop must tolerate a partial or skipped start (guard
	// its state, or append the OnStop from inside its OnStart only once the
	// resource is acquired).
	OnStop []func(context.Context) error
}

// Registration is a capability's hook into the Foundation: it mutates the
// composed App, typically by mounting routes on App.Mux.
type Registration func(*App)

// registrations is populated by init() hooks in capability-shipped files in
// this package. New applies each entry to the composed App at startup.
var registrations []Registration

// Register adds a capability hook to the registry. Capability modules call it
// from an init() in their own file in this package, so adding a capability
// never requires editing the developer's entrypoint.
func Register(r Registration) {
	registrations = append(registrations, r)
}

// New builds the composed App from the configuration and logger: it creates
// the router, mounts the Foundation's own liveness/readiness endpoints, then
// applies every registered capability hook in registration order.
func New(cfg *config.Config, logger *slog.Logger) *App {
	a := &App{Mux: http.NewServeMux(), Log: logger, Cfg: cfg}

	a.Mux.HandleFunc("GET /healthz", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ok"))
	})
	a.Mux.HandleFunc("GET /readyz", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ready"))
	})

	for _, register := range registrations {
		register(a)
	}
	return a
}

// Run builds the App and serves it on cfg.Addr until the process receives
// SIGINT or SIGTERM (or the passed context is cancelled), then shuts the
// server down gracefully within a bounded timeout.
func Run(ctx context.Context, cfg *config.Config, logger *slog.Logger) error {
	ctx, stop := signal.NotifyContext(ctx, os.Interrupt, syscall.SIGTERM)
	defer stop()

	a := New(cfg, logger)

	// Acquire resources before serving. On the first failure, startAll has
	// already released what it wired (best-effort, reverse), so just abort.
	if err := a.startAll(ctx); err != nil {
		return err
	}
	// Release resources on the way out, after the server has stopped.
	defer a.shutdown()

	srv := &http.Server{
		Addr:    cfg.Addr,
		Handler: a.Mux,
	}

	serveErr := make(chan error, 1)
	go func() {
		logger.Info("server listening", "addr", cfg.Addr)
		if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			serveErr <- err
			return
		}
		serveErr <- nil
	}()

	select {
	case err := <-serveErr:
		return err
	case <-ctx.Done():
		logger.Info("shutdown signal received")
	}

	shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	return srv.Shutdown(shutdownCtx)
}

// startAll runs every OnStart hook in registration order. Startup is
// fail-fast: on the first error it releases everything wired so far via
// shutdown (reverse, best-effort) and returns the error, so the caller aborts
// and the process exits non-zero. A capability that needs no startup work
// simply registers no OnStart.
func (a *App) startAll(ctx context.Context) error {
	for _, start := range a.OnStart {
		if err := start(ctx); err != nil {
			a.shutdown()
			return fmt.Errorf("startup: %w", err)
		}
	}
	return nil
}

// shutdown runs every registered OnStop hook in reverse (LIFO) order,
// best-effort: a failing hook is logged and the remaining hooks still run, so
// one capability cannot strand another's cleanup. It uses a fresh bounded
// context because the run-loop's context is already cancelled by shutdown time.
func (a *App) shutdown() {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	for i := len(a.OnStop) - 1; i >= 0; i-- {
		if err := a.OnStop[i](ctx); err != nil {
			a.Log.Error("shutdown hook failed", "index", i, "err", err)
		}
	}
}
