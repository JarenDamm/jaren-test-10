// Owner module: language-go-http
//
// These tests assert the Foundation's contract: the seam applies registered
// capability hooks, and the liveness/readiness endpoints respond. They ship
// managed alongside app.go so the contract travels with the file the platform
// owns.
package app

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"slices"
	"testing"

	"jt-10/internal/config"
)

// newTestApp builds an App with a throwaway config and a discard logger.
func newTestApp() *App {
	return New(
		&config.Config{Addr: ":0", LogLevel: "info"},
		slog.New(slog.NewTextHandler(io.Discard, nil)),
	)
}

func TestNew_MountsHealthAndReadyEndpoints(t *testing.T) {
	a := newTestApp()
	for _, path := range []string{"/healthz", "/readyz"} {
		req := httptest.NewRequest(http.MethodGet, path, nil)
		rec := httptest.NewRecorder()
		a.Mux.ServeHTTP(rec, req)
		if rec.Code != http.StatusOK {
			t.Errorf("GET %s = %d, want %d", path, rec.Code, http.StatusOK)
		}
	}
}

func TestNew_AppliesRegisteredCapabilities(t *testing.T) {
	Register(func(a *App) {
		a.Mux.HandleFunc("GET /seam-probe", func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(http.StatusTeapot)
		})
	})

	a := newTestApp()
	req := httptest.NewRequest(http.MethodGet, "/seam-probe", nil)
	rec := httptest.NewRecorder()
	a.Mux.ServeHTTP(rec, req)
	if rec.Code != http.StatusTeapot {
		t.Errorf("registered route /seam-probe = %d, want %d (registry not applied)", rec.Code, http.StatusTeapot)
	}
}

func TestApp_Lifecycle_StartInOrder_StopInReverse(t *testing.T) {
	a := newTestApp()
	var events []string
	a.OnStart = append(a.OnStart, func(context.Context) error { events = append(events, "start-A"); return nil })
	a.OnStop = append(a.OnStop, func(context.Context) error { events = append(events, "stop-A"); return nil })
	a.OnStart = append(a.OnStart, func(context.Context) error { events = append(events, "start-B"); return nil })
	a.OnStop = append(a.OnStop, func(context.Context) error { events = append(events, "stop-B"); return errors.New("close failed") })

	if err := a.startAll(context.Background()); err != nil {
		t.Fatalf("startAll: %v", err)
	}
	a.shutdown() // best-effort: stop-B errors, but stop-A must still run

	want := []string{"start-A", "start-B", "stop-B", "stop-A"}
	if !slices.Equal(events, want) {
		t.Errorf("lifecycle order = %v, want %v", events, want)
	}
}

func TestApp_StartAll_FailFast_RunsRegisteredStops(t *testing.T) {
	a := newTestApp()
	var events []string
	a.OnStart = append(a.OnStart, func(context.Context) error { events = append(events, "start-A"); return nil })
	a.OnStop = append(a.OnStop, func(context.Context) error { events = append(events, "stop-A"); return nil })
	a.OnStart = append(a.OnStart, func(context.Context) error { events = append(events, "start-B"); return errors.New("db unreachable") })
	a.OnStop = append(a.OnStop, func(context.Context) error { events = append(events, "stop-B"); return nil })
	// A later OnStart must never run once B fails (fail-fast).
	a.OnStart = append(a.OnStart, func(context.Context) error { events = append(events, "start-C"); return nil })

	if err := a.startAll(context.Background()); err == nil {
		t.Fatal("expected startAll to return an error when an OnStart fails")
	}

	// B failed → C never ran; startAll unwound by running the registered
	// OnStops in reverse.
	want := []string{"start-A", "start-B", "stop-B", "stop-A"}
	if !slices.Equal(events, want) {
		t.Errorf("events = %v, want %v", events, want)
	}
}
