package http

import (
	"net/http"

	"github.com/DataDog/datadog-agent/pkg/appsec/api/http/v0_1_0"
	"github.com/DataDog/datadog-agent/pkg/appsec/config"
)

// NewServeMux creates and returns the HTTP request multiplexer serving the
// AppSec HTTP API. The API is versioned according to the X-Api-Version header
// value so that requests are routed to the request multiplexer of the
// given version.
func NewServeMux(cfg *config.Config) *http.ServeMux {
	mux := http.NewServeMux()
	mux.Handle("/", newAPIVersionHandler(v010.NewServeMux(cfg)))
	return mux
}

// newAPIVersionHandler creates the API version handler routing requests to
// the API handler according to their X-Api-Version header value.
func newAPIVersionHandler(v0_1_0 *http.ServeMux) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var api http.HandlerFunc
		switch v := r.Header.Get("X-Api-Version"); v {
		case "v0.1.0":
			api = v0_1_0.ServeHTTP
		default:
			http.Error(w, "Unexpected X-Api-Version value", http.StatusBadRequest)
			return
		}
		api.ServeHTTP(w, r)
	})
}
