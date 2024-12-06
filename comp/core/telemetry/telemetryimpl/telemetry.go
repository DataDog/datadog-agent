// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2023-present Datadog, Inc.

// Package telemetryimpl implements the telemetry component interface.
package telemetryimpl

import (
	"context"
	"net/http"
	"sync"

	"go.uber.org/fx"

	"github.com/DataDog/datadog-agent/comp/core/telemetry"
	"github.com/DataDog/datadog-agent/pkg/util/fxutil"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/collectors"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

// Module defines the fx options for this component.
func Module() fxutil.Module {
	return fxutil.Component(
		fx.Provide(newTelemetryComponent))
}

// TODO (components): Remove the globals and move this into `newTelemetry` after all telemetry is migrated to the component
var (
	registry = newRegistry()
	mutex    = sync.Mutex{}

	defaultRegistry = prometheus.NewRegistry()
)

type telemetryImpl struct {
	mutex    *sync.Mutex
	registry *prometheus.Registry

	defaultRegistry *prometheus.Registry
}

type dependencies struct {
	fx.In

	Lyfecycle fx.Lifecycle
}

func newRegistry() *prometheus.Registry {
	reg := prometheus.NewRegistry()
	reg.MustRegister(collectors.NewProcessCollector(collectors.ProcessCollectorOpts{}))
	reg.MustRegister(collectors.NewGoCollector(collectors.WithGoCollectorRuntimeMetrics(collectors.MetricsGC, collectors.MetricsMemory, collectors.MetricsScheduler)))
	return reg
}

func newTelemetryComponent(deps dependencies) telemetry.Component {
	comp := newTelemetry()

	// Since we are in the middle of a migration to components, we need to ensure that the global variables are reset
	// when the component is stopped.
	deps.Lyfecycle.Append(fx.Hook{
		OnStop: func(_ context.Context) error {
			comp.Reset()

			return nil
		},
	})
	return comp
}

func newTelemetry() telemetry.Component {
	return &telemetryImpl{
		mutex:    &mutex,
		registry: registry,

		defaultRegistry: defaultRegistry,
	}
}

// GetCompatComponent returns a component wrapping telemetry global variables
// TODO (components): Remove this when all telemetry is migrated to the component
func GetCompatComponent() telemetry.Component {
	return newTelemetry()
}

func (t *telemetryImpl) Handler() http.Handler {
	return promhttp.HandlerFor(t.registry, promhttp.HandlerOpts{})
}

func (t *telemetryImpl) Reset() {
	mutex.Lock()
	defer mutex.Unlock()
	registry = newRegistry()
	t.registry = registry
}

// RegisterCollector Registers a Collector with the prometheus registry
func (t *telemetryImpl) RegisterCollector(c prometheus.Collector) {
	registry.MustRegister(c)
}

// UnregisterCollector unregisters a Collector with the prometheus registry
func (t *telemetryImpl) UnregisterCollector(c prometheus.Collector) bool {
	return registry.Unregister(c)
}

func (t *telemetryImpl) NewCounter(subsystem, name string, tags []string, help string) telemetry.Counter {
	return t.NewCounterWithOpts(subsystem, name, tags, help, telemetry.DefaultOptions)
}

func (t *telemetryImpl) NewCounterWithOpts(subsystem, name string, tags []string, help string, opts telemetry.Options) telemetry.Counter {
	t.mutex.Lock()
	defer t.mutex.Unlock()

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
	t.mustRegister(c.pc, opts)
	return c
}

func (t *telemetryImpl) NewSimpleCounter(subsystem, name, help string) telemetry.SimpleCounter {
	return t.NewSimpleCounterWithOpts(subsystem, name, help, telemetry.DefaultOptions)
}

func (t *telemetryImpl) NewSimpleCounterWithOpts(subsystem, name, help string, opts telemetry.Options) telemetry.SimpleCounter {
	t.mutex.Lock()
	defer t.mutex.Unlock()

	name = opts.NameWithSeparator(subsystem, name)

	pc := prometheus.NewCounter(prometheus.CounterOpts{
		Subsystem: subsystem,
		Name:      name,
		Help:      help,
	})

	t.mustRegister(pc, opts)
	return &simplePromCounter{c: pc}
}

func (t *telemetryImpl) NewGauge(subsystem, name string, tags []string, help string) telemetry.Gauge {
	return t.NewGaugeWithOpts(subsystem, name, tags, help, telemetry.DefaultOptions)
}

func (t *telemetryImpl) NewGaugeWithOpts(subsystem, name string, tags []string, help string, opts telemetry.Options) telemetry.Gauge {
	t.mutex.Lock()
	defer t.mutex.Unlock()

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
	t.mustRegister(g.pg, opts)
	return g
}

func (t *telemetryImpl) NewSimpleGauge(subsystem, name, help string) telemetry.SimpleGauge {
	return t.NewSimpleGaugeWithOpts(subsystem, name, help, telemetry.DefaultOptions)
}

func (t *telemetryImpl) NewSimpleGaugeWithOpts(subsystem, name, help string, opts telemetry.Options) telemetry.SimpleGauge {
	t.mutex.Lock()
	defer t.mutex.Unlock()

	name = opts.NameWithSeparator(subsystem, name)

	pc := &simplePromGauge{g: prometheus.NewGauge(prometheus.GaugeOpts{
		Subsystem: subsystem,
		Name:      name,
		Help:      help,
	})}

	t.mustRegister(pc.g, opts)
	return pc
}

func (t *telemetryImpl) NewHistogram(subsystem, name string, tags []string, help string, buckets []float64) telemetry.Histogram {
	return t.NewHistogramWithOpts(subsystem, name, tags, help, buckets, telemetry.DefaultOptions)
}

func (t *telemetryImpl) NewHistogramWithOpts(subsystem, name string, tags []string, help string, buckets []float64, opts telemetry.Options) telemetry.Histogram {
	t.mutex.Lock()
	defer t.mutex.Unlock()

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

	t.mustRegister(h.ph, opts)

	return h
}

func (t *telemetryImpl) NewSimpleHistogram(subsystem, name, help string, buckets []float64) telemetry.SimpleHistogram {
	return t.NewSimpleHistogramWithOpts(subsystem, name, help, buckets, telemetry.DefaultOptions)
}

func (t *telemetryImpl) NewSimpleHistogramWithOpts(subsystem, name, help string, buckets []float64, opts telemetry.Options) telemetry.SimpleHistogram {
	t.mutex.Lock()
	defer t.mutex.Unlock()

	name = opts.NameWithSeparator(subsystem, name)

	pc := &simplePromHistogram{h: prometheus.NewHistogram(prometheus.HistogramOpts{
		Subsystem: subsystem,
		Name:      name,
		Help:      help,
		Buckets:   buckets,
	})}

	t.mustRegister(pc.h, opts)
	return pc
}

func (t *telemetryImpl) mustRegister(c prometheus.Collector, opts telemetry.Options) {
	if opts.DefaultMetric {
		t.defaultRegistry.MustRegister(c)
	} else {
		t.registry.MustRegister(c)
	}
}

func (t *telemetryImpl) Gather(defaultGather bool) ([]*telemetry.MetricFamily, error) {
	if defaultGather {
		return t.defaultRegistry.Gather()
	}

	t.mutex.Lock()
	defer t.mutex.Unlock()

	return registry.Gather()
}
