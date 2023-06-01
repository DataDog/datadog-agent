package telemetry

import (
	"github.com/prometheus/client_golang/prometheus"

)

type ebpfConntrackerCollector struct {
}

// Describe returns all descriptions of the collector.
func (c *ebpfConntrackerCollector) Describe(ch chan<- *prometheus.Desc) {

}

// Collect returns the current state of all metrics of the collector.
func (c *ebpfConntrackerCollector) Collect(ch chan<- prometheus.Metric) {
	ebpfTelemetryMap := GetTelemetryMap()
}
