// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

// Package scheduler defines the interface for the health platform scheduler
// (the periodic runner of built-in health checks).
package scheduler

import (
	"time"

	healthplatformpayload "github.com/DataDog/agent-payload/v5/healthplatform"
	storedef "github.com/DataDog/datadog-agent/comp/healthplatform/store/def"
)

// team: agent-health

// HealthCheckFunc is a function that performs a health check.
// It returns the proto IssueReport from the agent-payload registry; the
// scheduler converts it to a storedef.IssueReport before forwarding to the
// IssueReporter. Returning (nil, nil) means no issue was detected.
type HealthCheckFunc func() (*healthplatformpayload.IssueReport, error)

// IssueReporter receives health-check results from the scheduler.
type IssueReporter interface {
	ReportIssue(report storedef.IssueReport) error
	ResolveIssue(issueID string)
}

// Component is the health-platform scheduler component.
type Component interface {
	// SetReporter wires the issue reporter after construction, breaking the
	// circular fx dependency between store and scheduler.
	// Must be called before the first health check fires (i.e. from the store
	// lifecycle start hook).
	SetReporter(reporter IssueReporter)

	// ScheduleHealthCheck registers a periodic health check that runs at the
	// given interval. The check is identified by checkID (must be unique) and
	// checkName (human-readable label). If interval is zero or negative, the
	// scheduler's default interval is used.
	ScheduleHealthCheck(checkID string, checkName string, fn HealthCheckFunc, interval time.Duration) error

	// RunHealthCheck executes a health check immediately, outside the periodic
	// schedule. Results are reported to the registered IssueReporter.
	RunHealthCheck(checkID string, checkName string, fn HealthCheckFunc) error
}
