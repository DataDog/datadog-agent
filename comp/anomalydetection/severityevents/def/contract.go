// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package severityevents provides the shared anomaly scorer severity transition
// contract.
package severityevents

// team: q-branch

// Subscriber is the minimal push-based severity event source exposed to
// consumers that only care about severity transitions.
type Subscriber interface {
	// SubscribeSeverityEvents registers the listener described by cfg. If the
	// current level is known, an initial synthetic event is delivered before it
	// returns. Otherwise the dispatcher starts at Low, so the first observed
	// non-Low level emits a real escalation event. The result includes the
	// created dispatcher and unsubscribe function.
	SubscribeSeverityEvents(cfg SeverityEventsConfiguration) (SeverityEventsSubscription, error)
}
