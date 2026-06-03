// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

package networkprobefailure

import (
	"fmt"

	"github.com/DataDog/agent-payload/v5/healthplatform"
	"google.golang.org/protobuf/types/known/structpb"
)

const (
	category = "runtime"
	location = "system-probe"
	severity = healthplatform.IssueSeverity_ISSUE_SEVERITY_HIGH
	source   = "system-probe"
)

// NetworkProbeFailureIssue provides the issue template for network probe initialization failures.
type NetworkProbeFailureIssue struct{}

// NewNetworkProbeFailureIssue creates a new network probe failure issue template.
func NewNetworkProbeFailureIssue() *NetworkProbeFailureIssue {
	return &NetworkProbeFailureIssue{}
}

// BuildIssue creates a complete issue with metadata and remediation steps.
// Context keys: "error" (initialization error message), "npm_enabled", "usm_enabled".
func (t *NetworkProbeFailureIssue) BuildIssue(ctx map[string]string) (*healthplatform.Issue, error) {
	errMsg := ctx["error"]
	if errMsg == "" {
		errMsg = "unknown error"
	}
	npmEnabled := ctx["npm_enabled"]
	usmEnabled := ctx["usm_enabled"]

	var which string
	switch {
	case npmEnabled == "true" && usmEnabled == "true":
		which = "NPM and USM"
	case npmEnabled == "true":
		which = "NPM"
	case usmEnabled == "true":
		which = "USM"
	default:
		which = "network monitoring"
	}

	extra, err := structpb.NewStruct(map[string]any{
		"error":       errMsg,
		"npm_enabled": npmEnabled,
		"usm_enabled": usmEnabled,
		"impact":      fmt.Sprintf("%s data will not be collected until system-probe is restarted with a working eBPF probe.", which),
	})
	if err != nil {
		return nil, fmt.Errorf("failed to build networkprobefailure extra: %w", err)
	}

	return &healthplatform.Issue{
		Id:          IssueID,
		IssueName:   IssueName,
		Title:       fmt.Sprintf("%s eBPF Probe Failed to Initialize", which),
		Description: fmt.Sprintf("%s is enabled but the eBPF network probe failed to load: %s", which, errMsg),
		Category:    category,
		Location:    location,
		Severity:    severity,
		Source:      source,
		Extra:       extra,
		Remediation: &healthplatform.Remediation{
			Summary: "Check kernel compatibility and system-probe capabilities, then restart system-probe.",
			Steps: []*healthplatform.RemediationStep{
				{Order: 1, Text: "Check system-probe logs for the root cause: journalctl -u datadog-agent-sysprobe or /var/log/datadog/system-probe.log"},
				{Order: 2, Text: "Verify kernel version (>= 4.4 for NPM, >= 4.14 for USM): uname -r"},
				{Order: 3, Text: "Check BTF availability for CO-RE probes: ls /sys/kernel/btf/vmlinux"},
				{Order: 4, Text: "Verify system-probe has required capabilities: CAP_NET_ADMIN, CAP_SYS_ADMIN (CAP_BPF on kernel >= 5.8)"},
				{Order: 5, Text: "If running in a container, ensure the container is privileged or has a permissive seccomp profile"},
				{Order: 6, Text: "Restart system-probe after fixing the underlying issue: systemctl restart datadog-agent-sysprobe"},
			},
		},
		Tags: []string{"system-probe", "npm", "usm", "ebpf", "network-monitoring"},
	}, nil
}
