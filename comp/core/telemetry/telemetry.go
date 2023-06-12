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
