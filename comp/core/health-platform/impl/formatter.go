// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

// Package healthplatformimpl provides the implementation for the health platform component.
package healthplatformimpl

import (
	healthplatform "github.com/DataDog/datadog-agent/comp/core/health-platform/def"
)

const (
	// SchemaVersion is the version of the health report schema
	SchemaVersion = "1.0"

	// EventType is the type of health event
	EventType = "agent_health"
)

// Severity indicates the impact level of an issue
type Severity string

const (
	// SeverityLow indicates a minor issue with minimal impact
	SeverityLow Severity = "low"
	// SeverityMedium indicates a moderate issue that may affect performance
	SeverityMedium Severity = "medium"
	// SeverityHigh indicates a significant issue that may cause failures
	SeverityHigh Severity = "high"
	// SeverityCritical indicates a critical issue that will cause failures
	SeverityCritical Severity = "critical"
)

// formatHealthReport formats the collected issues into a structured health report
func formatHealthReport(issues []healthplatform.Issue, hostInfo healthplatform.HostInfo) healthplatform.HealthReport {
	formattedIssues := make([]healthplatform.Issue, len(issues))

	for i, issue := range issues {
		// Set default values for new fields
		issueName := issue.ID
		if issue.IssueName != "" {
			issueName = issue.IssueName
		}

		title := issue.Description
		if issue.Title != "" {
			title = issue.Title
		}

		category := "general"
		if issue.Category != "" {
			category = issue.Category
		}

		detectedAt := ""
		if issue.DetectedAt != "" {
			detectedAt = issue.DetectedAt
		}

		integration := issue.Integration
		if integration == nil && issue.IntegrationFeature != "" {
			integration = &issue.IntegrationFeature
		}

		remediation := issue.Remediation
		if remediation == nil {
			// Provide default remediation for common issue types
			remediation = getDefaultRemediation(issue)
		}

		tags := issue.Tags
		if len(tags) == 0 {
			// Provide default tags based on location and category
			tags = getDefaultTags(issue)
		}

		formattedIssues[i] = healthplatform.Issue{
			ID:                 issue.ID,
			IssueName:          issueName,
			Title:              title,
			Description:        issue.Description,
			Category:           category,
			Location:           issue.Location,
			Severity:           issue.Severity,
			DetectedAt:         detectedAt,
			Integration:        integration,
			Extra:              issue.Extra,
			IntegrationFeature: issue.IntegrationFeature,
			Remediation:        remediation,
			Tags:               tags,
		}
	}

	return healthplatform.HealthReport{
		SchemaVersion: SchemaVersion,
		EventType:     EventType,
		EmittedAt:     "", // Will be filled later
		Host:          hostInfo,
		Issues:        formattedIssues,
	}
}

// getDefaultRemediation provides default remediation for common issue types
func getDefaultRemediation(issue healthplatform.Issue) *healthplatform.Remediation {
	switch issue.Category {
	case "permissions":
		return &healthplatform.Remediation{
			Summary: "Check and fix permission issues for the affected component.",
			Steps: []healthplatform.RemediationStep{
				{Order: 1, Text: "Verify file/directory permissions"},
				{Order: 2, Text: "Check user/group membership"},
				{Order: 3, Text: "Restart the affected service"},
			},
		}
	case "connectivity":
		return &healthplatform.Remediation{
			Summary: "Check network connectivity and configuration.",
			Steps: []healthplatform.RemediationStep{
				{Order: 1, Text: "Verify network configuration"},
				{Order: 2, Text: "Check firewall settings"},
				{Order: 3, Text: "Test connectivity to required endpoints"},
			},
		}
	default:
		return &healthplatform.Remediation{
			Summary: "Review the issue and take appropriate action.",
			Steps: []healthplatform.RemediationStep{
				{Order: 1, Text: "Review the issue description"},
				{Order: 2, Text: "Check relevant logs"},
				{Order: 3, Text: "Apply recommended fixes"},
			},
		}
	}
}

// getDefaultTags provides default tags based on issue properties
func getDefaultTags(issue healthplatform.Issue) []string {
	tags := []string{issue.Location}

	if issue.Category != "" {
		tags = append(tags, issue.Category)
	}

	if issue.IntegrationFeature != "" {
		tags = append(tags, issue.IntegrationFeature)
	}

	// Add severity-based tags
	switch issue.Severity {
	case "critical", "high":
		tags = append(tags, "urgent")
	case "medium":
		tags = append(tags, "attention")
	case "low":
		tags = append(tags, "info")
	}

	return tags
}
