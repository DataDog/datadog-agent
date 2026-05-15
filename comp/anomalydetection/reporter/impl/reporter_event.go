// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package reporterimpl

import (
	observerdef "github.com/DataDog/datadog-agent/comp/anomalydetection/observer/def"
	reporterdef "github.com/DataDog/datadog-agent/comp/anomalydetection/reporter/def"
	log "github.com/DataDog/datadog-agent/comp/core/log/def"
)

// EventReporter sends Datadog events for new correlations via eventSender.
// It tracks seen correlations and only fires when a correlation first appears
// or reappears after going inactive.
// It implements reporterdef.StorageConsumer so the observer can inject engine
// storage post-construction for windowed log-rate annotations in change messages.
type EventReporter struct {
	sender           *eventSender
	logger           log.Component
	seenCorrelations map[string]bool // pattern -> reported
	// activeBefore tracks patterns observed in ActiveCorrelations on a prior
	// advance. Only entries marked here are eligible for recurrence cleanup;
	// patterns that only ever appear via CorrelationHistory (e.g. batch
	// detector clusters) are emitted once and never deleted.
	activeBefore map[string]bool
}

// Ensure EventReporter satisfies both interfaces at compile time.
var _ reporterdef.Reporter = (*EventReporter)(nil)
var _ reporterdef.StorageConsumer = (*EventReporter)(nil)

// Name returns the reporter name.
func (r *EventReporter) Name() string {
	return "event_reporter"
}

// SetStorage implements reporterdef.StorageConsumer.
// Called by the observer after engine construction to enable windowed log-rate
// annotations in change-event messages.
func (r *EventReporter) SetStorage(storage observerdef.StorageReader) {
	r.sender.storage = storage
}

// Report checks for new correlations and sends a Datadog change event for each one.
//
// Emission is driven by output.CorrelationHistory so that batch-detector
// clusters — whose changepoint timestamps may already be evicted from the
// sliding window — still fire. Recurrence cleanup is driven by
// output.ActiveCorrelations: a streaming pattern that leaves the active set
// is removed from seenCorrelations so it fires again on the next activation.
// Patterns that only ever appear via CorrelationHistory (no active phase) are
// emitted once and stay seen for the lifetime of the agent.
func (r *EventReporter) Report(output reporterdef.ReportOutput) {
	if r.seenCorrelations == nil {
		r.seenCorrelations = make(map[string]bool)
	}
	if r.activeBefore == nil {
		r.activeBefore = make(map[string]bool)
	}

	// Build the set of currently active patterns.
	currentlyActive := make(map[string]bool, len(output.ActiveCorrelations))
	for _, ac := range output.ActiveCorrelations {
		currentlyActive[ac.Pattern] = true
	}

	// Send an event for each newly-seen correlation. Mark the pattern as
	// seen only after a successful send: a transient forwarder error leaves
	// the pattern unmarked so the next advance retries publication. A
	// persistent failure will keep producing one error log per advance until
	// either the forwarder recovers or the correlation goes inactive.
	for _, ac := range output.CorrelationHistory {
		if !r.seenCorrelations[ac.Pattern] {
			if err := r.sender.send(ac); err != nil {
				r.logger.Errorf("[observer] failed to send event for pattern %s: %v", ac.Pattern, err)
				continue
			}
			r.seenCorrelations[ac.Pattern] = true
		}
	}

	// Recurrence cleanup: remove seenCorrelations entries for patterns that
	// were previously active and are no longer active. This lets streaming
	// patterns re-fire when they come back. Patterns that have never been in
	// ActiveCorrelations (batch-only) are left alone — they fire once and
	// stay marked.
	for pattern := range r.activeBefore {
		if !currentlyActive[pattern] {
			delete(r.seenCorrelations, pattern)
			delete(r.activeBefore, pattern)
		}
	}
	// Carry the current active set forward for the next advance.
	for pattern := range currentlyActive {
		r.activeBefore[pattern] = true
	}
}
