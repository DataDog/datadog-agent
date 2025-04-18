// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

// Package apiobserverimpl implements the apiobserver component interface
package apiobserverimpl

import (
	"net/http"
	"strconv"
	"time"

	"github.com/benbjohnson/clock"

	"github.com/gorilla/mux"

	apiobserver "github.com/DataDog/datadog-agent/comp/api/apiobserver/def"
	"github.com/DataDog/datadog-agent/comp/core/telemetry"
)

const (
	// MetricSubsystem is the subsystem for the metric
	MetricSubsystem = "api_server"
	// MetricName is the name of the metric
	MetricName = "request_duration_seconds"
	metricHelp = "Request duration distribution by server, method, path, and status (in seconds)."
)

// Requires defines the dependencies for the telemetry component
type Requires struct {
	Telemetry telemetry.Component
}

// Provides defines the output of the telemetry component
type Provides struct {
	Comp apiobserver.Component
}

type apiObserverFactory struct {
	requestDuration telemetry.Histogram
	clock           clock.Clock
}

// NewComponent creates a new apiobserver component
func NewComponent(reqs Requires) Provides {
	return newComponentWithClock(reqs.Telemetry, clock.New())
}

func newComponentWithClock(telemetry telemetry.Component, clock clock.Clock) Provides {
	tags := []string{"servername", "status_code", "method", "path", "auth"}
	var buckets []float64 // use default buckets
	requestDuration := telemetry.NewHistogram(MetricSubsystem, MetricName, tags, metricHelp, buckets)

	return Provides{
		Comp: &apiObserverFactory{
			requestDuration,
			clock,
		},
	}
}

func (th *apiObserverFactory) Middleware(serverName string, authTagGetter func(r *http.Request) string) mux.MiddlewareFunc {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			var statusCode int
			// next is an argument of the MiddlewareFunc, it is defined outside the HandlerFunc so it is shared between calls,
			// and so it must not be updated otherwise every call of the HandlerFunc will add a new layer of middlewares
			// (and the HandlerFunc is called multiple times)
			next := extractStatusCodeHandler(&statusCode)(next)

			var duration time.Duration
			next = timeHandler(th.clock, &duration)(next)

			next.ServeHTTP(w, r)

			path := extractPath(r)

			durationSeconds := duration.Seconds()

			auth := authTagGetter(r)

			th.requestDuration.Observe(durationSeconds, serverName, strconv.Itoa(statusCode), r.Method, path, auth)
		})
	}
}
