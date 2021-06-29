package v010

import (
	"net/http"

	"github.com/DataDog/datadog-agent/pkg/appsec/backend"
	"github.com/DataDog/datadog-agent/pkg/appsec/config"
)

// NewServeMux creates and returns the HTTP request multiplexer serving the
// AppSec HTTP API v0.1.0 with the following endpoint:
//   - `/` is a reverse proxy to the Intake API.
func NewServeMux(cfg *config.Config) *http.ServeMux {
	mux := http.NewServeMux()
	proxy := backend.NewReverseProxy(cfg)
	mux.Handle("/", proxy)
	return mux
}
