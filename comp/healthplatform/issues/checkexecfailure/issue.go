// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

// Package checkexecfailure provides the issue type for check-execution
// failures. It is a Path-B issue type: errors are detected and reported
// directly by the check wrapper (comp/collector/collector/impl/internal/middleware/check_wrapper.go),
// which runs the check on every scheduled interval.
package checkexecfailure

import (
	"fmt"

	"github.com/DataDog/agent-payload/v5/healthplatform"
	"google.golang.org/protobuf/types/known/structpb"
)

const (
	// IssueName is the human-readable issue name for check-execution failure issues.
	IssueName = "Check Execution Failure"
	// IssueType is the snake_case type key for check-execution failure issues:
	// IssueName lowercased with spaces replaced by underscores.
	IssueType = "check_execution_failure"
	// IssueID is the IssueID prefix for check-execution failure issues. Reporters
	// append a discriminator + check-id digest: IssueID + ":" + fnv(discriminator + checkID)
	IssueID = "check-run-failure"
	// Source is the reporting component identifier used in health-platform issues.
	Source = "collector"

	contextKeyCheckName   = "check_name"
	contextKeyErrors      = "errors"
	contextKeyImpact      = "impact"
	contextKeyOccurrences = "consecutive_failures"

	category  = "configuration"
	location  = "agent"
	severity  = healthplatform.IssueSeverity_ISSUE_SEVERITY_HIGH
	impactMsg = "This check's metrics, logs, or events are not being collected."
)

// CheckExecFailureIssue provides the issue template for check-execution failure issues.
type CheckExecFailureIssue struct{}

// NewCheckExecFailureIssue creates a new check-execution failure issue template.
func NewCheckExecFailureIssue() *CheckExecFailureIssue {
	return &CheckExecFailureIssue{}
}

// BuildIssue creates a complete issue with metadata and remediation for a
// check-execution failure. context["check_name"] is the integration name,
// context["errors"] the last run error message, and
// context["consecutive_failures"] the number of consecutive failed runs.
func (t *CheckExecFailureIssue) BuildIssue(context map[string]string) (*healthplatform.Issue, error) {
	checkName := context[contextKeyCheckName]
	if checkName == "" {
		checkName = "unknown"
	}
	errMsg := context[contextKeyErrors]
	occurrences := context[contextKeyOccurrences]

	extra, err := structpb.NewStruct(map[string]any{
		contextKeyCheckName:   checkName,
		contextKeyErrors:      errMsg,
		contextKeyOccurrences: occurrences,
		contextKeyImpact:      impactMsg,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to create extra: %v", err)
	}

	return &healthplatform.Issue{
		IssueName:   IssueName,
		IssueType:   IssueType,
		Title:       fmt.Sprintf("Check '%s' Is Failing to Run", checkName),
		Description: fmt.Sprintf("Check '%s' has failed %s consecutive runs: %s", checkName, occurrences, errMsg),
		Category:    category,
		Location:    location,
		Severity:    severity,
		Source:      Source,
		Extra:       extra,
		Tags:        []string{"collector", "check_execution"},
		Remediation: &healthplatform.Remediation{
			Summary: fmt.Sprintf("Fix the error preventing '%s' from running successfully.", checkName),
			Steps: []*healthplatform.RemediationStep{
				{Order: 1, Text: "Review the error message above for the specific run failure."},
				{Order: 2, Text: fmt.Sprintf("Run `datadog-agent check %s` for a detailed trace of the failure.", checkName)},
				{Order: 3, Text: "Fix the underlying issue (credentials, connectivity, target availability), then wait for the check to recover."},
			},
		},
	}, nil
}
