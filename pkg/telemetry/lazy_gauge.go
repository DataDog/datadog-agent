package telemetry

import (
	"sync"

	"github.com/DataDog/datadog-agent/pkg/config"
	"github.com/prometheus/client_golang/prometheus"
)

// TODO(remy): comment me.
type lazyGauge struct {
	subsystem string
	name      string
	tags      []string
	help      string

	gauge Gauge
	m     sync.Mutex
}

func (g *lazyGauge) init() {
	g.m.Lock()
	defer g.m.Unlock()
	// use Prometheus gauge if the settings is enabled
	// otherwise fallback on a noop gauge.
	if config.Datadog.GetBool("telemetry.enabled") {
		prom := &promGauge{
			pg: prometheus.NewGaugeVec(
				prometheus.GaugeOpts{
					Namespace: namespace,
					Subsystem: g.subsystem,
					Name:      g.name,
					Help:      g.help,
				},
				g.tags,
			),
		}
		g.gauge = prom
		prometheus.MustRegister(prom.pg)
	} else {
		g.gauge = &noopGauge{}
	}
}

// Set stores the value for the given tags.
func (g *lazyGauge) Set(value float64, tags ...string) {
	if g.gauge == nil {
		g.init()
	}
	g.gauge.Set(value, tags...)
}

// Inc increments the Gauge value.
func (g *lazyGauge) Inc(tags ...string) {
	if g.gauge == nil {
		g.init()
	}
	g.gauge.Inc(tags...)
}

// Dec decrements the Gauge value.
func (g *lazyGauge) Dec(tags ...string) {
	if g.gauge == nil {
		g.init()
	}
	g.gauge.Dec(tags...)
}

// Delete deletes the value for the Gauge with the given tags.
func (g *lazyGauge) Delete(tags ...string) {
	if g.gauge == nil {
		g.init()
	}
	g.gauge.Delete(tags...)
}

// Add adds the value to the Gauge value.
func (g *lazyGauge) Add(value float64, tags ...string) {
	if g.gauge == nil {
		g.init()
	}
	g.gauge.Add(value, tags...)
}

// Sub subtracts the value to the Gauge value.
func (g *lazyGauge) Sub(value float64, tags ...string) {
	if g.gauge == nil {
		g.init()
	}
	g.gauge.Sub(value, tags...)
}
