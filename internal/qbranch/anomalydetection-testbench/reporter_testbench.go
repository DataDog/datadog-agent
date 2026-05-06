// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package main

import (
	"time"

	observerdef "github.com/DataDog/datadog-agent/comp/anomalydetection/observer/def"
	observerimpl "github.com/DataDog/datadog-agent/comp/anomalydetection/observer/impl"
)

// replayReporter mirrors EventReporter.Report() during testbench replay.
// It tracks which patterns are currently active and appends a new ReportedEvent
// each time a pattern first appears or reappears after going inactive — exactly
// matching the live EventReporter semantics, including repeated incidents for
// stable pattern names.
type replayReporter struct {
	seenPatterns map[string]bool
	events       []observerimpl.ReportedEvent
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
			msg := observerimpl.BuildChangeMessage(ac, r.storage)
			tags := observerimpl.BuildEventTags(ac)
			r.events = append(r.events, observerimpl.ReportedEvent{
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
