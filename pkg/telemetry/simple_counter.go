package telemetry

import (
	"github.com/prometheus/client_golang/prometheus"
)

// SimpleCounter tracks how many times something is happening.
type SimpleCounter interface {
	// Inc increments the counter.
	Inc()
	// Add increments the counter by given amount.
	Add(float64)
}

// NewSimpleCounter creates a new SimpleCounter with default options.
func NewSimpleCounter(subsystem, name, help string) SimpleCounter {
	return NewSimpleCounterWithOpts(subsystem, name, help, DefaultOptions)
}

// NewSimpleCounterWithOpts creates a new SimpleCounter.
func NewSimpleCounterWithOpts(subsystem, name, help string, opts Options) SimpleCounter {
	name = opts.NameWithSeparator(subsystem, name)

	pc := prometheus.NewCounter(prometheus.CounterOpts{
		Subsystem: subsystem,
		Name:      name,
		Help:      help,
	})

	telemetryRegistry.MustRegister(pc)

	return pc
}
