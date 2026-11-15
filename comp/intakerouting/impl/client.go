// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package intakeroutingimpl

import (
	"context"

	intakerouting "github.com/DataDog/datadog-agent/comp/intakerouting/def"
)

// routingClient talks to the routing backend. The router only depends on this interface, so the
// wire format can change without touching the scheduling logic. httpRoutingClient is the only
// implementation, and it speaks a provisional format until the backend contract is final.
type routingClient interface {
	// query asks the backend of site where the telemetry of apiKey should be sent. The answer holds
	// every telemetry type the backend knows of: the router picks the entries it needs.
	query(ctx context.Context, site, apiKey string) queryResult
	// preflight checks that ep accepts apiKey. Telemetry is only switched to ep if it passes.
	preflight(ctx context.Context, ep intakerouting.Endpoint, apiKey string) error
}

// Query statuses. They are also the values of the "status" tag of the queries counter.
const (
	// statusSuccess is a parseable answer.
	statusSuccess = "success"
	// statusNotOffered means the backend does not offer routing (HTTP 404). Like statusSuccess, it
	// settles the query: the current endpoints are kept until the next refresh.
	statusNotOffered = "not_offered"
	// statusSkipped means the query was not sent, because no endpoint of the group may be routed
	// (see checkPaths). Like statusNotOffered, it settles the query. The queries counter does not
	// count it.
	statusSkipped = "skipped"
	// The other statuses are failures: the current endpoints are kept and the query is retried.
	statusNetworkError    = "network_error"
	statusTimeout         = "timeout"
	statusHTTPError       = "http_error"
	statusInvalidResponse = "invalid_response"
)

// queryResult is the outcome of a routing query.
type queryResult struct {
	status string
	// code is the HTTP status code, when a response was received.
	code int
	// routes holds the answer for each telemetry type when status is statusSuccess. A type with
	// no entry got no answer.
	routes map[intakerouting.TelemetryType]routeAnswer
	// err describes a failure.
	err error
}

// settled reports whether the query got an answer, which ends the startup retries.
func (q queryResult) settled() bool {
	return q.status == statusSuccess || q.status == statusNotOffered || q.status == statusSkipped
}

// routeAnswer is the backend's answer for one telemetry type.
type routeAnswer struct {
	// override is false when the backend explicitly says to use the default endpoint.
	override bool
	host     string
	port     int
}
