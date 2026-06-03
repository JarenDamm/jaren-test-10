// Owner module: postgres-go-http (scaffolded — you own this file)
//
// This file is the scaffolded half of the postgres capability (ADR-0011).
// It ships once as a worked example and then is yours: edit it, replace it
// with your real queries, or delete it. Subsequent applies will not overwrite
// it, so anything you put here survives a required update to the managed
// wiring file cap_postgres.go.
//
// The example registers a GET /db/example route that runs a schema-less
// SELECT now() through the pool exposed by the managed wiring. It is
// intentionally trivial — no table, no migration — so the rendered Foundation
// boots against a fresh postgres without setup. Delete this file (and its
// init() registration) once you have real routes that take its place.
package app

import (
	"context"
	"fmt"
	"net/http"
	"time"
)

func init() {
	Register(registerPostgresExample)
}

func registerPostgresExample(a *App) {
	a.Mux.HandleFunc("GET /db/example", func(w http.ResponseWriter, r *http.Request) {
		if DB == nil {
			http.Error(w, "db not ready", http.StatusServiceUnavailable)
			return
		}
		ctx, cancel := context.WithTimeout(r.Context(), 2*time.Second)
		defer cancel()

		var now time.Time
		if err := DB.QueryRowContext(ctx, "SELECT now()").Scan(&now); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		_, _ = fmt.Fprintf(w, "db time: %s\n", now.Format(time.RFC3339Nano))
	})
}
