// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package bench

import (
	"time"

	observerdef "github.com/DataDog/datadog-agent/comp/anomalydetection/observer/def"
	observerimpl "github.com/DataDog/datadog-agent/comp/anomalydetection/observer/impl"
	reporterimpl "github.com/DataDog/datadog-agent/comp/anomalydetection/reporter/impl"
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

// buildReportedEvents builds the set of ReportedEvents from a correlation history
// (used after replay to compute the final event log).
// storage is nil — log rate annotations fall back to DebugInfo.CurrentValue.
func buildReportedEvents(correlations []observerdef.ActiveCorrelation, _ observerimpl.StateView) []ReportedEvent {
	events := make([]ReportedEvent, 0, len(correlations))
	for _, ac := range correlations {
		msg := reporterimpl.BuildChangeMessage(ac, nil)
		tags := reporterimpl.BuildEventTags(ac)
		events = append(events, ReportedEvent{
			Pattern:       ac.Pattern,
			Title:         ac.Title,
			Message:       msg,
			Tags:          tags,
			FirstSeen:     ac.FirstSeen,
			LastUpdated:   ac.LastUpdated,
			FormattedTime: time.Unix(ac.FirstSeen, 0).UTC().Format(time.RFC3339),
		})
	}
	return events
}
