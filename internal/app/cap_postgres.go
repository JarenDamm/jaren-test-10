// Owner module: postgres-go-http
//
// This file is the managed wiring for the postgres capability (ADR-0011). It
// lives in package app so its init() registers a Registration with the
// Foundation when the package is compiled — no import in the developer's
// cmd/server entrypoint needs editing. The seam it registers against (App,
// Register, App.OnStart, App.OnStop) is defined by the language module's
// app.go in this same package.
//
// What the platform owns here: the connection pool's lifecycle (open + ping in
// OnStart, Close in OnStop), pool tuning, the /db/health endpoint, and the
// driver import. Driver bumps and pool-best-practice changes propagate to
// existing repos as required-update PRs (ADR-0002). The companion scaffolded
// file cap_postgres_example.go carries a sample query route the developer
// owns.
package app

import (
	"context"
	"database/sql"
	"fmt"
	"net/http"
	"os"
	"strconv"
	"time"

	// pgx via the database/sql stdlib adapter (ADR-0011): keeps the *sql.DB
	// pool API the Foundation expects while moving off lib/pq (maintenance
	// mode).
	_ "github.com/jackc/pgx/v5/stdlib"
)

// DB is the process-wide connection pool, opened by the registered OnStart
// hook and closed by OnStop. It is exported so the scaffolded example file
// (and any developer-written code in package app) can query through it. It is
// nil until OnStart runs, so callers must be invoked through the App seam
// rather than during package init.
var DB *sql.DB

// pgPoolSize is the compiled-in default for the pool's max-open-connections
// knob, rendered from the manifest's pool_size input. POSTGRES_POOL_SIZE in
// the environment overrides it at startup.
const pgPoolSize = 25

func init() {
	Register(registerPostgres)
}

// registerPostgres wires the postgres capability into the App. It appends an
// OnStart that opens + pings the pool (fail-fast: a connect error aborts
// startup via the seam, no log.Fatal) and an OnStop that closes it. Routes
// that depend on the pool — /db/health here — are mounted now; they only
// execute after OnStart has run, so DB is non-nil by the time a request
// arrives.
func registerPostgres(a *App) {
	a.OnStart = append(a.OnStart, func(ctx context.Context) error {
		dsn := os.Getenv("POSTGRES_DSN")
		if dsn == "" {
			dsn = "host=localhost user=postgres password=insecure dbname=app sslmode=disable"
		}

		pool, err := sql.Open("pgx", dsn)
		if err != nil {
			return fmt.Errorf("postgres open: %w", err)
		}

		pool.SetMaxOpenConns(envInt("POSTGRES_POOL_SIZE", pgPoolSize))
		pool.SetMaxIdleConns(envInt("POSTGRES_IDLE_CONNS", pgPoolSize/2))
		pool.SetConnMaxLifetime(envDuration("POSTGRES_CONN_MAX_LIFETIME", time.Hour))
		pool.SetConnMaxIdleTime(envDuration("POSTGRES_CONN_MAX_IDLE_TIME", 30*time.Minute))

		// Ping inside OnStart so a bad DSN or an unreachable database surfaces
		// as a startup error rather than a runtime 500 on the first request.
		// We use a bounded sub-context so a hung DNS lookup can't wedge boot.
		pingCtx, cancel := context.WithTimeout(ctx, envDuration("POSTGRES_PING_TIMEOUT", 5*time.Second))
		defer cancel()
		if err := pool.PingContext(pingCtx); err != nil {
			// Close the half-opened pool before returning so we don't leak the
			// driver goroutines on a failed start.
			_ = pool.Close()
			return fmt.Errorf("postgres ping: %w", err)
		}

		DB = pool
		// Append OnStop only after the resource is acquired (ADR-0011): the
		// independent OnStart/OnStop slices mean a failed start would still
		// run a pre-registered OnStop, so registering Close here guarantees we
		// only try to close a pool we actually opened.
		a.OnStop = append(a.OnStop, func(context.Context) error {
			DB = nil
			return pool.Close()
		})
		a.Log.Info("postgres pool ready", "module", "postgres-go-http")
		return nil
	})

	a.Mux.HandleFunc("GET /db/health", func(w http.ResponseWriter, r *http.Request) {
		// DB is nil if the route is hit before OnStart has run — treat that as
		// not-ready rather than panicking.
		if DB == nil {
			http.Error(w, "db not ready", http.StatusServiceUnavailable)
			return
		}
		ctx, cancel := context.WithTimeout(r.Context(), 2*time.Second)
		defer cancel()
		if err := DB.PingContext(ctx); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		_, _ = fmt.Fprintln(w, "ok")
	})
}

// envInt reads an int from the environment, falling back to def when the var
// is unset or unparseable. Capability config is read at OnStart (ADR-0011) so
// the Foundation's managed config struct stays untouched.
func envInt(key string, def int) int {
	if v := os.Getenv(key); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			return n
		}
	}
	return def
}

// envDuration reads a time.Duration from the environment, falling back to def
// when the var is unset or unparseable.
func envDuration(key string, def time.Duration) time.Duration {
	if v := os.Getenv(key); v != "" {
		if d, err := time.ParseDuration(v); err == nil {
			return d
		}
	}
	return def
}
