// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

// Package healthplatform ... /* TODO: detailed doc comment for the component */
package healthplatform

import (
	"context"
)

// team: agent-health

// Issue represents an individual issue to be reported
type Issue struct {
	// ID is the unique identifier for the issue
	ID string `json:"ID"`
	// IssueName is the human-readable name for the issue
	IssueName string `json:"IssueName"`
	// Title is the short title/headline of the issue
	Title string `json:"Title"`
	// Description is the detailed description of the issue
	Description string `json:"Description"`
	// Category indicates the type/category of the issue (e.g., permissions, connectivity, etc.)
	Category string `json:"Category"`
	// Location indicates where the issue occurred (e.g., core agent, log agent, etc.)
	Location string `json:"Location"`
	// Severity indicates the impact level of the issue
	Severity string `json:"Severity"`
	// DetectedAt is the timestamp when the issue was detected
	DetectedAt string `json:"DetectedAt"`
	// Integration indicates which integration or feature is affected
	Integration *string `json:"Integration,omitempty"`
	// Extra is optional complementary information
	Extra string `json:"Extra"`
	// IntegrationFeature indicates which integration or feature is affected (legacy field)
	IntegrationFeature string `json:"IntegrationFeature"`
	// Remediation provides steps to fix the issue
	Remediation *Remediation `json:"Remediation,omitempty"`
	// Tags are additional labels for the issue
	Tags []string `json:"Tags"`
}

// Remediation represents remediation steps for an issue
type Remediation struct {
	// Summary is a brief description of the remediation
	Summary string `json:"Summary"`
	// Steps are the ordered steps to fix the issue
	Steps []RemediationStep `json:"Steps"`
	// Script is an automated script to fix the issue
	Script *Script `json:"Script,omitempty"`
}

// RemediationStep represents a single remediation step
type RemediationStep struct {
	// Order is the sequence number of the step
	Order int `json:"Order"`
	// Text is the description of what to do
	Text string `json:"Text"`
}

// Script represents a remediation script
type Script struct {
	// Language is the scripting language (e.g., bash, powershell)
	Language string `json:"Language"`
	// Filename is the suggested filename for the script
	Filename string `json:"Filename"`
	// RequiresRoot indicates if the script needs root privileges
	RequiresRoot bool `json:"RequiresRoot"`
	// Content is the actual script content
	Content string `json:"Content"`
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

// JSONAPIResponse represents the JSON:API wrapper for health reports
type JSONAPIResponse struct {
	Data *HealthReport `json:"data"`
	Meta *JSONAPIMeta  `json:"meta,omitempty"`
}

// JSONAPIMeta represents metadata in JSON:API format
type JSONAPIMeta struct {
	SchemaVersion string `json:"schema_version,omitempty"`
	EventType     string `json:"event_type,omitempty"`
	EmittedAt     string `json:"emitted_at,omitempty"`
}

// CheckConfig is a configuration for a health check
type CheckConfig struct {
	CheckName string
	CheckID   string

	Callback func() ([]Issue, error)
}

// HealthRecommendation is an interface for a health recommendation
type HealthRecommendation interface {
	RegisterCheck(check CheckConfig) error
}

// Component is the health platform component interface
type Component interface {
	/* ================================
		Scheduler Functions:
	=============================== */

	// Run runs the health checks and reports the issues
	Run(ctx context.Context) (*HealthReport, error)
}
