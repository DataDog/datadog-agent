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

// TelemetryHandler is a middleware which sends a metric representing the duration of the request,
// tagged with the given server name, response status code, path, and method.
func TelemetryHandler(telemetry telemetry.Component, serverName string) mux.MiddlewareFunc {
	return telemetryHandler(telemetry, serverName, clock.New())
}

func telemetryHandler(telemetry telemetry.Component, serverName string, clock clock.Clock) mux.MiddlewareFunc {
	tags := []string{"servername", "status_code", "method", "path"}
	var buckets []float64 // use default buckets
	requestDuration := telemetry.NewHistogram(metricSubsystem, metricName, tags, metricHelp, buckets)

	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			var code int
			next = extractStatusHandler(&code)(next)

			var duration time.Duration
			next = timeHandler(clock, &duration)(next)

			next.ServeHTTP(w, r)

			path := extractPath(r)
			metricWithTags := requestDuration.WithValues(serverName, strconv.Itoa(code), r.Method, path)
			metricWithTags.Observe(duration.Seconds())
		})
	}
}
