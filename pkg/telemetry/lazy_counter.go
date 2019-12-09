package telemetry

import (
	"sync"

	"github.com/DataDog/datadog-agent/pkg/config"
	"github.com/prometheus/client_golang/prometheus"
)

// TODO(remy): comment me.
type lazyCounter struct {
	subsystem string
	name      string
	tags      []string
	help      string

	counter Counter
	m       sync.Mutex
}

func (c *lazyCounter) init() {
	c.m.Lock()
	defer c.m.Unlock()
	// use Prometheus counter if the settings is enabled
	// otherwise fallback on a noop counter.
	if config.Datadog.GetBool("telemetry.enabled") {
		prom := &promCounter{
			pc: prometheus.NewCounterVec(
				prometheus.CounterOpts{
					Namespace: namespace,
					Subsystem: c.subsystem,
					Name:      c.name,
					Help:      c.help,
				},
				c.tags,
			),
		}
		c.counter = prom
		prometheus.MustRegister(prom.pc)
	} else {
		c.counter = &noopCounter{}
	}
}

// Inc increments the Counter value.
func (c *lazyCounter) Inc(tags ...string) {
	if c.counter == nil {
		c.init()
	}
	c.counter.Inc(tags...)
}

// Delete deletes the value for the Counter with the given tags.
func (c *lazyCounter) Delete(tags ...string) {
	if c.counter == nil {
		c.init()
	}
	c.counter.Delete(tags...)
}

// Add adds the value to the Counter value.
func (c *lazyCounter) Add(value float64, tags ...string) {
	if c.counter == nil {
		c.init()
	}
	c.counter.Add(value, tags...)
}
