// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

//go:build (linux && linux_bpf) || (windows && npm) || darwin

package modules

import (
	"fmt"

	healthplatformpayload "github.com/DataDog/agent-payload/v5/healthplatform"
	"github.com/DataDog/datadog-agent/pkg/system-probe/api/module"
	"github.com/DataDog/datadog-agent/pkg/system-probe/healthreporter"
	"google.golang.org/protobuf/types/known/structpb"
)

const (
	networkProbeIssueID   = "network-probe-init-failure"
	networkProbeIssueName = "network_probe_init_failure"
)

// reportNetworkProbeInitFailure sends a fully-built health issue to the core agent.
// system-probe owns the issue metadata so it does not rely on the core agent's
// template registry.
func reportNetworkProbeInitFailure(deps module.FactoryDependencies, initErr error, npmEnabled, usmEnabled bool) {
	healthreporter.New(deps.Ipc).ReportWithRetry(buildNetworkProbeIssue(initErr, npmEnabled, usmEnabled))
}

// resolveNetworkProbeInitFailure clears a previously reported network probe failure.
// Called on successful initialization to clean up stale issues from prior failed runs.
func resolveNetworkProbeInitFailure(deps module.FactoryDependencies) {
	healthreporter.New(deps.Ipc).ResolveWithRetry(networkProbeIssueID)
}

func buildNetworkProbeIssue(initErr error, npmEnabled, usmEnabled bool) *healthplatformpayload.Issue {
	errStr := "unknown error"
	if initErr != nil {
		errStr = initErr.Error()
	}

	var which string
	switch {
	case npmEnabled && usmEnabled:
		which = "NPM and USM"
	case npmEnabled:
		which = "NPM"
	case usmEnabled:
		which = "USM"
	default:
		which = "network monitoring"
	}

	extra, _ := structpb.NewStruct(map[string]any{
		"error":       errStr,
		"npm_enabled": fmt.Sprintf("%v", npmEnabled),
		"usm_enabled": fmt.Sprintf("%v", usmEnabled),
	})

	return &healthplatformpayload.Issue{
		Id:          networkProbeIssueID,
		IssueName:   networkProbeIssueName,
		Title:       fmt.Sprintf("%s eBPF Probe Failed to Initialize", which),
		Description: fmt.Sprintf("%s is enabled but the eBPF network probe failed to load: %s", which, errStr),
		Category:    "runtime",
		Location:    "system-probe",
		Severity:    healthplatformpayload.IssueSeverity_ISSUE_SEVERITY_HIGH,
		Source:      "system-probe",
		Extra:       extra,
		Tags:        []string{"system-probe", "npm", "usm", "ebpf", "network-monitoring"},
		Remediation: &healthplatformpayload.Remediation{
			Summary: "Check kernel compatibility and system-probe capabilities, then restart system-probe.",
			Steps: []*healthplatformpayload.RemediationStep{
				{Order: 1, Text: "Check system-probe logs: journalctl -u datadog-agent-sysprobe or /var/log/datadog/system-probe.log"},
				{Order: 2, Text: "Verify kernel version (>= 4.4 for NPM, >= 4.14 for USM): uname -r"},
				{Order: 3, Text: "Check BTF availability for CO-RE probes: ls /sys/kernel/btf/vmlinux"},
				{Order: 4, Text: "Verify capabilities: CAP_NET_ADMIN, CAP_SYS_ADMIN (CAP_BPF on kernel >= 5.8)"},
				{Order: 5, Text: "If in a container, ensure privileged mode or a permissive seccomp profile"},
				{Order: 6, Text: "Restart after fixing: systemctl restart datadog-agent-sysprobe"},
			},
		},
	}
}
