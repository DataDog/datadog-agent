// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2023-present Datadog, Inc.

//go:build !serverless

// Package telemetryimpl implements the telemetry component interface.
package telemetryimpl

import (
	"bytes"
	"context"
	"net/http"
	"sync"

	flaretypes "github.com/DataDog/datadog-agent/comp/core/flare/types"
	telemetry "github.com/DataDog/datadog-agent/comp/core/telemetry/def"
	compdef "github.com/DataDog/datadog-agent/comp/def"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/collectors"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"github.com/prometheus/common/expfmt"
)

// TODO (components): Remove the globals and move this into `newTelemetry` after all telemetry is migrated to the component
var (
	registry        = newRegistry()
	mutex           = sync.Mutex{}
	metricHelpMutex = sync.RWMutex{}
	metricHelp      = make(map[string]string)
)

type telemetryImpl struct {
	mutex           *sync.Mutex
	registry        *prometheus.Registry
	metricHelpMutex *sync.RWMutex
	metricHelp      map[string]string
}

func newRegistry() *prometheus.Registry {
	reg := prometheus.NewRegistry()
	reg.MustRegister(collectors.NewProcessCollector(collectors.ProcessCollectorOpts{}))
	reg.MustRegister(collectors.NewGoCollector(collectors.WithGoCollectorRuntimeMetrics(collectors.MetricsAll), collectors.WithoutGoCollectorRuntimeMetrics(collectors.MetricsDebug.Matcher)))
	return reg
}

// Requires defines the dependencies for the telemetry component
type Requires struct {
	compdef.In

	Lc compdef.Lifecycle
}

// Provides defines the output of the telemetry component
type Provides struct {
	compdef.Out

	Comp          telemetry.Component
	FlareProvider flaretypes.Provider
}

// NewComponent creates a new telemetry component.
func NewComponent(deps Requires) Provides {
	comp := newTelemetry()
	// Since we are in the middle of a migration to components, we need to ensure that the global variables are reset
	// when the component is stopped.
	deps.Lc.Append(compdef.Hook{
		OnStop: func(_ context.Context) error {
			comp.Reset()
			return nil
		},
	})
	return Provides{
		Comp:          comp,
		FlareProvider: flaretypes.NewProvider(comp.fillFlare),
	}
}

func newTelemetry() *telemetryImpl {
	mutex.Lock()
	defer mutex.Unlock()

	return &telemetryImpl{
		mutex:           &mutex,
		registry:        registry,
		metricHelpMutex: &metricHelpMutex,
		metricHelp:      metricHelp,
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
	t.metricHelpMutex.Lock()
	defer t.metricHelpMutex.Unlock()
	metricHelp = make(map[string]string)
	t.metricHelp = metricHelp
}

// CanonicalMetricHelp returns the HELP text for a metric family created in the normal telemetry registry.
func (t *telemetryImpl) CanonicalMetricHelp(metricName string) (help string, found bool) {
	t.metricHelpMutex.RLock()
	defer t.metricHelpMutex.RUnlock()
	help, found = t.metricHelp[metricName]
	return help, found
}

// RegisterCollector Registers a Collector with the prometheus registry
func (t *telemetryImpl) RegisterCollector(c prometheus.Collector) {
	t.registry.MustRegister(c)
}

// UnregisterCollector unregisters a Collector with the prometheus registry
func (t *telemetryImpl) UnregisterCollector(c prometheus.Collector) bool {
	return t.registry.Unregister(c)
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
	t.mustRegister(c.pc, subsystem, name, help)
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

	t.mustRegister(pc, subsystem, name, help)
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
	t.mustRegister(g.pg, subsystem, name, help)
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

	t.mustRegister(pc.g, subsystem, name, help)
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

	t.mustRegister(h.ph, subsystem, name, help)

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

	t.mustRegister(pc.h, subsystem, name, help)
	return pc
}

func (t *telemetryImpl) mustRegister(c prometheus.Collector, subsystem, name, help string) {
	t.registry.MustRegister(c)
	t.metricHelpMutex.Lock()
	defer t.metricHelpMutex.Unlock()
	t.metricHelp[prometheus.BuildFQName("", subsystem, name)] = help
}

func (t *telemetryImpl) Gather(filter telemetry.MetricFilter) ([]*telemetry.MetricFamily, error) {
	t.mutex.Lock()
	metricFamilies, err := t.registry.Gather()
	t.mutex.Unlock()
	if err != nil {
		return nil, err
	}

	filtered := make([]*telemetry.MetricFamily, 0, len(metricFamilies))
	for _, mf := range metricFamilies {
		if filter(mf) {
			filtered = append(filtered, mf)
		}
	}

	return filtered, nil
}

func (t *telemetryImpl) fillFlare(_ context.Context, fb flaretypes.FlareBuilder) error {
	text, err := t.GatherText(telemetry.NoFilter)
	if err != nil {
		return err
	}
	return fb.AddFile("telemetry.log", []byte(text))
}

func (t *telemetryImpl) GatherText(filter telemetry.MetricFilter) (string, error) {
	// Gather metrics
	metricFamilies, err := t.Gather(filter)
	if err != nil {
		return "", err
	}

	// Write to a buffer (or any io.Writer)
	var buf bytes.Buffer
	encoder := expfmt.NewEncoder(&buf, expfmt.NewFormat(expfmt.TypeTextPlain))
	for _, mf := range metricFamilies {
		if err := encoder.Encode(mf); err != nil {
			return "", err
		}
	}

	// buf.String() now contains the Prometheus text format
	prometheusText := buf.String()

	return prometheusText, nil
}
