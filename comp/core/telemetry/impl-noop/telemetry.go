// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2023-present Datadog, Inc.

// Package implnoop creates the noop telemetry component
package implnoop

import (
	"net/http"

	dto "github.com/prometheus/client_model/go"
	"go.uber.org/fx"

	"github.com/DataDog/datadog-agent/comp/core/telemetry"
	telemetrydef "github.com/DataDog/datadog-agent/comp/core/telemetry/def"
	"github.com/DataDog/datadog-agent/pkg/util/fxutil"
	"github.com/prometheus/client_golang/prometheus"
)

type noopImpl struct{}

func newTelemetry() telemetry.Component {
	return &noopImpl{}
}

// NewComponent creates a new noop telemetry component
func NewComponent() telemetry.Component {
	return newTelemetry()
}

type dummy struct{}

func (d *dummy) ServeHTTP(w http.ResponseWriter, _ *http.Request) {
	w.Write([]byte("Telemetry is not enabled"))
	w.WriteHeader(200)
}

var dummyHandler = dummy{}

func (t *noopImpl) Handler() http.Handler {
	return &dummyHandler
}

func (t *noopImpl) Reset() {
}

func (t *noopImpl) NewCounter(subsystem, name string, tags []string, help string) telemetrydef.Counter {
	return t.NewCounterWithOpts(subsystem, name, tags, help, telemetrydef.DefaultOptions)
}

func (t *noopImpl) NewCounterWithOpts(_, _ string, _ []string, _ string, _ telemetrydef.Options) telemetrydef.Counter {
	return &slsCounter{}

}

func (t *noopImpl) NewSimpleCounter(subsystem, name, help string) telemetrydef.SimpleCounter {
	return t.NewSimpleCounterWithOpts(subsystem, name, help, telemetrydef.DefaultOptions)
}

func (t *noopImpl) NewSimpleCounterWithOpts(_, _, _ string, _ telemetrydef.Options) telemetrydef.SimpleCounter {
	return &simpleNoOpCounter{}

}

func (t *noopImpl) NewGauge(subsystem, name string, tags []string, help string) telemetrydef.Gauge {
	return t.NewGaugeWithOpts(subsystem, name, tags, help, telemetrydef.DefaultOptions)
}

func (t *noopImpl) NewGaugeWithOpts(_, _ string, _ []string, _ string, _ telemetrydef.Options) telemetrydef.Gauge {
	return &slsGauge{}

}

func (t *noopImpl) NewSimpleGauge(subsystem, name, help string) telemetrydef.SimpleGauge {
	return t.NewSimpleGaugeWithOpts(subsystem, name, help, telemetrydef.DefaultOptions)
}

func (t *noopImpl) NewSimpleGaugeWithOpts(_, _, _ string, _ telemetrydef.Options) telemetrydef.SimpleGauge {
	return &simpleNoOpGauge{}

}

func (t *noopImpl) NewHistogram(subsystem, name string, tags []string, help string, buckets []float64) telemetrydef.Histogram {
	return t.NewHistogramWithOpts(subsystem, name, tags, help, buckets, telemetrydef.DefaultOptions)
}

func (t *noopImpl) NewHistogramWithOpts(_, _ string, _ []string, _ string, _ []float64, _ telemetrydef.Options) telemetrydef.Histogram {
	return &slsHistogram{}
}

func (t *noopImpl) NewSimpleHistogram(subsystem, name, help string, buckets []float64) telemetrydef.SimpleHistogram {
	return t.NewSimpleHistogramWithOpts(subsystem, name, help, buckets, telemetrydef.DefaultOptions)
}

func (t *noopImpl) NewSimpleHistogramWithOpts(_, _, _ string, _ []float64, _ telemetrydef.Options) telemetrydef.SimpleHistogram {
	return &simpleNoOpHistogram{}
}

func (t *noopImpl) RegisterCollector(prometheus.Collector) {}

func (t *noopImpl) UnregisterCollector(prometheus.Collector) bool {
	return true
}

func (t *noopImpl) Gather(bool) ([]*dto.MetricFamily, error) {
	return nil, nil
}

// GetCompatComponent returns a component wrapping telemetry global variables
// TODO (components): Remove this when all telemetry is migrated to the component
func GetCompatComponent() telemetry.Component {
	return newTelemetry()
}

// Module defines the fx options for this component.
func Module() fxutil.Module {
	return fxutil.Component(
		fx.Provide(newTelemetry))
}
