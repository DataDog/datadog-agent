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
// Correlations are provided via output.ActiveCorrelations from the engine's event subscription.
func (r *EventReporter) Report(output reporterdef.ReportOutput) {
	if r.seenCorrelations == nil {
		r.seenCorrelations = make(map[string]bool)
	}

	activeCorrelations := output.ActiveCorrelations

	// Build the set of currently active patterns.
	currentlyActive := make(map[string]bool, len(activeCorrelations))
	for _, ac := range activeCorrelations {
		currentlyActive[ac.Pattern] = true
	}

	// Send an event for each newly-seen correlation.
	for _, ac := range activeCorrelations {
		if !r.seenCorrelations[ac.Pattern] {
			if err := r.sender.send(ac); err != nil {
				r.logger.Errorf("[observer] failed to send event for pattern %s: %v", ac.Pattern, err)
			}
			r.seenCorrelations[ac.Pattern] = true
		}
	}

	// Remove correlations that are no longer active so they can fire again if they recur.
	for pattern := range r.seenCorrelations {
		if !currentlyActive[pattern] {
			delete(r.seenCorrelations, pattern)
		}
	}
}
