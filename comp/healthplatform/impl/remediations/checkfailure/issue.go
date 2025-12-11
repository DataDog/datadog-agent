// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

// Package checkfailure provides remediation for check execution failures.
package checkfailure

import (
	"fmt"

	healthplatform "github.com/DataDog/datadog-agent/comp/healthplatform/def"
)

// CheckFailureIssue provides complete issue template for check failures
type CheckFailureIssue struct{}

// NewCheckFailureIssue creates a new check failure issue template
func NewCheckFailureIssue() *CheckFailureIssue {
	return &CheckFailureIssue{}
}

// BuildIssue creates a complete issue with metadata and remediation for check failures
func (t *CheckFailureIssue) BuildIssue(context map[string]string) *healthplatform.Issue {
	checkName := context["checkName"]
	if checkName == "" {
		checkName = "unknown"
	}

	errorMessage := context["errorMessage"]
	if errorMessage == "" {
		errorMessage = "Check execution failed"
	}

	totalErrors := context["totalErrors"]
	if totalErrors == "" {
		totalErrors = "unknown"
	}

	configSource := context["configSource"]
	if configSource == "" {
		configSource = "unknown"
	}

	checkVersion := context["checkVersion"]

	// Build description
	description := fmt.Sprintf("The check '%s' has encountered an error during execution: %s", checkName, errorMessage)
	if totalErrors != "unknown" && totalErrors != "1" {
		description += fmt.Sprintf(" (Total errors: %s)", totalErrors)
	}

	// Build title
	title := fmt.Sprintf("Check '%s' Failed", checkName)

	// Build remediation steps
	remediationSteps := []healthplatform.RemediationStep{
		{Order: 1, Text: "Check the agent logs for more detailed error information: 'datadog-agent status' or 'tail -f /var/log/datadog/agent.log'"},
		{Order: 2, Text: fmt.Sprintf("Review the check configuration at: %s", configSource)},
		{Order: 3, Text: "Verify that all required permissions and dependencies are in place for this check"},
		{Order: 4, Text: "Check if the monitored service/resource is accessible and running correctly"},
		{Order: 5, Text: "Consult the integration documentation: https://docs.datadoghq.com/integrations/"},
	}

	// Add version-specific step if version is available
	if checkVersion != "" {
		remediationSteps = append(remediationSteps, healthplatform.RemediationStep{
			Order: 6,
			Text:  fmt.Sprintf("Check if there are known issues with version %s of the integration", checkVersion),
		})
	}

	remediationSteps = append(remediationSteps, healthplatform.RemediationStep{
		Order: 7,
		Text:  "If the issue persists, enable debug logging: set 'log_level: debug' in datadog.yaml and restart the agent",
	})

	return &healthplatform.Issue{
		ID:          "check-execution-failure",
		IssueName:   "check_execution_failure",
		Title:       title,
		Description: description,
		Category:    "check-execution",
		Location:    "collector",
		Severity:    "medium",
		DetectedAt:  "", // Will be filled by health platform
		Source:      "collector",
		Extra: map[any]any{
			"check_name":    checkName,
			"error_message": errorMessage,
			"total_errors":  totalErrors,
			"config_source": configSource,
			"check_version": checkVersion,
			"impact":        "Metrics, events, or service checks from this integration may not be collected",
		},
		Remediation: &healthplatform.Remediation{
			Summary: "Review check configuration and logs to diagnose the failure",
			Steps:   remediationSteps,
		},
		Tags: []string{"check-failure", "collector", checkName},
	}
}
