// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

// Package checkrunner defines the interface for the health platform check runner.
package checkrunner

import (
	"time"

	healthplatformpayload "github.com/DataDog/agent-payload/v5/healthplatform"
)

// team: agent-health

// HealthCheckFunc is a function that performs a health check.
type HealthCheckFunc func() (*healthplatformpayload.IssueReport, error)

// IssueReporter receives check results from the check runner.
type IssueReporter interface {
	ReportIssue(checkID string, checkName string, report *healthplatformpayload.IssueReport) error
}

// Component is the check runner component.
type Component interface {
	// SetReporter wires the issue reporter after construction, breaking the
	// circular fx dependency between core and checkrunner.
	// Must be called before the first check fires (i.e. from the core lifecycle start hook).
	SetReporter(reporter IssueReporter)

	// RegisterCheck registers a periodic health check that runs at the given interval.
	// The check is identified by checkID (must be unique) and checkName (human-readable label).
	// If interval is zero or negative, a default interval is used.
	RegisterCheck(checkID string, checkName string, fn HealthCheckFunc, interval time.Duration) error

	// RunCheck executes a health check immediately, outside the periodic schedule.
	// Results are reported to the registered IssueReporter.
	RunCheck(checkID string, checkName string, fn HealthCheckFunc) error
}
