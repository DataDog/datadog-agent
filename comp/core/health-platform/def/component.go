// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

// Package healthplatform provides the interface for the health platform component.
// This component collects and reports health information from the host system,
// sending it to the Datadog backend with hostname, host ID, organization ID,
// and a list of issues.
package healthplatform

import (
	"context"
)

// team: agent-runtimes

// Issue represents an individual issue to be reported
type Issue struct {
	// ID is the unique identifier for the issue
	ID string
	// Name is the human-readable name of the issue
	Name string
	// Extra is optional complementary information
	Extra string
}

// Component is the health platform component interface
type Component interface {
	// AddIssue adds a new issue to be reported
	AddIssue(issue Issue) error

	// RemoveIssue removes an issue by ID
	RemoveIssue(id string) error

	// ListIssues returns all currently tracked issues
	ListIssues() []Issue

	// SubmitReport immediately submits the current issues to the backend
	SubmitReport(ctx context.Context) error

	// Start begins the periodic reporting of issues
	Start(ctx context.Context) error

	// Stop stops the periodic reporting
	Stop() error
}
