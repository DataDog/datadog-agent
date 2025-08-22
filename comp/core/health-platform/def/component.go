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
	// IssueName is the human-readable name for the issue
	IssueName string
	// Title is the short title/headline of the issue
	Title string
	// Description is the detailed description of the issue
	Description string
	// Category indicates the type/category of the issue (e.g., permissions, connectivity, etc.)
	Category string
	// Location indicates where the issue occurred (e.g., core agent, log agent, etc.)
	Location string
	// Severity indicates the impact level of the issue
	Severity string
	// DetectedAt is the timestamp when the issue was detected
	DetectedAt string
	// Integration indicates which integration or feature is affected
	Integration *string
	// Extra is optional complementary information
	Extra string
	// IntegrationFeature indicates which integration or feature is affected (legacy field)
	IntegrationFeature string
	// Remediation provides steps to fix the issue
	Remediation *Remediation
	// Tags are additional labels for the issue
	Tags []string
}

// Remediation represents remediation steps for an issue
type Remediation struct {
	// Summary is a brief description of the remediation
	Summary string
	// Steps are the ordered steps to fix the issue
	Steps []RemediationStep
	// Script is an automated script to fix the issue
	Script *Script
}

// RemediationStep represents a single remediation step
type RemediationStep struct {
	// Order is the sequence number of the step
	Order int
	// Text is the description of what to do
	Text string
}

// Script represents a remediation script
type Script struct {
	// Language is the scripting language (e.g., bash, powershell)
	Language string
	// Filename is the suggested filename for the script
	Filename string
	// RequiresRoot indicates if the script needs root privileges
	RequiresRoot bool
	// Content is the actual script content
	Content string
}

// SubComponent represents a health checker sub-component
type SubComponent interface {
	// CheckHealth performs health checks and returns any issues found
	CheckHealth(ctx context.Context) ([]Issue, error)
}

// HealthReport represents the formatted health report structure
type HealthReport struct {
	SchemaVersion string   `json:"schema_version"`
	EventType     string   `json:"event_type"`
	EmittedAt     string   `json:"emitted_at"`
	Host          HostInfo `json:"host"`
	Issues        []Issue  `json:"issues"`
}

// HostInfo represents the host information in the health report
type HostInfo struct {
	Hostname     string   `json:"hostname"`
	AgentVersion string   `json:"agent_version"`
	ParIDs       []string `json:"par_ids"`
}

// Component is the health platform component interface
type Component interface {
	/* ================================
		Scheduler Functions:
	=============================== */

	// Start begins the periodic reporting of issues
	Start(ctx context.Context) error

	// Stop stops the periodic reporting
	Stop() error

	// Run runs the health checks and reports the issues
	Run(ctx context.Context) (*HealthReport, error)

	/* ================================
		Issue Management Functions:
	=============================== */

	// FlushIssues flushes the current issues to the backend
	FlushIssues() error

	/* ================================
		Sub-Component Management Functions:
	=============================== */

	// RegisterSubComponent registers a health checker sub-component
	RegisterSubComponent(sub SubComponent) error
}
