// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2023-present Datadog, Inc.

// package noopsimpl creates the noop telemetry component
package noopsimpl

import (
	"net/http"

	"github.com/DataDog/datadog-agent/comp/core/telemetry"
	"github.com/DataDog/datadog-agent/pkg/util/fxutil"
	"go.uber.org/fx"
)

type noopImpl struct{}

func newTelemetry() telemetry.Component {
	return &noopImpl{}
}

type dummy struct{}

func (d *dummy) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	w.Write([]byte("Telemtry is not enabled"))
	w.WriteHeader(200)
}

var dummyHandler = dummy{}

func (t *noopImpl) Handler() http.Handler {
	return &dummyHandler
}

func (t *noopImpl) Reset() {
}

func (t *noopImpl) NewCounter(subsystem, name string, tags []string, help string) telemetry.Counter {
	return t.NewCounterWithOpts(subsystem, name, tags, help, telemetry.DefaultOptions)
}

func (t *noopImpl) NewCounterWithOpts(subsystem, name string, tags []string, help string, opts telemetry.Options) telemetry.Counter {
	return &slsCounter{}

}

func (t *noopImpl) NewSimpleCounter(subsystem, name, help string) telemetry.SimpleCounter {
	return t.NewSimpleCounterWithOpts(subsystem, name, help, telemetry.DefaultOptions)
}

func (t *noopImpl) NewSimpleCounterWithOpts(subsystem, name, help string, opts telemetry.Options) telemetry.SimpleCounter {
	return &simpleNoOpCounter{}

}

func (t *noopImpl) NewGauge(subsystem, name string, tags []string, help string) telemetry.Gauge {
	return t.NewGaugeWithOpts(subsystem, name, tags, help, telemetry.DefaultOptions)
}

func (t *noopImpl) NewGaugeWithOpts(subsystem, name string, tags []string, help string, opts telemetry.Options) telemetry.Gauge {
	return &slsGauge{}

}

func (t *noopImpl) NewSimpleGauge(subsystem, name, help string) telemetry.SimpleGauge {
	return t.NewSimpleGaugeWithOpts(subsystem, name, help, telemetry.DefaultOptions)
}

func (t *noopImpl) NewSimpleGaugeWithOpts(subsystem, name, help string, opts telemetry.Options) telemetry.SimpleGauge {
	return &simpleNoOpGauge{}

}

func (t *noopImpl) NewHistogram(subsystem, name string, tags []string, help string, buckets []float64) telemetry.Histogram {
	return t.NewHistogramWithOpts(subsystem, name, tags, help, buckets, telemetry.DefaultOptions)
}

func (t *noopImpl) NewHistogramWithOpts(subsystem, name string, tags []string, help string, buckets []float64, opts telemetry.Options) telemetry.Histogram {
	return &slsHistogram{}
}

func (t *noopImpl) NewSimpleHistogram(subsystem, name, help string, buckets []float64) telemetry.SimpleHistogram {
	return t.NewSimpleHistogramWithOpts(subsystem, name, help, buckets, telemetry.DefaultOptions)
}

func (t *noopImpl) NewSimpleHistogramWithOpts(subsystem, name, help string, buckets []float64, opts telemetry.Options) telemetry.SimpleHistogram {
	return &simpleNoOpHistogram{}
}

func (t *noopImpl) Meter(name string, opts ...telemetry.MeterOption) telemetry.Meter {
	return nil
}

func (t *noopImpl) RegisterCollector(c telemetry.Collector) {}

func (t *noopImpl) UnregisterCollector(c telemetry.Collector) bool {
	return true
}

func (t *noopImpl) GatherDefault() ([]*telemetry.MetricFamily, error) {
	return nil, nil
}

func GetCompatComponent() telemetry.Component {
	return newTelemetry()
}

// Module defines the fx options for this component.
func Module() fxutil.Module {
	return fxutil.Component(
		fx.Provide(newTelemetry))
}
