// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

package logshosttagsmissing

import (
	"github.com/DataDog/agent-payload/v5/healthplatform"
)

// Issue provides the issue template for logs host tags missing.
type Issue struct{}

// NewIssue creates a new Issue template.
func NewIssue() *Issue {
	return &Issue{}
}

// BuildIssue creates a complete issue with metadata and remediation steps.
func (t *Issue) BuildIssue(_ map[string]string) (*healthplatform.Issue, error) {
	return &healthplatform.Issue{
		Id:          IssueID,
		IssueName:   "logs_host_tags_missing",
		Title:       "Host Tags Not Applied to Logs in Logs-Only Agent Mode",
		Description: "The agent is collecting logs but host-level tags (env, service, version, custom tags) are not configured. In logs-only deployments, tags must be explicitly set via the `tags:` key in datadog.yaml or the `DD_TAGS` environment variable to appear on log entries.",
		Category:    "configuration",
		Location:    "logs-agent",
		Severity:    "medium",
		DetectedAt:  "", // Filled by the health platform
		Source:      "logs",
		Remediation: buildRemediation(),
		Tags:        []string{"logs", "tags", "configuration", "logs-only", "host-tags"},
	}, nil
}

// buildRemediation returns the remediation steps for the issue.
func buildRemediation() *healthplatform.Remediation {
	return &healthplatform.Remediation{
		Summary: "Configure host-level tags so they are applied to all collected log entries",
		Steps: []*healthplatform.RemediationStep{
			{
				Order: 1,
				Text:  "Add tags to datadog.yaml: `tags: [\"env:production\", \"service:myapp\"]`",
			},
			{
				Order: 2,
				Text:  "OR set the DD_TAGS environment variable: DD_TAGS=env:production,service:myapp",
			},
			{
				Order: 3,
				Text:  "Restart the agent after applying the configuration change",
			},
			{
				Order: 4,
				Text:  "Verify tags appear on log entries: datadog-agent status | grep -A5 \"Log Agent\"",
			},
		},
	}
}
