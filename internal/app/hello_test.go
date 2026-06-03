// Owner module: language-go-http
//
// A table-driven test for the sample route — the pattern to copy when you add
// your own. It builds the App through the seam (so the route registered by
// hello.go's init() is mounted) and exercises the handler with httptest.
package app

import (
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"

	"jt-10/internal/config"
)

func TestHelloRoute(t *testing.T) {
	cases := []struct {
		name     string
		query    string
		wantBody string
	}{
		{name: "default", query: "", wantBody: "hello, world\n"},
		{name: "named", query: "?name=primus", wantBody: "hello, primus\n"},
	}

	a := New(
		&config.Config{Addr: ":0", LogLevel: "info"},
		slog.New(slog.NewTextHandler(io.Discard, nil)),
	)

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, "/hello"+tc.query, nil)
			rec := httptest.NewRecorder()
			a.Mux.ServeHTTP(rec, req)

			if rec.Code != http.StatusOK {
				t.Fatalf("status = %d, want %d", rec.Code, http.StatusOK)
			}
			if got := rec.Body.String(); got != tc.wantBody {
				t.Errorf("body = %q, want %q", got, tc.wantBody)
			}
		})
	}
}
