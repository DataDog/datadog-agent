// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package severityevents provides the shared anomaly scorer severity transition
// contract.
package severityevents

// team: q-branch

// Subscriber is the minimal severity event source exposed to consumers that
// only care about severity transitions.
type Subscriber interface {
	// SubscribeSeverityEvents registers listener, filtered/cooled down per
	// cfg. If the current level is known and is Medium or High, the initial
	// event is delivered as Low -> current level before the call returns; if
	// the current level is Low, no initial event is emitted. Otherwise the
	// dispatcher starts at Low, so the first observed non-Low level emits a
	// real escalation event. The result includes the created dispatcher and
	// unsubscribe function.
	SubscribeSeverityEvents(cfg SeverityEventsConfiguration, listener SeverityEventListener) (SeverityEventsSubscription, error)

	// SubscribeSeverityEventsReader is a convenience for pull-only consumers:
	// it registers its own internal listener per cfg and returns a Reader
	// whose GetSeverity() reflects the latest delivered level, plus the
	// unsubscribe function that stops the underlying subscription.
	SubscribeSeverityEventsReader(cfg SeverityEventsConfiguration) (SeverityEventsReaderSubscription, error)
}
