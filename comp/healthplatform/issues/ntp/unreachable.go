// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

package ntp

import (
	"fmt"

	"github.com/DataDog/agent-payload/v5/healthplatform"
	"google.golang.org/protobuf/types/known/structpb"
)

const (
	// UnreachableIssueName is the identifier for NTP unreachable issues,
	// used as the template registry key and the proto IssueName field.
	UnreachableIssueName = "NTP Servers Unreachable"

	// UnreachableIssueID is the unique instance id used when reporting this issue.
	UnreachableIssueID = "ntp-unreachable"
)

// Context keys read by UnreachableIssue.BuildIssue.
const (
	contextKeyServers = "servers"
	contextKeyError   = "error"
)

// UnreachableIssue provides the complete issue template (metadata + remediation)
// for when the NTP check cannot reach any configured NTP server.
type UnreachableIssue struct{}

// NewUnreachableIssue creates a new NTP unreachable issue template.
func NewUnreachableIssue() *UnreachableIssue {
	return &UnreachableIssue{}
}

// BuildIssue creates a complete issue with metadata and remediation.
func (t *UnreachableIssue) BuildIssue(context map[string]string) (*healthplatform.Issue, error) {
	servers := context[contextKeyServers]
	if servers == "" {
		servers = "unknown"
	}
	queryErr := context[contextKeyError]
	if queryErr == "" {
		queryErr = "unknown"
	}

	extra, err := structpb.NewStruct(map[string]any{
		"servers": servers,
		"error":   queryErr,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to create issue extra: %w", err)
	}

	return &healthplatform.Issue{
		IssueName: UnreachableIssueName,
		Title:     "Datadog Agent Cannot Reach Any NTP Server",
		Description: fmt.Sprintf(
			"The NTP check could not reach any of the configured NTP servers (%s), so the Agent cannot verify "+
				"whether the system clock is in sync. This is usually caused by outbound NTP traffic (UDP/123) "+
				"being blocked by a firewall, security group, or network policy, rather than an actual clock "+
				"problem. Underlying error: %s",
			servers, queryErr,
		),
		Category:   "connectivity",
		Location:   "system",
		Severity:   healthplatform.IssueSeverity_ISSUE_SEVERITY_MEDIUM,
		DetectedAt: "", // Filled by the health platform
		Source:     "ntp",
		Extra:      extra,
		Remediation: &healthplatform.Remediation{
			Summary: "Allow outbound NTP traffic, or disable the check if NTP monitoring isn't needed",
			Steps:   unreachableRemediationSteps(),
		},
		Tags: []string{"ntp", "connectivity"},
	}, nil
}

func unreachableRemediationSteps() []*healthplatform.RemediationStep {
	return []*healthplatform.RemediationStep{
		{Order: 1, Text: "Confirm outbound UDP traffic on port 123 is allowed to the configured NTP server(s)."},
		{Order: 2, Text: "If running in a container platform (e.g. ECS/Fargate), check security group and network ACL rules for the task."},
		{Order: 3, Text: "If NTP monitoring isn't needed, disable the check: set 'instances: []' in ntp.d/conf.yaml, or replace the ntp.d directory with an empty mount."},
		{Order: 4, Text: "After making changes, restart the Agent and confirm with 'agent status' that the ntp check reports OK."},
	}
}
