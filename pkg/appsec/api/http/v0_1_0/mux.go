package v010

import (
	"net/http"
	"net/http/httputil"

	"github.com/DataDog/datadog-agent/pkg/appsec/backend"
	"github.com/DataDog/datadog-agent/pkg/appsec/config"
)

// NewServeMux creates and returns the HTTP request multiplexer serving the
// AppSec HTTP API. It can be attached to other
func NewServeMux(cfg *config.Config) *http.ServeMux {
	mux := http.NewServeMux()

	s := serveMux{
		proxy: backend.NewReverseProxy(cfg.IntakeURL, cfg.APIKey),
	}
	mux.HandleFunc("/", s.HandleEvents)
	return mux
}

type serveMux struct {
	proxy *httputil.ReverseProxy
}

func (s *serveMux) HandleEvents(w http.ResponseWriter, r *http.Request) {
	switch ct := r.Header.Get("Content-Type"); ct {
	case "application/json":
		s.proxy.ServeHTTP(w, r)
	default:
		http.Error(w, "unexpected Content-Type value", http.StatusBadRequest)
	}
}
