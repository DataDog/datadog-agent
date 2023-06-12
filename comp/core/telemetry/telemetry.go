// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2023-present Datadog, Inc.

package telemetry

import (
	"net/http"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/collectors"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

// TODO (components): Remove the global and move this into `newTelemetry` after all telemetry is migrated to the component
var (
	registry = newRegistry()
)

type telemetryImpl struct {
	registry *prometheus.Registry
}

func newRegistry() *prometheus.Registry {
	reg := prometheus.NewRegistry()
	reg.MustRegister(collectors.NewProcessCollector(collectors.ProcessCollectorOpts{}))
	reg.MustRegister(collectors.NewGoCollector())
	return reg
}

func newTelemetry() Component {
	return &telemetryImpl{
		registry: registry,
	}
}

// Same as `newTelemetry“ without the global.
// Can be merged with `newTelemetry` when the global is removed
func newMock() Component {
	return &telemetryImpl{
		registry: newRegistry(),
	}
}

// TODO (components): Remove this when all telemetry is migrated to the component
func GetCompatComponent() Component {
	return newTelemetry()
}

func (t *telemetryImpl) Handler() http.Handler {
	return promhttp.HandlerFor(t.registry, promhttp.HandlerOpts{})
}

func (t *telemetryImpl) Reset() {
	registry = prometheus.NewRegistry()
	t.registry = registry
}

func (t *telemetryImpl) NewCounter(subsystem, name string, tags []string, help string) Counter {
	return t.NewCounterWithOpts(subsystem, name, tags, help, DefaultOptions)
}

func (t *telemetryImpl) NewCounterWithOpts(subsystem, name string, tags []string, help string, opts Options) Counter {
	name = opts.NameWithSeparator(subsystem, name)

	c := &promCounter{
		pc: prometheus.NewCounterVec(
			prometheus.CounterOpts{
				Subsystem: subsystem,
				Name:      name,
				Help:      help,
			},
			tags,
		),
	}
	_ = t.registry.Register(c.pc)
	return c
}

func (t *telemetryImpl) NewSimpleCounter(subsystem, name, help string) SimpleCounter {
	return t.NewSimpleCounterWithOpts(subsystem, name, help, DefaultOptions)
}

func (t *telemetryImpl) NewSimpleCounterWithOpts(subsystem, name, help string, opts Options) SimpleCounter {
	name = opts.NameWithSeparator(subsystem, name)

	pc := prometheus.NewCounter(prometheus.CounterOpts{
		Subsystem: subsystem,
		Name:      name,
		Help:      help,
	})

	_ = t.registry.Register(pc)
	return &simplePromCounter{c: pc}
}

func (t *telemetryImpl) NewGauge(subsystem, name string, tags []string, help string) Gauge {
	return t.NewGaugeWithOpts(subsystem, name, tags, help, DefaultOptions)
}

func (t *telemetryImpl) NewGaugeWithOpts(subsystem, name string, tags []string, help string, opts Options) Gauge {
	name = opts.NameWithSeparator(subsystem, name)

	g := &promGauge{
		pg: prometheus.NewGaugeVec(
			prometheus.GaugeOpts{
				Subsystem: subsystem,
				Name:      name,
				Help:      help,
			},
			tags,
		),
	}
	_ = t.registry.Register(g.pg)
	return g
}

func (t *telemetryImpl) NewSimpleGauge(subsystem, name, help string) SimpleGauge {
	return t.NewSimpleGaugeWithOpts(subsystem, name, help, DefaultOptions)
}

func (t *telemetryImpl) NewSimpleGaugeWithOpts(subsystem, name, help string, opts Options) SimpleGauge {
	name = opts.NameWithSeparator(subsystem, name)

	pc := &simplePromGauge{g: prometheus.NewGauge(prometheus.GaugeOpts{
		Subsystem: subsystem,
		Name:      name,
		Help:      help,
	})}

	_ = t.registry.Register(pc.g)
	return pc
}

func (t *telemetryImpl) NewHistogram(subsystem, name string, tags []string, help string, buckets []float64) Histogram {
	return t.NewHistogramWithOpts(subsystem, name, tags, help, buckets, DefaultOptions)
}

func (t *telemetryImpl) NewHistogramWithOpts(subsystem, name string, tags []string, help string, buckets []float64, opts Options) Histogram {
	name = opts.NameWithSeparator(subsystem, name)

	h := &promHistogram{
		ph: prometheus.NewHistogramVec(
			prometheus.HistogramOpts{
				Subsystem: subsystem,
				Name:      name,
				Help:      help,
				Buckets:   buckets,
			},
			tags,
		),
	}

	_ = t.registry.Register(h.ph)
	return h
}

func (t *telemetryImpl) NewSimpleHistogram(subsystem, name, help string, buckets []float64) SimpleHistogram {
	return t.NewSimpleHistogramWithOpts(subsystem, name, help, buckets, DefaultOptions)
}

func (t *telemetryImpl) NewSimpleHistogramWithOpts(subsystem, name, help string, buckets []float64, opts Options) SimpleHistogram {
	name = opts.NameWithSeparator(subsystem, name)

	pc := &simplePromHistogram{h: prometheus.NewHistogram(prometheus.HistogramOpts{
		Subsystem: subsystem,
		Name:      name,
		Help:      help,
		Buckets:   buckets,
	})}

	_ = t.registry.Register(pc.h)
	return pc
}
