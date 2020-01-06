package telemetry

import (
	"net/http"
	"os"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

var (
	telemetryRegistry                         = prometheus.NewRegistry()
	telemetryRegisterer prometheus.Registerer = telemetryRegistry
	telemetryGatherer   prometheus.Gatherer   = telemetryRegistry
)

func init() {
	telemetryRegistry.MustRegister(prometheus.NewProcessCollector(os.Getpid(), ""))
	telemetryRegistry.MustRegister(prometheus.NewGoCollector())
}

// Handler serves the HTTP route containing the prometheus metrics.
func Handler() http.Handler {
	return promhttp.HandlerFor(telemetryRegistry, promhttp.HandlerOpts{})
}
