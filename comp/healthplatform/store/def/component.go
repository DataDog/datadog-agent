// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

// Package store provides the interface for the health platform store component.
// The store is the central state owner: it receives issue reports, owns the
// in-memory issue map, persists state to disk, and exposes the local
// /health-platform/issues HTTP endpoint.
package store

// team: agent-health

import (
	healthplatformpayload "github.com/DataDog/agent-payload/v5/healthplatform"
)

// Component is the health platform store component interface.
type Component interface {
	// ReportIssue records a new or ongoing issue. Two calls with the same
	// report.IssueID update the same instance (state machine: new → ongoing).
	// Call ResolveIssue to mark an issue as resolved.
	ReportIssue(report IssueReport) error

	// =========================================================================
	// Query Methods
	// =========================================================================

	// GetAllIssues returns the count and all active issues, indexed by IssueId.
	// The returned map contains deep copies; modifying it does not affect the store.
	GetAllIssues() (int, map[string]*healthplatformpayload.Issue)

	// GetIssue returns the active issue with the given IssueId, or nil if none.
	GetIssue(issueID string) *healthplatformpayload.Issue

	// =========================================================================
	// Resolve Methods
	// =========================================================================

	// ResolveIssue marks the issue with the given IssueId as resolved.
	// No-op if no such issue is currently active.
	ResolveIssue(issueID string)

	// ResolveAllIssues marks every active issue as resolved.
	ResolveAllIssues()
}
