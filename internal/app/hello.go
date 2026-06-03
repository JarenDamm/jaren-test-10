// Owner module: language-go-http
//
// hello.go is a worked example of the extension seam — and it's yours to edit
// or delete (scaffolded). It self-registers a route via Register, reads the
// shared logger off the App, and serves a greeting. Add your own routes the
// same way: a file in package app with an init() that calls Register.
package app

import (
	"fmt"
	"net/http"
)

func init() {
	Register(func(a *App) {
		a.Mux.HandleFunc("GET /hello", func(w http.ResponseWriter, r *http.Request) {
			name := r.URL.Query().Get("name")
			if name == "" {
				name = "world"
			}
			a.Log.Info("greeting", "name", name)
			_, _ = fmt.Fprintf(w, "hello, %s\n", name)
		})
	})
}
