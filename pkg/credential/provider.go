// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

// Package credential holds the canonical types for credential providers in the
// agent. It is importable by both comp/ (the agent) and pkg/trace (the trace
// agent, which is vendored by the OTel Collector), so neither side needs to
// redeclare the Provider interface or duplicate the provider-resolution logic.
package credential

import "net/http"

// Provider supplies the credential for outbound requests to one destination.
// Implementations are safe for concurrent use and are cheap enough to call on
// every request.
//
// The canonical declaration lives here so that pkg/trace (which cannot import
// comp/) and comp/ (which can) share the same type. comp/core/delegatedauth/def
// re-exports it as an alias.
type Provider interface {
	// Authorize stamps the credential onto h and reports whether it did.
	//
	// A false return means no credential is available yet. The caller MUST NOT
	// send the request; it should retain the payload and retry, so nothing is
	// lost while the first exchange with the cloud provider is still in flight.
	// It never means "send unauthenticated".
	//
	// Which header is set, and whether the credential is an API key or a token,
	// is the provider's business — that is the point of the interface. Callers
	// only learn whether they may send.
	Authorize(h http.Header) bool

	// Refresh signals that the current credential has been rejected (e.g. a 403
	// from the intake) and should be re-exchanged as soon as possible. It resets
	// the credential to its buffering state so Authorize returns false until a
	// new exchange succeeds, preventing further sends under a stale key.
	//
	// Returns true when a background refresh was queued, false when no refresh
	// mechanism is available. Callers should treat false as "drop the
	// transaction" and true as "reschedule it".
	//
	// Anti-storm: the underlying trigger is a buffered channel of capacity 1
	// with a non-blocking send, so a burst of 403s from many in-flight
	// transactions coalesces into a single refresh.
	Refresh() bool
}
