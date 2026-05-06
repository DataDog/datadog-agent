// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package bench

import (
	"fmt"
	"slices"
	"sort"
	"strings"
	"time"

	observerdef "github.com/DataDog/datadog-agent/comp/anomalydetection/observer/def"
	observerimpl "github.com/DataDog/datadog-agent/comp/anomalydetection/observer/impl"
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

// replayReporter mirrors EventReporter.Report() during testbench replay.
// It tracks which patterns are currently active and appends a new ReportedEvent
// each time a pattern first appears or reappears after going inactive.
type replayReporter struct {
	seenPatterns map[string]bool
	events       []ReportedEvent
	sv           observerimpl.StateView
}

// Name satisfies observerdef.Reporter.
func (r *replayReporter) Name() string { return "replay_reporter" }

// Report fires a new ReportedEvent for each pattern that is newly active.
func (r *replayReporter) Report(output observerdef.ReportOutput) {
	if r.seenPatterns == nil {
		r.seenPatterns = make(map[string]bool)
	}

	currentlyActive := make(map[string]bool, len(output.ActiveCorrelations))
	for _, ac := range output.ActiveCorrelations {
		currentlyActive[ac.Pattern] = true
	}

	for _, ac := range output.ActiveCorrelations {
		if !r.seenPatterns[ac.Pattern] {
			msg := buildLocalChangeMessage(ac)
			tags := buildLocalEventTags(ac)
			r.events = append(r.events, ReportedEvent{
				Pattern:       ac.Pattern,
				Title:         ac.Title,
				Message:       msg,
				Tags:          tags,
				FirstSeen:     ac.FirstSeen,
				LastUpdated:   ac.LastUpdated,
				FormattedTime: time.Unix(ac.FirstSeen, 0).UTC().Format(time.RFC3339),
			})
			r.seenPatterns[ac.Pattern] = true
		}
	}

	for pattern := range r.seenPatterns {
		if !currentlyActive[pattern] {
			delete(r.seenPatterns, pattern)
		}
	}
}

// buildReportedEvents builds the set of ReportedEvents from a correlation history
// (used after replay to compute the final event log).
func buildReportedEvents(correlations []observerdef.ActiveCorrelation, _ observerimpl.StateView) []ReportedEvent {
	events := make([]ReportedEvent, 0, len(correlations))
	for _, ac := range correlations {
		msg := buildLocalChangeMessage(ac)
		tags := buildLocalEventTags(ac)
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

// buildLocalChangeMessage creates a compact human-readable summary for a correlation.
func buildLocalChangeMessage(c observerdef.ActiveCorrelation) string {
	var lines []string
	lines = append(lines, fmt.Sprintf("Correlated behavior change detected: %d anomalies in pattern %q", len(c.Anomalies), c.Pattern))
	lines = append(lines, "")

	anomalyLines := []string{}
	for _, a := range c.Anomalies {
		if a.DebugInfo != nil {
			display := a.Source.DisplayName()
			if display == "" {
				display = a.Source.String()
			}
			anomalyLines = append(anomalyLines, fmt.Sprintf("- %s: %.2f (baseline mean: %.2f, %.1f sigma)", display, a.DebugInfo.CurrentValue, a.DebugInfo.BaselineMean, a.DebugInfo.DeviationSigma))
		} else if a.Description != "" {
			anomalyLines = append(anomalyLines, "- "+a.Description)
		} else {
			display := a.Source.DisplayName()
			if display == "" {
				display = a.Source.String()
			}
			anomalyLines = append(anomalyLines, "- "+display)
		}
	}

	slices.Sort(anomalyLines)
	lines = append(lines, slices.Compact(anomalyLines)...)
	text := strings.Join(lines, "\n")
	const maxLen = 4000
	if len(text) > maxLen {
		text = text[:maxLen-3] + "..."
	}
	return text
}

// buildLocalEventTags returns the Datadog event tags for a correlation.
func buildLocalEventTags(c observerdef.ActiveCorrelation) []string {
	hasMetric := false
	hasLog := false
	dimensionSet := make(map[string]struct{})

	for _, a := range c.Anomalies {
		if a.Type == observerdef.AnomalyTypeLog {
			hasLog = true
		} else {
			hasMetric = true
		}
		for _, t := range a.Source.Tags {
			for _, prefix := range []string{"service:", "env:", "host:"} {
				if strings.HasPrefix(t, prefix) {
					dimensionSet[t] = struct{}{}
					break
				}
			}
		}
		if a.Context != nil {
			for _, k := range []string{"service", "env", "host"} {
				if v, ok := a.Context.SplitTags[k]; ok {
					dimensionSet[k+":"+v] = struct{}{}
				}
			}
		}
	}

	tags := []string{"source:agent-q-branch-observer", "pattern:" + c.Pattern}
	if hasMetric {
		tags = append(tags, "anomaly_type:metric")
	}
	if hasLog {
		tags = append(tags, "anomaly_type:log")
	}
	for t := range dimensionSet {
		tags = append(tags, t)
	}
	sort.Strings(tags[2:])
	return tags
}
