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

// EgressCallbacks groups the functions the egress registers with the store.
// The store calls these on state transitions; either field may be nil (no-op).
type EgressCallbacks struct {
	// OnReportIssue is called after a new or updated issue is stored.
	// The issue is a fully-hydrated clone safe for the egress to retain.
	OnReportIssue func(issue *healthplatformpayload.Issue)

	// OnResolveIssue is called when an issue transitions to RESOLVED.
	// The tombstone is a minimal proto (ID, IssueName, PersistedIssue only).
	OnResolveIssue func(tombstone *healthplatformpayload.Issue)
}

// Component is the health platform store component interface.
type Component interface {
	// SetEgressCallbacks registers the egress callbacks invoked on state changes.
	// Must be called before the component's OnStart lifecycle hook fires (i.e.
	// from the egress constructor, before App.Start).
	SetEgressCallbacks(cbs EgressCallbacks)

	// ReportIssue records a new or ongoing issue keyed by issue.Id. Two calls
	// with the same issue.Id update the same instance (state machine: new →
	// ongoing). issue.IssueName is used as the issue-type key for telemetry
	// and persistence. Call ResolveIssue to mark an issue as resolved.
	ReportIssue(issue *healthplatformpayload.Issue) error

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

	// GetActiveIssueIDsByIssueName returns the IDs of all currently active issues
	// with the given IssueName (e.g. "docker_file_tailing_disabled").
	GetActiveIssueIDsByIssueName(issueName string) []string
}
