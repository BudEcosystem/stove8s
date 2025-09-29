package daemonset

import (
	"net/http"

	"bud.studio/stove8s/internal/version"
)

func middlewareServerHeader(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Server", "Stove8s/"+version.Version)
		next.ServeHTTP(w, r)
	})
}
