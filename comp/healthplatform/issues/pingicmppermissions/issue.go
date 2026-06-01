// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

//go:build linux

package pingicmppermissions

import (
	"fmt"

	"github.com/DataDog/agent-payload/v5/healthplatform"
	"google.golang.org/protobuf/types/known/structpb"
)

const (
	// IssueName is the registry key and the proto IssueName field for ping ICMP permission issues.
	IssueName = "ping_icmp_permissions"

	// IssueID is the unique instance ID used when reporting this issue.
	IssueID = "ping-icmp-permissions"
)

// PingICMPPermissionsIssue provides the issue template for ping ICMP socket permission errors.
type PingICMPPermissionsIssue struct{}

// IssueName returns the registry key for this issue type.
func (t *PingICMPPermissionsIssue) IssueName() string { return IssueName }

// BuildIssue creates a complete issue with metadata and remediation steps.
func (t *PingICMPPermissionsIssue) BuildIssue(context map[string]string) (*healthplatform.Issue, error) {
	errMsg := context["error"]
	if errMsg == "" {
		errMsg = "operation not permitted"
	}

	extra, err := structpb.NewStruct(map[string]any{
		"integration": "ping",
		"error":       errMsg,
		"capability":  "NET_RAW",
	})
	if err != nil {
		return nil, fmt.Errorf("failed to create issue extra: %v", err)
	}

	return &healthplatform.Issue{
		IssueName:   IssueName,
		Title:       "Agent Cannot Create ICMP Socket for Ping Check",
		Description: "The Datadog agent does not have permission to create a raw ICMP socket required by the ping integration. The ping check will fail with 'operation not permitted'.",
		Category:    "permissions",
		Location:    "ping-integration",
		Severity:    healthplatform.IssueSeverity_ISSUE_SEVERITY_MEDIUM,
		Source:      "ping",
		Extra:       extra,
		Remediation: &healthplatform.Remediation{
			Summary: "Grant the agent NET_RAW capability or run with sufficient privileges to create raw ICMP sockets",
			Steps: []*healthplatform.RemediationStep{
				{Order: 1, Text: "Grant NET_RAW capability to the agent binary: setcap cap_net_raw+ep /usr/bin/datadog-agent"},
				{Order: 2, Text: "In Kubernetes, add NET_RAW to the agent container's securityContext: capabilities: add: [NET_RAW]"},
				{Order: 3, Text: "Verify with: capsh --print | grep net_raw"},
			},
		},
		Tags: []string{"ping", "icmp", "permissions", "net_raw", "linux"},
	}, nil
}
