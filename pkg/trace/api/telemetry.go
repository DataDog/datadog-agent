package api

import (
	"fmt"
	stdlog "log"
	"net/http"
	"time"

	"github.com/DataDog/datadog-agent/pkg/trace/logutil"
	"github.com/DataDog/datadog-agent/pkg/trace/telemetry"
)

// telemetryProxyHandler parses returns a new HTTP handler which will proxy requests to the configured intakes.
// If the main intake URL can not be computed because of config, the returned handler will always
// return http.StatusInternalServerError along with a clarification.
func (r *HTTPReceiver) telemetryProxyHandler() http.Handler {
	if !telemetry.IsEnabled() {
		return http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
			msg := fmt.Sprintf("Telemetry proxy forwarder is Disabled")
			http.Error(w, msg, http.StatusMethodNotAllowed)
		})
	}
	baseTarget, err := telemetry.BuildBaseTarget(r.conf.APIKey())

	if err != nil {
		return http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
			msg := fmt.Sprintf("Telemetry proxy forwarder encountered error: %v", err)
			http.Error(w, msg, http.StatusInternalServerError)
		})
	}

	transport := telemetry.MultiTransport{
		Transport:         r.conf.NewHTTPTransport(),
		BaseTarget:        *baseTarget,
		AdditionalTargets: telemetry.BuildAdditionalTargets(),
		Director: func(req *http.Request) {
			req.Header.Set("DD-Agent-Hostname", r.conf.Hostname)
			req.Header.Set("DD-Agent-Env", r.conf.DefaultEnv)
		},
	}

	limitedLogger := logutil.NewThrottled(5, 10*time.Second) // limit to 5 messages every 10 seconds
	logger := stdlog.New(limitedLogger, "telemetry.Proxy: ", 0)

	return telemetry.NewReverseProxy(&transport, logger)
}
