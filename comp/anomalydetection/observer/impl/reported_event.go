// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package observerimpl

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
