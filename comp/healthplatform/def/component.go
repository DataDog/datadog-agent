// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

// Package healthplatform provides the interface for the health platform component.
// This component collects and reports health information from the host system,
// sending it to the Datadog backend with hostname, host ID, organization ID,
// and a list of issues.
package healthplatform

// team: agent-health

import (
	"time"

	healthplatformpayload "github.com/DataDog/agent-payload/v5/healthplatform"
)

// HealthCheckFunc is a function that checks for health issues
// Returns an IssueReport if an issue is detected, nil if healthy
// The function should be idempotent and safe to call repeatedly
type HealthCheckFunc func() (*healthplatformpayload.IssueReport, error)

// Component is the health platform component interface
type Component interface {
	// ReportIssue reports an issue with context, and the health platform fills in remediation
	// This is the main way for integrations to report issues
	// If report is nil, it clears any existing issue (issue resolution)
	ReportIssue(checkID string, checkName string, report *healthplatformpayload.IssueReport) error

	// RegisterCheck registers a function to be called periodically to check for issues
	// Use this when you need the health platform to run your check at regular intervals
	// If interval is 0 or negative, defaults to 15 minutes
	RegisterCheck(checkID string, checkName string, checkFn HealthCheckFunc, interval time.Duration) error

	// =========================================================================
	// Query Methods
	// =========================================================================

	// GetAllIssues returns the count and all issues from all checks (indexed by check ID)
	// Returns the total number of issues and a map of issues (nil for checks with no issues)
	GetAllIssues() (int, map[string]*healthplatformpayload.Issue)

	// GetIssueForCheck returns the issue for a specific check
	// Returns nil if no issue
	GetIssueForCheck(checkID string) *healthplatformpayload.Issue

	// =========================================================================
	// Clear Methods
	// =========================================================================

	// ClearIssuesForCheck clears issues for a specific check (useful when issues are resolved)
	ClearIssuesForCheck(checkID string)

	// ClearAllIssues clears all issues (useful for testing or when all issues are resolved)
	ClearAllIssues()
}
