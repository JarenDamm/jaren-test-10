// Owner module: redis-go-http
//
// Managed wiring for the redis capability (ADR-0011). Lives in package app so
// its init() registers a Registration with the Foundation when the package is
// compiled — no import in the developer's cmd/server entrypoint needs editing.
// The seam it registers against (App, Register, App.OnStart, App.OnStop) is
// defined by the language module's app.go in this same package.
//
// What the platform owns here: the client's lifecycle (open + ping in OnStart,
// Close in OnStop), pool tuning, the /redis/health endpoint, and the driver
// import. Driver bumps and pool-best-practice changes propagate to existing
// repos as required-update PRs (ADR-0002). The companion scaffolded file
// cap_redis_example.go carries a sample SET/GET route the developer owns.
package app

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"strconv"
	"time"

	"github.com/redis/go-redis/v9"
)

// Cache is the process-wide redis client, opened by the registered OnStart
// hook and closed by OnStop. Exported so the scaffolded example file (and any
// developer-written code in package app) can use it. Nil until OnStart has
// run; callers must be invoked through the App seam rather than during package
// init.
var Cache *redis.Client

// redisPoolSize is the compiled-in default for the client's PoolSize knob,
// rendered from the manifest's pool_size input. REDIS_POOL_SIZE in the
// environment overrides it at startup.
const redisPoolSize = 25

func init() {
	Register(registerRedis)
}

// registerRedis wires the redis capability into the App. It appends an
// OnStart that opens + pings the client (fail-fast: a connect error aborts
// startup via the seam, no log.Fatal) and an OnStop that closes it. Routes
// that depend on the client — /redis/health here — are mounted now; they
// only execute after OnStart has run, so Cache is non-nil by the time a
// request arrives.
func registerRedis(a *App) {
	a.OnStart = append(a.OnStart, func(ctx context.Context) error {
		addr := os.Getenv("REDIS_ADDR")
		if addr == "" {
			addr = "localhost:6379"
		}

		client := redis.NewClient(&redis.Options{
			Addr:         addr,
			Password:     os.Getenv("REDIS_PASSWORD"),
			DB:           redisEnvInt("REDIS_DB", 0),
			PoolSize:     redisEnvInt("REDIS_POOL_SIZE", redisPoolSize),
			MinIdleConns: redisEnvInt("REDIS_MIN_IDLE_CONNS", redisPoolSize/2),
			DialTimeout:  redisEnvDuration("REDIS_DIAL_TIMEOUT", 5*time.Second),
			ReadTimeout:  redisEnvDuration("REDIS_READ_TIMEOUT", 2*time.Second),
			WriteTimeout: redisEnvDuration("REDIS_WRITE_TIMEOUT", 2*time.Second),
		})

		// Ping inside OnStart so a bad addr or unreachable instance surfaces as
		// a startup error rather than a runtime 500 on the first cache hit. The
		// bounded sub-context guards against a hung DNS lookup wedging boot.
		pingCtx, cancel := context.WithTimeout(ctx, redisEnvDuration("REDIS_PING_TIMEOUT", 5*time.Second))
		defer cancel()
		if err := client.Ping(pingCtx).Err(); err != nil {
			// Close the half-opened client before returning so we don't leak
			// the goroutines on a failed start.
			_ = client.Close()
			return fmt.Errorf("redis ping: %w", err)
		}

		Cache = client
		// Append OnStop only after the resource is acquired (ADR-0011): the
		// independent OnStart/OnStop slices mean a failed start would still
		// run a pre-registered OnStop, so registering Close here guarantees
		// we only try to close a client we actually opened.
		a.OnStop = append(a.OnStop, func(context.Context) error {
			Cache = nil
			return client.Close()
		})
		a.Log.Info("redis client ready", "module", "redis-go-http", "addr", addr)
		return nil
	})

	a.Mux.HandleFunc("GET /redis/health", func(w http.ResponseWriter, r *http.Request) {
		// Cache is nil if the route is hit before OnStart has run — treat
		// that as not-ready rather than panicking.
		if Cache == nil {
			http.Error(w, "redis not ready", http.StatusServiceUnavailable)
			return
		}
		ctx, cancel := context.WithTimeout(r.Context(), 2*time.Second)
		defer cancel()
		if err := Cache.Ping(ctx).Err(); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		_, _ = fmt.Fprintln(w, "ok")
	})
}

// redisEnvInt / redisEnvDuration are module-prefixed so cap_redis.go can
// coexist with cap_postgres.go in the same package — postgres-go-http
// declares envInt / envDuration with the same shape. A future "shared
// helpers owned by the language module" follow-up would deduplicate; for
// v1 the suffix keeps each capability's managed file self-contained and
// independently upgradable.
func redisEnvInt(key string, def int) int {
	if v := os.Getenv(key); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			return n
		}
	}
	return def
}

func redisEnvDuration(key string, def time.Duration) time.Duration {
	if v := os.Getenv(key); v != "" {
		if d, err := time.ParseDuration(v); err == nil {
			return d
		}
	}
	return def
}
