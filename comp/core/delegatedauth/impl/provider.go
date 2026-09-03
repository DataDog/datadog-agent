// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

package delegatedauthimpl

import (
	"crypto/subtle"
	"net/http"
	"sync/atomic"
)

// apiKeyHeader is the header a resolved delegated-auth credential is stamped onto.
//
// It lives here, inside the provider, rather than at each consumer's send site. That is the point
// of the Provider interface: when delegated auth moves from API keys to stateless tokens, the
// header name and credential format change here and nowhere else.
const apiKeyHeader = "DD-Api-Key"

// credential is an immutable snapshot of what a provider currently has to offer. It is swapped
// atomically so Authorize stays lock-free on the request path.
type credential struct {
	// value is the credential to send. Only meaningful when usable is true.
	value string
	// usable reports whether the caller may send. False means "still resolving, buffer" - never
	// "send without a credential".
	usable bool
	// invalid is terminal. Resolution and fallback updates cannot replace it.
	invalid bool
}

// buffering is the credential state before the first exchange completes, and after a failure when
// no fallback key is configured.
var buffering = &credential{usable: false}

var invalidCredential = &credential{invalid: true}

// instanceProvider is the Provider for a single delegated-auth instance.
//
// Credential lifecycle:
//
//	                 ┌──────────────────────────────────────────┐
//	start ──────────▶│ resolving        Authorize -> false      │  caller buffers
//	                 └───────────┬──────────────────┬───────────┘
//	         exchange succeeded  │                  │  exchange failed
//	                             ▼                  ▼
//	        ┌────────────────────────────┐   ┌──────────────────────────────────┐
//	        │ resolved                   │   │ fallback configured?             │
//	        │ Authorize -> true (real)   │   │  yes -> Authorize -> true (static)│
//	        └────────────────────────────┘   │  no  -> Authorize -> false        │
//	                     ▲                   └──────────────┬───────────────────┘
//	                     └──────────────────────────────────┘
//	                            a later refresh succeeds
//
// A fallback is only ever used after an attempt has actually failed. While the very first exchange
// is still in flight the provider reports "not yet", so callers hold their payloads instead of
// shipping them under a key the operator only meant as a safety net.
type instanceProvider struct {
	cred atomic.Pointer[credential]
	// refreshTrigger is a buffered channel (capacity 1) that Refresh() sends to in order to
	// nudge the background goroutine into an immediate re-exchange. nil when no background
	// refresh goroutine is running (no cloud provider detected), in which case Refresh()
	// returns false.
	refreshTrigger chan struct{}
}

func newInstanceProvider() *instanceProvider {
	p := &instanceProvider{}
	p.cred.Store(buffering)
	return p
}

// setRefreshTrigger attaches the channel that Refresh() uses to nudge the background goroutine.
// Called by the component only when a background refresh goroutine is about to start; when no
// cloud provider was detected the channel stays nil and Refresh() returns false.
func (p *instanceProvider) setRefreshTrigger(ch chan struct{}) {
	p.refreshTrigger = ch
}

// Authorize implements delegatedauth.Provider.
func (p *instanceProvider) Authorize(h http.Header) bool {
	c := p.cred.Load()
	if c == nil || c.invalid || !c.usable {
		return false
	}
	h.Set(apiKeyHeader, c.value)
	return true
}

// Refresh implements delegatedauth.Provider. It resets the credential to buffering so Authorize
// returns false (no further sends under the stale key) and nudges the background goroutine to
// re-exchange as soon as possible.
//
// The send is non-blocking and the channel has capacity 1, so a burst of 403s from many
// in-flight transactions coalesces into a single refresh — no storm.
func (p *instanceProvider) Refresh() bool {
	if p.refreshTrigger == nil {
		return false
	}
	for {
		current := p.cred.Load()
		if current == nil || current.invalid {
			return false
		}
		if p.cred.CompareAndSwap(current, buffering) {
			break
		}
	}
	select {
	case p.refreshTrigger <- struct{}{}:
	default:
		// A refresh is already queued or in progress; this call coalesces into it.
	}
	return true
}

// setResolved records a credential fetched from the cloud provider. It always wins over a
// fallback, including when a refresh recovers after earlier failures.
func (p *instanceProvider) setResolved(key string) bool {
	for {
		current := p.cred.Load()
		if current != nil && current.invalid {
			return false
		}
		if p.cred.CompareAndSwap(current, &credential{value: key, usable: true}) {
			return true
		}
	}
}

// invalidate stops new requests from using this provider.
func (p *instanceProvider) invalidate() {
	p.cred.Store(invalidCredential)
}

// setFallback records the operator-supplied static key after an exchange has failed. It does not
// overwrite an already-resolved credential: a transient refresh failure should keep using the last
// known-good key rather than regress to the fallback.
func (p *instanceProvider) setFallback(key string) bool {
	if key == "" {
		// Nothing to fall back to - keep buffering and let retries continue.
		return false
	}
	for {
		current := p.cred.Load()
		if current != nil && (current.invalid || current.usable) {
			return false
		}
		if p.cred.CompareAndSwap(current, &credential{value: key, usable: true}) {
			return true
		}
	}
}

// hasCredential reports whether the provider can currently authorize a request. For status
// reporting and tests.
func (p *instanceProvider) hasCredential() bool {
	c := p.cred.Load()
	return c != nil && !c.invalid && c.usable
}

func (p *instanceProvider) matches(value string) bool {
	c := p.cred.Load()
	return c != nil && !c.invalid && c.usable && subtle.ConstantTimeCompare([]byte(c.value), []byte(value)) == 1
}
