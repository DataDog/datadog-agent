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

// replayReporter mirrors EventReporter.Report() during testbench replay.
// It tracks which patterns are currently active and appends a new ReportedEvent
// each time a pattern first appears or reappears after going inactive — exactly
// matching the live EventReporter semantics, including repeated incidents for
// stable pattern names.
type replayReporter struct {
	seenPatterns map[string]bool
	events       []ReportedEvent
	storage      observerdef.StorageReader
}

// Name satisfies observerdef.Reporter.
func (r *replayReporter) Name() string { return "replay_reporter" }

// Report fires a new ReportedEvent for each pattern that is newly active (first
// appearance or reappearance after being absent in the previous call).
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
			msg := buildChangeMessage(ac, r.storage)
			tags := []string{"source:agent-q-branch-observer", "pattern:" + ac.Pattern}
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

	// Drop inactive patterns so they can fire again if they reappear.
	for pattern := range r.seenPatterns {
		if !currentlyActive[pattern] {
			delete(r.seenPatterns, pattern)
		}
	}
}
