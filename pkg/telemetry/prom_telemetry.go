package telemetry

import (
	"net/http"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

var (
	telemetryRegistry = prometheus.NewRegistry()
)

func init() {
	telemetryRegistry.MustRegister(prometheus.NewProcessCollector(prometheus.ProcessCollectorOpts{}))
	telemetryRegistry.MustRegister(prometheus.NewGoCollector())
}

// Handler serves the HTTP route containing the prometheus metrics.
func Handler() http.Handler {
	return promhttp.HandlerFor(telemetryRegistry, promhttp.HandlerOpts{})
}

// Reset can be needed for testing other packages
func Reset() {
	telemetryRegistry = prometheus.NewRegistry()
}
