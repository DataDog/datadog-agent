// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

package pingicmppermissions

import (
	"fmt"

	"github.com/DataDog/agent-payload/v5/healthplatform"
	"google.golang.org/protobuf/types/known/structpb"
)

// PingICMPPermissionsIssue provides the complete issue template (metadata + remediation)
type PingICMPPermissionsIssue struct{}

// NewPingICMPPermissionsIssue creates a new ping ICMP permissions issue template
func NewPingICMPPermissionsIssue() *PingICMPPermissionsIssue {
	return &PingICMPPermissionsIssue{}
}

// BuildIssue creates a complete issue with metadata and remediation steps
func (t *PingICMPPermissionsIssue) BuildIssue(context map[string]string) (*healthplatform.Issue, error) {
	errMsg := context["error"]
	if errMsg == "" {
		errMsg = "operation not permitted"
	}

	issueExtra, err := structpb.NewStruct(map[string]any{
		"integration": "ping",
		"error":       errMsg,
		"capability":  "NET_RAW",
	})
	if err != nil {
		return nil, fmt.Errorf("failed to create issue extra: %v", err)
	}

	return &healthplatform.Issue{
		Id:          IssueID,
		IssueName:   "ping_icmp_permissions",
		Title:       "Agent Cannot Create ICMP Socket for Ping Check",
		Description: "The Datadog agent does not have permission to create a raw ICMP socket required by the ping integration. The ping check will fail with 'operation not permitted'.",
		Category:    "permissions",
		Location:    "ping-integration",
		Severity:    "warning",
		DetectedAt:  "", // Will be filled by health platform
		Source:      "ping",
		Extra:       issueExtra,
		Remediation: t.buildRemediation(),
		Tags:        []string{"ping", "icmp", "permissions", "net_raw", "linux"},
	}, nil
}

// buildRemediation creates the remediation steps for the ICMP socket permission issue
func (t *PingICMPPermissionsIssue) buildRemediation() *healthplatform.Remediation {
	return &healthplatform.Remediation{
		Summary: "Grant the agent NET_RAW capability or run with sufficient privileges to create raw ICMP sockets",
		Steps: []*healthplatform.RemediationStep{
			{Order: 1, Text: "Grant NET_RAW capability to the agent binary: setcap cap_net_raw+ep /usr/bin/datadog-agent"},
			{Order: 2, Text: "Or run the agent as root (not recommended for production)"},
			{Order: 3, Text: "In Kubernetes, add NET_RAW to the agent container's securityContext capabilities: capabilities: add: [NET_RAW]"},
			{Order: 4, Text: "Verify with: capsh --print | grep net_raw"},
		},
	}
}
