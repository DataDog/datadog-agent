// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package httpimpl

import (
	telemetry "github.com/DataDog/datadog-agent/comp/core/telemetry/def"
)

const telemetrySubsystem = "dogstatsd_http"

// telemetryStore holds the telemetry counters for the dogstatsd http server.
type telemetryStore struct {
	requests        telemetry.Counter
	requestBytes    telemetry.Counter
	requestDuration telemetry.Counter
	metrics         telemetry.Counter
	points          telemetry.Counter
}

func newTelemetryStore(telemetrycomp telemetry.Component) *telemetryStore {
	return &telemetryStore{
		requests: telemetrycomp.NewCounter(telemetrySubsystem, "requests",
			[]string{"endpoint", "status"}, "Dogstatsd http requests count"),
		requestBytes: telemetrycomp.NewCounter(telemetrySubsystem, "request_bytes",
			[]string{"endpoint"}, "Dogstatsd http request bytes count"),
		requestDuration: telemetrycomp.NewCounter(telemetrySubsystem, "request_duration_seconds",
			[]string{"endpoint"}, "Time in seconds spent handling dogstatsd http requests"),
		metrics: telemetrycomp.NewCounter(telemetrySubsystem, "metrics",
			[]string{"endpoint", "state"}, "Count of metrics received by the dogstatsd http server"),
		points: telemetrycomp.NewCounter(telemetrySubsystem, "points",
			[]string{"endpoint", "state"}, "Count of points received by the dogstatsd http server"),
	}
}

// endpointTelemetry is a view of the store with the tags of a single endpoint
// pre-bound, so that the request path does no interior hashmap lookups.
type endpointTelemetry struct {
	requestOK           telemetry.SimpleCounter
	requestOverloaded   telemetry.SimpleCounter
	requestOriginError  telemetry.SimpleCounter
	requestTooLarge     telemetry.SimpleCounter
	requestTimeout      telemetry.SimpleCounter
	requestReadError    telemetry.SimpleCounter
	requestParseError   telemetry.SimpleCounter
	requestProcessError telemetry.SimpleCounter

	requestBytes    telemetry.SimpleCounter
	requestDuration telemetry.SimpleCounter

	metrics         telemetry.SimpleCounter
	filteredMetrics telemetry.SimpleCounter
	points          telemetry.SimpleCounter
	filteredPoints  telemetry.SimpleCounter
}

func (s *telemetryStore) forEndpoint(endpoint string) endpointTelemetry {
	return endpointTelemetry{
		requestOK:           s.requests.WithValues(endpoint, "ok"),
		requestOverloaded:   s.requests.WithValues(endpoint, "overloaded"),
		requestOriginError:  s.requests.WithValues(endpoint, "origin_error"),
		requestTooLarge:     s.requests.WithValues(endpoint, "too_large"),
		requestTimeout:      s.requests.WithValues(endpoint, "timeout"),
		requestReadError:    s.requests.WithValues(endpoint, "read_error"),
		requestParseError:   s.requests.WithValues(endpoint, "parse_error"),
		requestProcessError: s.requests.WithValues(endpoint, "process_error"),

		requestBytes:    s.requestBytes.WithValues(endpoint),
		requestDuration: s.requestDuration.WithValues(endpoint),

		metrics:         s.metrics.WithValues(endpoint, "ok"),
		filteredMetrics: s.metrics.WithValues(endpoint, "filtered"),
		points:          s.points.WithValues(endpoint, "ok"),
		filteredPoints:  s.points.WithValues(endpoint, "filtered"),
	}
}
