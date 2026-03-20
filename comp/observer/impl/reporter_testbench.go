// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package observerimpl

import (
	"time"

	observerdef "github.com/DataDog/datadog-agent/comp/observer/def"
)

// ReportedEvent captures a single event that would be sent to the Datadog backend
// in a live observer run. In testbench mode no network calls are made; events are
// stored here so they can be inspected via the UI and headless output.
type ReportedEvent struct {
	Pattern     string   `json:"pattern"`
	Title       string   `json:"title"`
	Message     string   `json:"message"`
	Tags        []string `json:"tags"`
	FirstSeen   int64    `json:"firstSeen"`
	LastUpdated int64    `json:"lastUpdated"`
	// FormattedTime is the human-readable UTC timestamp for FirstSeen.
	FormattedTime string `json:"formattedTime"`
}

// buildReportedEvents converts a slice of ActiveCorrelations into ReportedEvents
// using the same message formatting as the live EventReporter / eventSender.
// Each correlation produces exactly one event (no deduplication needed here since
// the testbench rebuilds from scratch on every rerun).
func buildReportedEvents(correlations []observerdef.ActiveCorrelation) []ReportedEvent {
	events := make([]ReportedEvent, 0, len(correlations))
	for _, c := range correlations {
		msg := correlationMessage(c)
		tags := []string{"source:agent-q-branch-observer", "pattern:" + c.Pattern}

		events = append(events, ReportedEvent{
			Pattern:       c.Pattern,
			Title:         c.Title,
			Message:       msg,
			Tags:          tags,
			FirstSeen:     c.FirstSeen,
			LastUpdated:   c.LastUpdated,
			FormattedTime: time.Unix(c.FirstSeen, 0).UTC().Format(time.RFC3339),
		})
	}
	return events
}
