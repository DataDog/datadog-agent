// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

package systemprobeunreachable

import (
	"fmt"

	"github.com/DataDog/agent-payload/v5/healthplatform"
	"google.golang.org/protobuf/types/known/structpb"
)

// SystemProbeUnreachableIssue provides the issue template for system-probe unreachable errors
type SystemProbeUnreachableIssue struct{}

// NewSystemProbeUnreachableIssue creates a new system-probe unreachable issue template
func NewSystemProbeUnreachableIssue() *SystemProbeUnreachableIssue {
	return &SystemProbeUnreachableIssue{}
}

// BuildIssue creates a complete issue with metadata and remediation steps
func (t *SystemProbeUnreachableIssue) BuildIssue(context map[string]string) (*healthplatform.Issue, error) {
	socketPath := context["socket"]
	if socketPath == "" {
		socketPath = "/var/run/sysprobe/sysprobe.sock"
	}

	networkEnabled := context["network_enabled"]

	extra, err := structpb.NewStruct(map[string]any{
		"socket":          socketPath,
		"network_enabled": networkEnabled,
		"impact":          "Network Performance Monitoring and/or Universal Service Monitoring data will not be collected.",
	})
	if err != nil {
		return nil, fmt.Errorf("error building system-probe unreachable issue: %w", err)
	}

	remediation := &healthplatform.Remediation{
		Summary: "Start system-probe and ensure its socket is accessible.",
		Steps: []*healthplatform.RemediationStep{
			{Order: 1, Text: "Start system-probe: systemctl start datadog-agent-sysprobe"},
			{Order: 2, Text: "Check system-probe logs: /var/log/datadog/system-probe.log"},
			{Order: 3, Text: fmt.Sprintf("Verify the socket path matches system_probe_config.sysprobe_socket in config (current: %s)", socketPath)},
			{Order: 4, Text: "Ensure system-probe has the required kernel permissions (e.g. CAP_NET_ADMIN, CAP_SYS_ADMIN)"},
		},
	}

	return &healthplatform.Issue{
		Id:          IssueID,
		IssueName:   "system_probe_unreachable",
		Title:       "System Probe Is Not Reachable",
		Description: fmt.Sprintf("Network Performance Monitoring or Universal Service Monitoring is enabled but the system-probe is not running or its socket %s is not accessible.", socketPath),
		Category:    "configuration",
		Location:    "system-probe",
		Severity:    "medium",
		DetectedAt:  "", // Will be filled by health platform
		Source:      "agent",
		Extra:       extra,
		Remediation: remediation,
		Tags:        []string{"system-probe", "npm", "usm", "network-monitoring"},
	}, nil
}
