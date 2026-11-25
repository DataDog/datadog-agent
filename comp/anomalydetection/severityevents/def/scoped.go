// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package severityevents

// This file defines service-scoped severity keys, events, and subscriber contracts.

// ScopeKey is a 64-bit hash identifying a scoring scope (0 is reserved for empty).
type ScopeKey uint64

// ScopeKeyForService hashes service with FNV-1a without allocating (0 if empty, raw 0 -> 1).
func ScopeKeyForService(service string) ScopeKey {
	if service == "" {
		return 0
	}
	const (
		offset64      = 14695981039346656037
		prime64       = 1099511628211
		servicePrefix = "service:"
	)
	hash := uint64(offset64)
	for index := 0; index < len(servicePrefix); index++ {
		hash ^= uint64(servicePrefix[index])
		hash *= prime64
	}
	for index := 0; index < len(service); index++ {
		hash ^= uint64(service[index])
		hash *= prime64
	}
	if hash == 0 {
		return 1
	}
	return ScopeKey(hash)
}

// ScopedSeverityEvent pairs a severity state-machine transition with its ScopeKey.
type ScopedSeverityEvent struct {
	ScopeKey ScopeKey      `json:"scope_key,string"`
	Event    SeverityEvent `json:"event"`
}

// ScopedSeverityEventListener receives severity transitions across active scopes.
type ScopedSeverityEventListener interface {
	OnScopedSeverityTransition(event ScopedSeverityEvent)
}

// ScopedSeverityEventsSubscription is returned by SubscribeScopedSeverityEvents.
type ScopedSeverityEventsSubscription struct {
	Unsubscribe func()
}

// ScopedSubscriber exposes scoped severity transition subscriptions.
type ScopedSubscriber interface {
	// SubscribeScopedSeverityEvents registers listener, bootstrapping active elevated scopes and returning an idempotent Unsubscribe.
	SubscribeScopedSeverityEvents(
		cfg SeverityEventsConfiguration,
		listener ScopedSeverityEventListener,
	) (ScopedSeverityEventsSubscription, error)
}
