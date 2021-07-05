package appsec

import (
	"fmt"
	stdlog "log"
	"net/http"
	"net/http/httputil"
	"net/url"
	"strconv"
	"time"

	coreconfig "github.com/DataDog/datadog-agent/pkg/config"
	"github.com/DataDog/datadog-agent/pkg/trace/info"
	"github.com/DataDog/datadog-agent/pkg/trace/logutil"
	"github.com/DataDog/datadog-agent/pkg/trace/metrics"
	"github.com/pkg/errors"
)

// ErrAgentDisabled is the error message logged when the AppSec agent is
// disabled by configuration.
var ErrAgentDisabled = errors.New("AppSec agent disabled. Set the " +
	"environment variable `DD_APPSEC_ENABLED=true` or add the entry " +
	"`appsec_config.enabled: true` to your datadog.yaml file")

// NewIntakeReverseProxy returns the AppSec Intake Proxy handler according to
// the agent configuration.
func NewIntakeReverseProxy(transport *http.Transport) (http.Handler, error) {
	disabled := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "appsec api disabled", http.StatusNotImplemented)
	})
	cfg, err := newConfig(coreconfig.Datadog)
	if err != nil {
		return disabled, errors.Wrap(err, "configuration: ")
	}
	if !cfg.Enabled {
		return disabled, ErrAgentDisabled
	}

	return newIntakeReverseProxy(cfg.IntakeURL, cfg.APIKey, transport), nil
}

// newIntakeReverseProxy creates a reverse proxy to the intake backend using the given
// transport round-tripper.
// The reverse proxy handler also adds extra headers such as Dd-Api-Key and Via.
func newIntakeReverseProxy(target *url.URL, apiKey string, transport http.RoundTripper) http.Handler {
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
	}
	proxy.Transport = transport
	proxy.ErrorLog = stdlog.New(logutil.NewThrottled(5, 10*time.Second), "appsec backend proxy: ", 0)
	return withMetrics(proxy)
}

func withMetrics(handler http.Handler) http.Handler {
	const (
		AppSecRequestMetricsPrefix = "datadog.trace_agent.appsec.api.request."
		CountID                    = AppSecRequestMetricsPrefix + "count"
		TimeID                     = AppSecRequestMetricsPrefix + "time"
		ContentLengthID            = AppSecRequestMetricsPrefix + "content_length"
	)
	return http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		tags := []string{"path:" + req.URL.Path}
		if ct := req.Header.Get("Content-Type"); ct != "" {
			tags = append(tags, "content_type:"+ct)
		}

		metrics.Gauge(CountID, 1, tags, 1)

		if cl := req.Header.Get("Content-Length"); cl != "" {
			if cl, err := strconv.Atoi(cl); err == nil {
				metrics.Histogram(ContentLengthID, float64(cl), tags, 1)
			}
		}

		now := time.Now()
		defer func() {
			metrics.Timing(TimeID, time.Since(now), tags, 1)
		}()
		handler.ServeHTTP(w, req)
	})
}
