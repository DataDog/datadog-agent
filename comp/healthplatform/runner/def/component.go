// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

// Package runner defines the interface for the health platform runner component.
// The runner executes a single HealthCheckFunc once, translates each emitted
// IssueReport into a proto Issue (via the issue registry), forwards it to the
// store, and returns the set of IssueIds that were reported.
package runner

// team: fleet-remediation

// IssueReport is the lightweight value that HealthCheckFunc implementations
// return. The runner translates each IssueReport into a proto Issue using the
// issue registry before forwarding to the store.
type IssueReport struct {
	// IssueID is the unique instance id, used as the store's map key.
	// Examples:
	//   "rofs-permissions:mysql:0123abcd"
	//   "ad-template:redis:svc-foo:deadbeef"
	IssueID string

	// IssueName is the issue name looked up in the issue registry.
	// Examples: "Read-Only Filesystem Error", "Docker File Tailing Disabled"
	IssueName string

	// Source is the reporting integration or component name.
	// If empty, the runner fills it from the source argument to Run.
	// Examples: "mysql", "docker", "agent"
	Source string

	// Context provides variables for filling in the issue template.
	Context map[string]string

	// Tags are appended to the template's default tags.
	Tags []string
}

// HealthCheckFunc is a function that performs a health check and returns zero
// or more issue reports. Returning (nil, nil) or (empty slice, nil) means no
// issue was detected. The function must return new IssueReport values on each
// call; the runner fills Source if empty.
type HealthCheckFunc func() ([]IssueReport, error)

// Component is the health platform runner component.
type Component interface {
	// Run executes fn once. Each emitted IssueReport is translated to a proto
	// Issue via the registry and forwarded to the store via ReportIssue. If a
	// report's Source field is empty, it is filled with the source argument.
	// Returns the slice of IssueIds that were successfully reported to the
	// store, so callers (the scheduler) can diff across runs.
	//
	// When a non-nil error is returned the IDs may be incomplete; callers must
	// not use them for issue-state diffs.
	Run(source string, fn HealthCheckFunc) ([]string, error)
}
