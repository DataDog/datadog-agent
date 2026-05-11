// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

// Package store provides the interface for the health platform store component.
// The store collects and reports health information from the host system,
// sending it to the Datadog backend with hostname, host ID, organization ID,
// and a list of issues.
package store

// team: agent-health

import (
	"time"

	healthplatformpayload "github.com/DataDog/agent-payload/v5/healthplatform"
	checkrunnerdef "github.com/DataDog/datadog-agent/comp/healthplatform/scheduler/def"
)

// HealthCheckFunc is an alias of checkrunnerdef.HealthCheckFunc to avoid callers
// having to import checkrunner/def directly.
type HealthCheckFunc = checkrunnerdef.HealthCheckFunc

// Component is the health platform store component interface.
type Component interface {
	// ReportIssue reports an issue with context; the health platform fills in
	// remediation from the issue template registry. It is the main way for
	// integrations to report issues. If report is nil, any existing issue for
	// the given checkID is resolved.
	ReportIssue(checkID string, checkName string, report *healthplatformpayload.IssueReport) error

	// ScheduleHealthCheck schedules a function to be called periodically to
	// check for issues. If interval is 0 or negative, the runner's default
	// interval is used.
	ScheduleHealthCheck(checkID string, checkName string, checkFn HealthCheckFunc, interval time.Duration) error

	// =========================================================================
	// Query Methods
	// =========================================================================

	// GetAllIssues returns the count and all active issues, indexed by checkID.
	// The returned map contains deep copies; modifying it does not affect the store.
	GetAllIssues() (int, map[string]*healthplatformpayload.Issue)

	// GetIssue returns the issue reported for the given checkID, or nil if
	// no such issue is currently active.
	GetIssue(checkID string) *healthplatformpayload.Issue

	// =========================================================================
	// Resolve Methods
	// =========================================================================

	// ResolveIssue marks the issue for the given checkID as resolved.
	// No-op if no issue is currently active for that checkID.
	ResolveIssue(checkID string)

	// ResolveAllIssues marks every active issue as resolved.
	ResolveAllIssues()
}
