// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package observability

import (
	"net/http"
	"strconv"
	"time"

	"github.com/benbjohnson/clock"
	"github.com/gorilla/mux"

	"github.com/DataDog/datadog-agent/comp/core/telemetry"
)

const (
	metricSubsystem = "api_server"
	metricName      = "request_duration_seconds"
	metricHelp      = "Request duration distribution by server, method, path, and status (in seconds)."
)

type telemetryMiddlewareFactory struct {
	requestDuration telemetry.Histogram
	clock           clock.Clock
}

// TelemetryMiddlewareFactory creates a telemetry middleware tagged with a given server name
type TelemetryMiddlewareFactory interface {
	Middleware(serverName string) mux.MiddlewareFunc
}

func (th *telemetryMiddlewareFactory) Middleware(serverName string) mux.MiddlewareFunc {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			var statusCode int
			next = extractStatusCodeHandler(&statusCode)(next)

			var duration time.Duration
			next = timeHandler(th.clock, &duration)(next)

			next.ServeHTTP(w, r)

			path := extractPath(r)
			th.requestDuration.Observe(duration.Seconds(), serverName, strconv.Itoa(statusCode), r.Method, path)
		})
	}
}

func newTelemetryMiddlewareFactory(telemetry telemetry.Component, clock clock.Clock) TelemetryMiddlewareFactory {
	tags := []string{"servername", "status_code", "method", "path"}
	var buckets []float64 // use default buckets
	requestDuration := telemetry.NewHistogram(metricSubsystem, metricName, tags, metricHelp, buckets)

	return &telemetryMiddlewareFactory{
		requestDuration,
		clock,
	}
}

// NewTelemetryMiddlewareFactory creates a new TelemetryMiddlewareFactory
//
// This function must be called only once for a given telemetry Component,
// as it creates a new metric, and Prometheus panics if the same metric is registered twice.
func NewTelemetryMiddlewareFactory(telemetry telemetry.Component) TelemetryMiddlewareFactory {
	return newTelemetryMiddlewareFactory(telemetry, clock.New())
}
