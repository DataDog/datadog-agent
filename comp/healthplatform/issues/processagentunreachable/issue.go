// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

package processagentunreachable

import (
	"fmt"

	"github.com/DataDog/agent-payload/v5/healthplatform"
	"google.golang.org/protobuf/types/known/structpb"
)

// ProcessAgentUnreachableIssue provides the issue template for process-agent reachability failures
type ProcessAgentUnreachableIssue struct{}

// NewProcessAgentUnreachableIssue creates a new process-agent unreachable issue template
func NewProcessAgentUnreachableIssue() *ProcessAgentUnreachableIssue {
	return &ProcessAgentUnreachableIssue{}
}

// BuildIssue creates a complete issue with metadata and remediation steps
func (t *ProcessAgentUnreachableIssue) BuildIssue(context map[string]string) (*healthplatform.Issue, error) {
	port := context["port"]
	if port == "" {
		port = fmt.Sprintf("%d", defaultCmdPort)
	}

	extra, err := structpb.NewStruct(map[string]any{
		"port":    port,
		"enabled": "true",
		"impact":  "Process metrics will not be collected by the Datadog Agent.",
	})
	if err != nil {
		return nil, fmt.Errorf("failed to create issue extra: %w", err)
	}

	return &healthplatform.Issue{
		Id:          IssueID,
		IssueName:   "process_agent_unreachable",
		Title:       "Process Agent Is Not Reachable",
		Description: fmt.Sprintf("Process collection is enabled in datadog.yaml but the process-agent is not running or not reachable on port %s. Process metrics will not be collected.", port),
		Category:    "configuration",
		Location:    "process-agent",
		Severity:    "medium",
		DetectedAt:  "", // Will be filled by health platform
		Source:      "agent",
		Extra:       extra,
		Remediation: &healthplatform.Remediation{
			Summary: "Start the process-agent service or verify the process collection configuration.",
			Steps: []*healthplatform.RemediationStep{
				{Order: 1, Text: "Start the process-agent (Linux): systemctl start datadog-agent-process"},
				{Order: 2, Text: "Start the process-agent (Windows): Start-Service datadog-agent-process"},
				{Order: 3, Text: "Check process-agent logs: /var/log/datadog/process-agent.log"},
				{Order: 4, Text: "Verify process_config.process_collection.enabled: true is set in datadog.yaml"},
				{Order: 5, Text: "Ensure the process-agent binary exists and is executable"},
			},
		},
		Tags: []string{"process-agent", "process-collection", "configuration"},
	}, nil
}
