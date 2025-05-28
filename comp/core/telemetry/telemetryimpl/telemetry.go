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

func (t *telemetryImpl) NewHistogram(subsystem, name string, tags []string, help string, buckets []float64) telemetry.Histogram { // JMW use this?
	return t.NewHistogramWithOpts(subsystem, name, tags, help, buckets, telemetry.DefaultOptions)
}

func (t *telemetryImpl) NewHistogramWithOpts(subsystem, name string, tags []string, help string, buckets []float64, opts telemetry.Options) telemetry.Histogram {
	t.mutex.Lock()
	defer t.mutex.Unlock()

	name = opts.NameWithSeparator(subsystem, name)
	// % drjmwqa-curlprometheus | rg api_server.*bucket
	// api_server__request_duration_seconds_bucket{auth="mTLS",method="GET",path="/agent/status/health",servername="CMD",status_code="200",le="0.01"} 1286
	// api_server__request_duration_seconds_bucket{auth="mTLS",method="GET",path="/agent/status/health",servername="CMD",status_code="200",le="0.025"} 1286
	// api_server__request_duration_seconds_bucket{auth="mTLS",method="GET",path="/agent/status/health",servername="CMD",status_code="200",le="0.05"} 1286
	// api_server__request_duration_seconds_bucket{auth="mTLS",method="GET",path="/agent/status/health",servername="CMD",status_code="200",le="0.1"} 1286
	// api_server__request_duration_seconds_bucket{auth="mTLS",method="GET",path="/agent/status/health",servername="CMD",status_code="200",le="0.25"} 1286
	// api_server__request_duration_seconds_bucket{auth="mTLS",method="GET",path="/agent/status/health",servername="CMD",status_code="200",le="0.5"} 1286
	// api_server__request_duration_seconds_bucket{auth="mTLS",method="GET",path="/agent/status/health",servername="CMD",status_code="200",le="1"} 1286
	// api_server__request_duration_seconds_bucket{auth="mTLS",method="GET",path="/agent/status/health",servername="CMD",status_code="200",le="2.5"} 1286
	// api_server__request_duration_seconds_bucket{auth="mTLS",method="GET",path="/agent/status/health",servername="CMD",status_code="200",le="5"} 1286
	// api_server__request_duration_seconds_bucket{auth="mTLS",method="GET",path="/agent/status/health",servername="CMD",status_code="200",le="10"} 1286
	// api_server__request_duration_seconds_bucket{auth="mTLS",method="GET",path="/agent/status/health",servername="CMD",status_code="200",le="+Inf"} 1286

	// JMWWED try for flowDuration var buckets = []float64{5, 10, 30, 60, 150, 300, 450, 600}
	// JMWWED try for numuses var buckets = []float64{1, 2, 3, 4, 5, 10, 20, 30}
	// JMWWED try for flowsAggregated var buckets = []float64{1, 2, 3, 4, 5, 10, 20, 30}
	//
	// % findgrep request_duration_seconds "*.go"
	// 262:        - name: api_server.request_duration_seconds
	// ./comp/core/agenttelemetry/impl/config.go
	// 23:	MetricName = "request_duration_seconds" // JMWJMW
	// ./comp/api/api/apiimpl/observability/telemetry.go
	// 125:		"rest_client_request_duration_seconds":        provider.restClientLatency,
	// ./pkg/collector/corechecks/containers/kubelet/provider/kubelet/provider.go
	h := &promHistogram{
		ph: prometheus.NewHistogramVec(
			prometheus.HistogramOpts{
				Subsystem: subsystem,
				Name:      name,
				Help:      help,
				Buckets:   buckets,
				// DefBuckets are the default Histogram buckets. The default buckets are //JMWJMW
				// tailored to broadly measure the response time (in seconds) of a network
				// service. Most likely, however, you will be required to define buckets
				// customized to your use case.
				// var DefBuckets = []float64{.005, .01, .025, .05, .1, .25, .5, 1, 2.5, 5, 10}
			},
			tags,
		),
	}

	t.mustRegister(h.ph, opts)

	return h
}

// JMWJMW use this instead, I don't care about having a vector of histograms based on tags, just one histogram for the aggregator
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
