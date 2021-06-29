package backend

import (
	"fmt"
	stdlog "log"
	"net/http"
	"net/http/httputil"
	"time"

	"github.com/DataDog/datadog-agent/pkg/appsec/config"
	"github.com/DataDog/datadog-agent/pkg/trace/info"
	"github.com/DataDog/datadog-agent/pkg/trace/logutil"
)

// NewReverseProxy creates a reverse proxy to the intake backend.
// The reverse proxy handler also adds extra headers such as Dd-Api-Key, and Via.
func NewReverseProxy(cfg *config.Config) *httputil.ReverseProxy {
	target := cfg.IntakeURL
	apiKey := cfg.APIKey
	proxy := httputil.NewSingleHostReverseProxy(target)
	via := fmt.Sprintf("appsec-agent %s", info.Version)
	// Wrap and overwrite the returned director to add extra headers
	director := proxy.Director
	proxy.Director = func(req *http.Request) {
		// Call the original director changing the request target
		director(req)
		// Set extra headers
		req.Header.Set("Via", via)
		req.Header.Set("Dd-Api-Key", apiKey)
		// TODO: add the container tag?
		// Remove extra headers
		req.Header.Del("X-Api-Version")
	}
	proxy.ErrorLog = stdlog.New(logutil.NewThrottled(5, 10*time.Second), "appsec backend proxy: ", 0)
	return proxy
}
