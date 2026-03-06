// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package observerimpl

import (
	log "github.com/DataDog/datadog-agent/comp/core/log/def"
	observerdef "github.com/DataDog/datadog-agent/comp/observer/def"
)

// EventReporter sends Datadog events for new correlations via eventSender.
// It tracks seen correlations and only fires when a correlation first appears.
type EventReporter struct {
	sender           *eventSender
	logger           log.Component
	correlationState observerdef.CorrelationState
	seenCorrelations map[string]bool // pattern -> reported
}

// Name returns the reporter name.
func (r *EventReporter) Name() string {
	return "event_reporter"
}

// SetCorrelationState sets the correlation state source for the reporter.
func (r *EventReporter) SetCorrelationState(state observerdef.CorrelationState) {
	r.correlationState = state
	r.seenCorrelations = make(map[string]bool)
}

// Report checks for new correlations and sends an event for each one.
func (r *EventReporter) Report(_ observerdef.ReportOutput) {
	if r.correlationState == nil {
		return
	}

	activeCorrelations := r.correlationState.ActiveCorrelations()

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
