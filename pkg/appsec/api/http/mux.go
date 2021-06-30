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
	mux.Handle("/v0.1/", http.StripPrefix("/v0.1", v010.NewServeMux(cfg)))
	return mux
}
