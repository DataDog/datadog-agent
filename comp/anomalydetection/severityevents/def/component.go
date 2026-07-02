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
	// SubscribeScorer registers a severity transition listener described by cfg.
	// If the current severity level is already known, an initial synthetic
	// event (FromLevel == ToLevel == current level, Direction ==
	// AnomalyScorerEventBoth) is delivered synchronously before
	// SubscribeScorer returns, so a subscriber joining mid-stream learns the
	// current state immediately instead of only future transitions.
	// Returns an unsubscribe function; call it to stop delivery.
	SubscribeScorer(cfg AnomalyScorerConfiguration) func()
}
