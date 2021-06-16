package http

import (
	"net/http"

	"github.com/DataDog/datadog-agent/pkg/appsec/api/http/v0_1_0"
	"github.com/DataDog/datadog-agent/pkg/appsec/config"
)

type Server struct {
	server *http.Server
}

func NewServeMux(cfg *config.Config) *http.ServeMux {
	mux := http.NewServeMux()
	mux.Handle("/", newAPIVersionHandler(v0_1_0.NewServeMux(cfg)))
	return mux
}

func newAPIVersionHandler(v0_1_0 *http.ServeMux) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch v := r.Header.Get("X-Api-Version"); v {
		case "v0.1.0":
			v0_1_0.ServeHTTP(w, r)
		default:
			http.Error(w, "unexpected X-Api-Version value", http.StatusBadRequest)
		}
	})
}
