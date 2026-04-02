// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

package logsmultilinejournal

import (
	"fmt"

	"github.com/DataDog/agent-payload/v5/healthplatform"
	"google.golang.org/protobuf/types/known/structpb"
)

// Issue provides the issue template for multi_line-on-journald misconfiguration
type Issue struct{}

// NewIssue creates a new Issue template
func NewIssue() *Issue {
	return &Issue{}
}

// BuildIssue creates a complete issue with metadata and remediation steps.
// Expected context keys:
//   - "affectedSources": comma-separated list of affected source names
//   - "sourceType":      "journald" (or "json" in future)
func (t *Issue) BuildIssue(context map[string]string) (*healthplatform.Issue, error) {
	affectedSources := context["affectedSources"]
	if affectedSources == "" {
		affectedSources = "unknown"
	}

	sourceType := context["sourceType"]
	if sourceType == "" {
		sourceType = "journald"
	}

	extra, err := structpb.NewStruct(map[string]any{
		"affected_sources": affectedSources,
		"source_type":      sourceType,
		"impact":           "Logs are collected as individual entries without the expected multi-line aggregation.",
	})
	if err != nil {
		return nil, fmt.Errorf("failed to create issue extra: %v", err)
	}

	return &healthplatform.Issue{
		Id:          IssueID,
		IssueName:   "logs_multiline_journald_unsupported",
		Title:       "multi_line Aggregation Has No Effect on journald/JSON Log Sources",
		Description: "One or more log sources are configured with `multi_line` processing rules but use journald or JSON-formatted sources, where multi_line aggregation is not supported. The rules are silently ignored — logs are collected as individual entries without the expected aggregation.",
		Category:    "configuration",
		Location:    "logs-agent",
		Severity:    "medium",
		Source:      "logs",
		Extra:       extra,
		Remediation: buildRemediation(affectedSources),
		Tags:        []string{"logs", "multiline", "journald", "json", "configuration", "aggregation"},
	}, nil
}

func buildRemediation(affectedSources string) *healthplatform.Remediation {
	return &healthplatform.Remediation{
		Summary: fmt.Sprintf("Remove multi_line processing rules from journald/JSON log sources (%s) or switch to a supported aggregation approach.", affectedSources),
		Steps: []*healthplatform.RemediationStep{
			{
				Order: 1,
				Text:  "Identify the affected log source configuration files (usually under /etc/datadog-agent/conf.d/).",
			},
			{
				Order: 2,
				Text:  "Open the relevant conf.yaml file and locate the `log_processing_rules` block for the journald source.",
			},
			{
				Order: 3,
				Text:  "Remove or comment out any rule with `type: multi_line` from the affected journald log source.",
			},
			{
				Order: 4,
				Text:  "If multi-line aggregation is still required, consider forwarding journal entries to a file tailer that supports multi_line, or restructure the application to emit single-line JSON events.",
			},
			{
				Order: 5,
				Text:  "Restart the Datadog Agent: `systemctl restart datadog-agent`",
			},
			{
				Order: 6,
				Text:  "Verify the agent status with `datadog-agent status` to confirm the log source is active and no warnings are shown.",
			},
		},
	}
}
