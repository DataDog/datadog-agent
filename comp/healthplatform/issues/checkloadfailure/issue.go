// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

// Package checkloadfailure provides the issue type for check-load failures.
// It is a Path-B issue type: errors are detected and reported directly by the
// collector's check scheduler (pkg/collector/scheduler.go), which loads check
// configurations into runnable checks.
package checkloadfailure

import (
	"fmt"

	"github.com/DataDog/agent-payload/v5/healthplatform"
	"google.golang.org/protobuf/types/known/structpb"
)

const (
	// IssueName is the human-readable issue name for check-load failure issues.
	IssueName = "Check Load Failure"
	// IssueType is the snake_case type key for check-load failure issues:
	// IssueName lowercased with spaces replaced by underscores.
	IssueType = "check_load_failure"
	// IssueID is the IssueID prefix for check-load failure issues. Reporters
	// append a discriminator + check-name digest: IssueID + ":" + fnv(discriminator + checkName)
	IssueID = "check-load-failure"
	// Source is the reporting component identifier used in health-platform issues.
	Source = "collector"

	contextKeyCheckName = "check_name"
	contextKeyErrors    = "errors"
	contextKeyImpact    = "impact"

	category  = "configuration"
	location  = "agent"
	severity  = healthplatform.IssueSeverity_ISSUE_SEVERITY_HIGH
	impactMsg = "This check's metrics, logs, or events are not being collected."
)

// CheckLoadFailureIssue provides the issue template for check-load failure issues.
type CheckLoadFailureIssue struct{}

// NewCheckLoadFailureIssue creates a new check-load failure issue template.
func NewCheckLoadFailureIssue() *CheckLoadFailureIssue {
	return &CheckLoadFailureIssue{}
}

// BuildIssue creates a complete issue with metadata and remediation for a
// check-load failure. context["check_name"] is the integration name and
// context["errors"] the concatenated per-loader error message.
func (t *CheckLoadFailureIssue) BuildIssue(context map[string]string) (*healthplatform.Issue, error) {
	checkName := context[contextKeyCheckName]
	if checkName == "" {
		checkName = "unknown"
	}
	errMsg := context[contextKeyErrors]

	extra, err := structpb.NewStruct(map[string]any{
		contextKeyCheckName: checkName,
		contextKeyErrors:    errMsg,
		contextKeyImpact:    impactMsg,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to create extra: %v", err)
	}

	return &healthplatform.Issue{
		IssueName:   IssueName,
		IssueType:   IssueType,
		Title:       fmt.Sprintf("Check '%s' Failed to Load", checkName),
		Description: fmt.Sprintf("Check '%s' could not be loaded by any loader: %s", checkName, errMsg),
		Category:    category,
		Location:    location,
		Severity:    severity,
		Source:      Source,
		Extra:       extra,
		Tags:        []string{"collector", "check_load"},
		Remediation: &healthplatform.Remediation{
			Summary: fmt.Sprintf("Fix the configuration or environment issue preventing '%s' from loading.", checkName),
			Steps: []*healthplatform.RemediationStep{
				{Order: 1, Text: "Review the error message above for the specific loader failure."},
				{Order: 2, Text: fmt.Sprintf("Run `datadog-agent check %s` for a detailed trace of the failure.", checkName)},
				{Order: 3, Text: "Fix the check configuration or the underlying integration prerequisite, then restart the Datadog Agent."},
			},
		},
	}, nil
}
