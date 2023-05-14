// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package telemetry

import (
	"net/http"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/collectors"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

var (
	telemetryRegistry = prometheus.NewRegistry()
)

func init() {
	telemetryRegistry.MustRegister(collectors.NewProcessCollector(collectors.ProcessCollectorOpts{}))
	telemetryRegistry.MustRegister(collectors.NewGoCollector())
}

// Handler serves the HTTP route containing the prometheus metrics.
func Handler() http.Handler {
	return promhttp.HandlerFor(telemetryRegistry, promhttp.HandlerOpts{})
}

// Reset resets the global telemetry registry, stopping the collection of every previously registered metrics.
// Mainly used for unit tests and integration tests.
func Reset() {
	telemetryRegistry = prometheus.NewRegistry()
}
