// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

// Package runner defines the interface for the health platform runner component.
// The runner executes a single HealthCheckFunc once, forwards each emitted
// IssueReport to the store, and returns the set of IssueIds that were reported.
package runner

import (
	storedef "github.com/DataDog/datadog-agent/comp/healthplatform/store/def"
)

// team: agent-health

// HealthCheckFunc is a function that performs a health check and returns zero
// or more issue reports. Returning (nil, nil) or (empty slice, nil) means no
// issue was detected. The function must return new IssueReport values on each
// call; the runner modifies the Source field of each report.
type HealthCheckFunc func() ([]storedef.IssueReport, error)

// Component is the health platform runner component.
type Component interface {
	// Run executes fn once. Each emitted IssueReport is forwarded to the store
	// via ReportIssue. If a report's Source field is empty, it is filled with
	// the source argument. Returns the slice of IssueIds that were successfully
	// reported to the store, so callers (the scheduler) can diff across runs.
	//
	// When a non-nil error is returned the IDs may be incomplete; callers must
	// not use them for issue-state diffs.
	Run(source string, fn HealthCheckFunc) ([]string, error)
}
