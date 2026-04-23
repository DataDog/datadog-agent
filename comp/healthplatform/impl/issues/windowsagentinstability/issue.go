// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

package windowsagentinstability

import (
	"fmt"

	"github.com/DataDog/agent-payload/v5/healthplatform"
	"google.golang.org/protobuf/types/known/structpb"
)

// WindowsAgentInstabilityIssue provides the complete issue template for Windows agent service crashes
type WindowsAgentInstabilityIssue struct{}

// NewWindowsAgentInstabilityIssue creates a new Windows agent instability issue template
func NewWindowsAgentInstabilityIssue() *WindowsAgentInstabilityIssue {
	return &WindowsAgentInstabilityIssue{}
}

// BuildIssue creates a complete issue with metadata and remediation for Windows agent crashes
func (t *WindowsAgentInstabilityIssue) BuildIssue(context map[string]string) (*healthplatform.Issue, error) {
	crashCount := context["crashCount"]
	if crashCount == "" {
		crashCount = "unknown"
	}

	tw := context["timeWindow"]
	if tw == "" {
		tw = "24h"
	}

	extra, err := structpb.NewStruct(map[string]any{
		"crash_count": crashCount,
		"time_window": tw,
		"impact":      "The Datadog Agent may fail to collect metrics, logs, and traces during crash/restart cycles",
	})
	if err != nil {
		return nil, fmt.Errorf("failed to create issue extra: %v", err)
	}

	return &healthplatform.Issue{
		Id:          IssueID,
		IssueName:   "windows_agent_service_instability",
		Title:       "Datadog Agent Service Is Unstable on Windows",
		Description: fmt.Sprintf("The Datadog Agent service has crashed or restarted %s times in the last %s. This may indicate a compatibility issue, resource exhaustion, or a bug.", crashCount, tw),
		Category:    "stability",
		Location:    "windows-service",
		Severity:    "high",
		DetectedAt:  "",
		Source:      "agent",
		Extra:       extra,
		Remediation: &healthplatform.Remediation{
			Summary: "Investigate repeated Datadog Agent service crashes on Windows and restore stable operation",
			Steps: []*healthplatform.RemediationStep{
				{Order: 1, Text: `Check Windows Event Log for service crash events: Get-WinEvent -LogName System -Source "Service Control Manager" | Where-Object {$_.Message -match "Datadog"}`},
				{Order: 2, Text: `Review agent logs at C:\ProgramData\Datadog\logs\agent.log`},
				{Order: 3, Text: "Ensure the agent has sufficient memory by reviewing the memory_limit_pct configuration option"},
				{Order: 4, Text: "Upgrade to the latest Datadog Agent version to benefit from recent stability fixes"},
				{Order: 5, Text: `Contact Datadog support with an agent flare: & "C:\Program Files\Datadog\Datadog Agent\bin\agent.exe" flare`},
			},
		},
		Tags: []string{"windows", "service-crash", "stability", "instability"},
	}, nil
}
