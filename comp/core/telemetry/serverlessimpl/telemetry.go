// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2023-present Datadog, Inc.

//go:build serverless

package serverlessimpl

import (
	"net/http"

	"github.com/DataDog/datadog-agent/comp/core/telemetry"
	"github.com/DataDog/datadog-agent/pkg/util/fxutil"
	"go.uber.org/fx"
)

type serverlessImpl struct{}

func newTelemetry() telemetry.Component {
	return &serverlessImpl{}
}

type dummy struct{}

func (d *dummy) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	w.Write([]byte("There is no telemetry in serverless mode"))
	w.WriteHeader(200)
}

var dummyHandler = dummy{}

func (t *serverlessImpl) Handler() http.Handler {
	return &dummyHandler
}

func (t *serverlessImpl) Reset() {
	// NADA
}

func (t *serverlessImpl) NewCounter(subsystem, name string, tags []string, help string) telemetry.Counter {
	return t.NewCounterWithOpts(subsystem, name, tags, help, telemetry.DefaultOptions)
}

func (t *serverlessImpl) NewCounterWithOpts(subsystem, name string, tags []string, help string, opts telemetry.Options) telemetry.Counter {
	return &slsCounter{}

}

func (t *serverlessImpl) NewSimpleCounter(subsystem, name, help string) telemetry.SimpleCounter {
	return t.NewSimpleCounterWithOpts(subsystem, name, help, telemetry.DefaultOptions)
}

func (t *serverlessImpl) NewSimpleCounterWithOpts(subsystem, name, help string, opts telemetry.Options) telemetry.SimpleCounter {
	return &simpleNoOpCounter{}

}

func (t *serverlessImpl) NewGauge(subsystem, name string, tags []string, help string) telemetry.Gauge {
	return t.NewGaugeWithOpts(subsystem, name, tags, help, telemetry.DefaultOptions)
}

func (t *serverlessImpl) NewGaugeWithOpts(subsystem, name string, tags []string, help string, opts telemetry.Options) telemetry.Gauge {
	return &slsGauge{}

}

func (t *serverlessImpl) NewSimpleGauge(subsystem, name, help string) telemetry.SimpleGauge {
	return t.NewSimpleGaugeWithOpts(subsystem, name, help, telemetry.DefaultOptions)
}

func (t *serverlessImpl) NewSimpleGaugeWithOpts(subsystem, name, help string, opts telemetry.Options) telemetry.SimpleGauge {
	return &simpleNoOpGauge{}

}

func (t *serverlessImpl) NewHistogram(subsystem, name string, tags []string, help string, buckets []float64) telemetry.Histogram {
	return t.NewHistogramWithOpts(subsystem, name, tags, help, buckets, telemetry.DefaultOptions)
}

func (t *serverlessImpl) NewHistogramWithOpts(subsystem, name string, tags []string, help string, buckets []float64, opts telemetry.Options) telemetry.Histogram {
	return &slsHistogram{}
}

func (t *serverlessImpl) NewSimpleHistogram(subsystem, name, help string, buckets []float64) telemetry.SimpleHistogram {
	return t.NewSimpleHistogramWithOpts(subsystem, name, help, buckets, telemetry.DefaultOptions)
}

func (t *serverlessImpl) NewSimpleHistogramWithOpts(subsystem, name, help string, buckets []float64, opts telemetry.Options) telemetry.SimpleHistogram {
	return &simpleNoOpHistogram{}
}

func GetCompatComponent() telemetry.Component {
	return newTelemetry()
}

// Module defines the fx options for this component.
func Module() fxutil.Module {
	return fxutil.Component(
		fx.Provide(newTelemetry))
}
