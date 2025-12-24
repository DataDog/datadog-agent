// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

// Package checkfailure provides remediation for check execution failures.
package checkfailure

import (
	healthplatform "github.com/DataDog/datadog-agent/comp/healthplatform/def"
)

const (
	issueID    = "check-execution-failure"
	issueName  = "check_execution_failure"
	category   = "check-execution"
	location   = "collector"
	severity   = "medium"
	source     = "collector"
	unknownVal = "unknown"
	failedMsg  = "Check execution failed"
	impactMsg  = "Metrics, events, or service checks from this integration may not be collected"
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
		checkName = unknownVal
	}

	errorMessage := context["errorMessage"]
	if errorMessage == "" {
		errorMessage = failedMsg
	}

	totalErrors := context["totalErrors"]
	if totalErrors == "" {
		totalErrors = unknownVal
	}

	configSource := context["configSource"]
	if configSource == "" {
		configSource = unknownVal
	}

	checkVersion := context["checkVersion"]

	// Build description efficiently
	var desc []byte
	desc = append(desc, "Check '"...)
	desc = append(desc, checkName...)
	desc = append(desc, "' error: "...)
	desc = append(desc, errorMessage...)
	if totalErrors != unknownVal && totalErrors != "1" {
		desc = append(desc, " (errors: "...)
		desc = append(desc, totalErrors...)
		desc = append(desc, ')')
	}

	// Build title efficiently
	var title []byte
	title = append(title, "Check '"...)
	title = append(title, checkName...)
	title = append(title, "' Failed"...)

	// Build remediation steps
	steps := make([]healthplatform.RemediationStep, 0, 7)
	steps = append(steps,
		healthplatform.RemediationStep{Order: 1, Text: "Check logs: 'datadog-agent status' or 'tail -f /var/log/datadog/agent.log'"},
		healthplatform.RemediationStep{Order: 2, Text: "Review config at: " + configSource},
		healthplatform.RemediationStep{Order: 3, Text: "Verify permissions and dependencies"},
		healthplatform.RemediationStep{Order: 4, Text: "Verify monitored service is accessible"},
		healthplatform.RemediationStep{Order: 5, Text: "See docs: https://docs.datadoghq.com/integrations/"},
	)

	if checkVersion != "" {
		var verStep []byte
		verStep = append(verStep, "Check known issues for version "...)
		verStep = append(verStep, checkVersion...)
		steps = append(steps, healthplatform.RemediationStep{
			Order: len(steps) + 1,
			Text:  string(verStep),
		})
	}

	steps = append(steps, healthplatform.RemediationStep{
		Order: len(steps) + 1,
		Text:  "Enable debug: set 'log_level: debug' in datadog.yaml and restart",
	})

	return &healthplatform.Issue{
		ID:          issueID,
		IssueName:   issueName,
		Title:       string(title),
		Description: string(desc),
		Category:    category,
		Location:    location,
		Severity:    severity,
		DetectedAt:  "",
		Source:      source,
		Extra: map[any]any{
			"check_name":    checkName,
			"error_message": errorMessage,
			"total_errors":  totalErrors,
			"config_source": configSource,
			"check_version": checkVersion,
			"impact":        impactMsg,
		},
		Remediation: &healthplatform.Remediation{
			Summary: "Review config and logs to diagnose",
			Steps:   steps,
		},
		Tags: []string{"check-failure", "collector", checkName},
	}
}
