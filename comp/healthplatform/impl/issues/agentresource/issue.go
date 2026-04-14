// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

package agentresource

import (
	"fmt"

	"github.com/DataDog/agent-payload/v5/healthplatform"
)

// AgentResourceIssue provides the issue template for agent high resource usage
type AgentResourceIssue struct{}

// NewAgentResourceIssue creates a new agent resource issue template
func NewAgentResourceIssue() *AgentResourceIssue {
	return &AgentResourceIssue{}
}

// BuildIssue creates a complete issue with metadata and remediation from context.
// Expected context keys: cpu_percent, cpu_threshold, memory_mb, memory_threshold_mb.
func (t *AgentResourceIssue) BuildIssue(context map[string]string) (*healthplatform.Issue, error) {
	cpuPercent := context["cpu_percent"]
	cpuThreshold := context["cpu_threshold"]
	memoryMB := context["memory_mb"]
	memoryThresholdMB := context["memory_threshold_mb"]

	return &healthplatform.Issue{
		Id:          IssueID,
		IssueName:   "agent_high_resource_usage",
		Title:       "Agent Process High Resource Usage",
		Description: buildDescription(cpuPercent, cpuThreshold, memoryMB, memoryThresholdMB),
		Category:    "resource",
		Location:    "core-agent",
		Severity:    "medium",
		Source:      "agent",
		Remediation: buildRemediation(),
		Tags:        []string{"cpu", "memory", "resource", "performance"},
	}, nil
}

func buildDescription(cpuPercent, cpuThreshold, memoryMB, memoryThresholdMB string) string {
	hasCPU := cpuPercent != "" && cpuThreshold != "0"
	hasMem := memoryMB != "" && memoryThresholdMB != "0"

	switch {
	case hasCPU && hasMem:
		return fmt.Sprintf(
			"The Datadog Agent process is consuming high CPU (%s%%, threshold: %s%%) "+
				"and memory (%s MB, threshold: %s MB). "+
				"This may indicate a resource leak, a misconfigured integration, or an unexpected spike in monitored activity.",
			cpuPercent, cpuThreshold, memoryMB, memoryThresholdMB,
		)
	case hasCPU:
		return fmt.Sprintf(
			"The Datadog Agent process is consuming high CPU (%s%%, threshold: %s%%). "+
				"This may indicate a busy integration, an eBPF probe processing too many events, or a resource leak.",
			cpuPercent, cpuThreshold,
		)
	default:
		return fmt.Sprintf(
			"The Datadog Agent process is consuming high memory (%s MB, threshold: %s MB). "+
				"This may indicate a memory leak in an integration or the agent core.",
			memoryMB, memoryThresholdMB,
		)
	}
}

func buildRemediation() *healthplatform.Remediation {
	return &healthplatform.Remediation{
		Summary: "Investigate which agent component is consuming excessive resources and consider restarting the agent",
		Steps: []*healthplatform.RemediationStep{
			{Order: 1, Text: "Check which integrations are running: datadog-agent check --list"},
			{Order: 2, Text: "Review agent status and check run times: datadog-agent status"},
			{Order: 3, Text: "Generate a diagnostic flare to capture profiles: datadog-agent flare"},
			{Order: 4, Text: "If the issue persists, restart the agent: systemctl restart datadog-agent (Linux) or Restart-Service datadogagent (Windows)"},
		},
	}
}
