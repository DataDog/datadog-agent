// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

// Package intakerouting asks the Datadog backend where to send telemetry.
//
// A consumer registers the endpoints it would normally send to. For each one, the component
// reports either that endpoint or another host chosen by the backend (a customer-specific
// endpoint, or a specialized one currently handling that customer), and notifies the consumer
// whenever the answer changes. See ARCHITECTURE.md in the component directory for the full design.
package intakerouting

import "context"

// team: agent-data-plane

// TelemetryType identifies the kind of telemetry an endpoint receives. The backend answers
// separately for each type.
type TelemetryType string

const (
	// Logs is the core logs agent's HTTP intake (agent-http-intake.logs.<site>).
	Logs TelemetryType = "logs"
)

// Endpoint is an intake host. It is always reached over HTTPS.
type Endpoint struct {
	Host string
	// Port is the TCP port. 0 means the default HTTPS port (443).
	Port int
}

// Request describes one endpoint a consumer wants routed.
type Request struct {
	// Type is the telemetry type the endpoint receives.
	Type TelemetryType
	// Default is the endpoint derived from the configuration. The Datadog site to query is taken
	// from its host. A host outside an official Datadog domain is never routed.
	Default Endpoint
	// APIKey returns the endpoint's current API key and whether it can be used yet (for example,
	// it is false while delegated auth has not delivered a key). It is called every few seconds
	// and before every query and pre-flight check, so key rotation is picked up. It must be cheap,
	// must not block, and must not call back into the component.
	APIKey func() (apiKey string, ready bool)
}

// Component asks the Datadog backend where to send telemetry.
type Component interface {
	// Register starts routing the given endpoints. It never blocks: the first query is sent in
	// the background. The returned Registration reports one endpoint per request, in order.
	Register(reqs []Request) Registration
}

// Registration is the routing state of the endpoints passed to Register.
type Registration interface {
	// WaitInitial waits until every endpoint has its first decision, the startup grace period
	// (counted from Register) has elapsed, or ctx is done, whichever comes first. It then returns
	// Current().
	WaitInitial(ctx context.Context) []Endpoint
	// Current returns, for each request in order, the endpoint to send to: the backend's choice if
	// one is active, otherwise the request's default.
	Current() []Endpoint
	// Changes receives a value when Current() may have changed. Notifications are coalesced
	// (the channel has a capacity of 1), so consumers must compare Current() with what they use.
	// The channel is never closed.
	Changes() <-chan struct{}
	// Close stops routing these endpoints. It is idempotent. Once it returns, nothing more is
	// sent on Changes.
	Close()
}
