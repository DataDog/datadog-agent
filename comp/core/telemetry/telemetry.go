// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2023-present Datadog, Inc.

package telemetry

import (
	"net/http"
	"sync"

	dto "github.com/prometheus/client_model/go"
	promOtel "go.opentelemetry.io/otel/exporters/prometheus"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/collectors"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"go.opentelemetry.io/otel/metric"
	sdk "go.opentelemetry.io/otel/sdk/metric"
)

// TODO (components): Remove the globals and move this into `newTelemetry` after all telemetry is migrated to the component
var (
	registry = newRegistry()
	provider = newProvider(registry)
	mutex    = sync.Mutex{}

	defaultRegistry = prometheus.NewRegistry()
)

type telemetryImpl struct {
	mutex         *sync.Mutex
	registry      *prometheus.Registry
	meterProvider *sdk.MeterProvider

	defaultRegistry *prometheus.Registry
}

func newRegistry() *prometheus.Registry {
	reg := prometheus.NewRegistry()
	reg.MustRegister(collectors.NewProcessCollector(collectors.ProcessCollectorOpts{}))
	reg.MustRegister(collectors.NewGoCollector(collectors.WithGoCollectorRuntimeMetrics(collectors.MetricsGC, collectors.MetricsMemory, collectors.MetricsScheduler)))
	return reg
}

func newProvider(reg *prometheus.Registry) *sdk.MeterProvider {
	exporter, err := promOtel.New(promOtel.WithRegisterer(reg))

	if err != nil {
		panic(err)
	}

	return sdk.NewMeterProvider(sdk.WithReader(exporter))
}

func newTelemetry() Component {
	return &telemetryImpl{
		mutex:         &mutex,
		registry:      registry,
		meterProvider: provider,

		defaultRegistry: defaultRegistry,
	}
}

// GetCompatComponent returns a component wrapping telemetry global variables
// TODO (components): Remove this when all telemetry is migrated to the component
func GetCompatComponent() Component {
	return newTelemetry()
}

func (t *telemetryImpl) Handler() http.Handler {
	return promhttp.HandlerFor(t.registry, promhttp.HandlerOpts{})
}

func (t *telemetryImpl) Reset() {
	mutex.Lock()
	defer mutex.Unlock()
	registry = prometheus.NewRegistry()
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

func (t *telemetryImpl) Meter(name string, opts ...metric.MeterOption) metric.Meter {
	return t.meterProvider.Meter(name, opts...)
}

func (t *telemetryImpl) NewCounter(subsystem, name string, tags []string, help string) Counter {
	return t.NewCounterWithOpts(subsystem, name, tags, help, DefaultOptions)
}

func (t *telemetryImpl) NewCounterWithOpts(subsystem, name string, tags []string, help string, opts Options) Counter {
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

func (t *telemetryImpl) NewSimpleCounter(subsystem, name, help string) SimpleCounter {
	return t.NewSimpleCounterWithOpts(subsystem, name, help, DefaultOptions)
}

func (t *telemetryImpl) NewSimpleCounterWithOpts(subsystem, name, help string, opts Options) SimpleCounter {
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

func (t *telemetryImpl) NewGauge(subsystem, name string, tags []string, help string) Gauge {
	return t.NewGaugeWithOpts(subsystem, name, tags, help, DefaultOptions)
}

func (t *telemetryImpl) NewGaugeWithOpts(subsystem, name string, tags []string, help string, opts Options) Gauge {
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

func (t *telemetryImpl) NewSimpleGauge(subsystem, name, help string) SimpleGauge {
	return t.NewSimpleGaugeWithOpts(subsystem, name, help, DefaultOptions)
}

func (t *telemetryImpl) NewSimpleGaugeWithOpts(subsystem, name, help string, opts Options) SimpleGauge {
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

func (t *telemetryImpl) NewHistogram(subsystem, name string, tags []string, help string, buckets []float64) Histogram {
	return t.NewHistogramWithOpts(subsystem, name, tags, help, buckets, DefaultOptions)
}

func (t *telemetryImpl) NewHistogramWithOpts(subsystem, name string, tags []string, help string, buckets []float64, opts Options) Histogram {
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

func (t *telemetryImpl) NewSimpleHistogram(subsystem, name, help string, buckets []float64) SimpleHistogram {
	return t.NewSimpleHistogramWithOpts(subsystem, name, help, buckets, DefaultOptions)
}

func (t *telemetryImpl) NewSimpleHistogramWithOpts(subsystem, name, help string, buckets []float64, opts Options) SimpleHistogram {
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

func (t *telemetryImpl) mustRegister(c prometheus.Collector, opts Options) {
	if opts.DefaultMetric {
		t.defaultRegistry.MustRegister(c)
	} else {
		t.registry.MustRegister(c)
	}
}

func (t *telemetryImpl) GatherDefault() ([]*dto.MetricFamily, error) {
	return t.defaultRegistry.Gather()
}
