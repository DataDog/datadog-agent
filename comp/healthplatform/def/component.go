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
	// Source is the sub-agent or product that reported the issue
	// (e.g., "logs", "apm", "error-tracking", "network-monitoring")
	Source string `json:"Source"`
	// Extra is optional complementary structured information
	Extra map[any]any `json:"Extra,omitempty"`
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
	// Language is the scripting language (e.g., bash, powershell, python, javascript)
	Language string `json:"Language"`
	// LanguageVersion is the required interpreter version (e.g., "3.8+" for Python, ">=14" for Node.js)
	LanguageVersion string `json:"LanguageVersion,omitempty"`
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

// IssueReport represents a lightweight issue report from an integration
// The health platform fills in all metadata and remediation based on the issue ID
type IssueReport struct {
	// IssueID is the unique identifier for the type of issue
	// The health platform registry uses this to look up all issue details
	IssueID string

	// Context provides variables for filling in templates
	// (e.g., {"dockerDir": "/var/lib/docker", "os": "linux"})
	Context map[string]string

	// Tags are optional additional labels for filtering and categorization
	// These are appended to the default tags from the registry
	Tags []string
}

// Component is the health platform component interface
type Component interface {
	// ReportIssue reports an issue with context, and the health platform fills in remediation
	// This is the main way for integrations to report issues
	// If report is nil, it clears any existing issue (issue resolution)
	ReportIssue(checkID string, checkName string, report *IssueReport) error

	// GetAllIssues returns the count and all issues from all checks (indexed by check ID)
	// Returns the total number of issues and a map of issues (nil for checks with no issues)
	GetAllIssues() (int, map[string]*Issue)

	// GetIssueForCheck returns the issue for a specific check
	// Returns nil if no issue
	GetIssueForCheck(checkID string) *Issue

	// ClearIssuesForCheck clears issues for a specific check (useful when issues are resolved)
	ClearIssuesForCheck(checkID string)

	// ClearAllIssues clears all issues (useful for testing or when all issues are resolved)
	ClearAllIssues()
}
