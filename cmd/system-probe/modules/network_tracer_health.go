// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

//go:build linux && linux_bpf

package modules

import (
	"errors"
	"strconv"

	healthplatformpayload "github.com/DataDog/agent-payload/v5/healthplatform"
	"github.com/DataDog/datadog-agent/pkg/system-probe/api/module"
	"github.com/DataDog/datadog-agent/pkg/system-probe/healthreporter"
)

// Per-error-type issue IDs so each failure mode has an independent lifecycle.
const (
	networkProbeKernelIssueID = "network-probe-kernel-unsupported"
	networkProbeUSMIssueID    = "network-probe-usm-unsupported"
)

// networkProbeIssueIDFor returns the issue ID for a known failure type, or "" for unrecognized errors.
func networkProbeIssueIDFor(initErr error) string {
	switch {
	case errors.Is(initErr, errNetworkProbeKernelUnsupported):
		return networkProbeKernelIssueID
	case errors.Is(initErr, errNetworkProbeUSMUnsupported):
		return networkProbeUSMIssueID
	}
	return ""
}

// networkProbeIssueNameFor returns the human-readable issue name for a known failure type.
func networkProbeIssueNameFor(initErr error) string {
	switch {
	case errors.Is(initErr, errNetworkProbeKernelUnsupported):
		return "Network Probe Kernel Unsupported"
	case errors.Is(initErr, errNetworkProbeUSMUnsupported):
		return "Network Probe USM Unsupported"
	}
	return ""
}

// reportNetworkProbeInitFailure sends a fully-built health issue to the core agent for known
// failure types. Unrecognized errors are not reported (they are already logged by the caller).
func reportNetworkProbeInitFailure(deps module.FactoryDependencies, initErr error, npmEnabled, usmEnabled bool) {
	issue := buildNetworkProbeIssue(initErr, npmEnabled, usmEnabled)
	if issue == nil {
		return
	}
	healthreporter.New(deps.Ipc).ReportWithRetry(issue)
}

// resolveNetworkProbeKernelIssue clears the kernel-unsupported issue once the kernel check passes.
func resolveNetworkProbeKernelIssue(deps module.FactoryDependencies) {
	healthreporter.New(deps.Ipc).ResolveWithRetry(networkProbeKernelIssueID)
}

// resolveNetworkProbeUSMIssue clears the USM issue once the tracer initializes successfully.
func resolveNetworkProbeUSMIssue(deps module.FactoryDependencies) {
	healthreporter.New(deps.Ipc).ResolveWithRetry(networkProbeUSMIssueID)
}

// buildNetworkProbeIssue returns a health issue for a known failure type, or nil for unrecognized errors.
func buildNetworkProbeIssue(initErr error, npmEnabled, usmEnabled bool) *healthplatformpayload.Issue {
	issueID := networkProbeIssueIDFor(initErr)
	if issueID == "" {
		return nil
	}

	errStr := "unknown error"
	if initErr != nil {
		errStr = initErr.Error()
	}

	var which string
	if errors.Is(initErr, errNetworkProbeUSMUnsupported) {
		// USM-specific failure: only USM is affected regardless of whether CNM is also enabled.
		which = "USM"
	} else {
		// Kernel-level failure: the entire tracer is affected, report all enabled features.
		switch {
		case npmEnabled && usmEnabled:
			which = "CNM and USM"
		case npmEnabled:
			which = "CNM"
		case usmEnabled:
			which = "USM"
		default:
			which = "network monitoring"
		}
	}

	extra := healthreporter.StringStruct(map[string]string{
		"error":       errStr,
		"npm_enabled": strconv.FormatBool(npmEnabled),
		"usm_enabled": strconv.FormatBool(usmEnabled),
	})

	tags := []string{"system-probe", "ebpf", "network-monitoring"}
	if npmEnabled {
		tags = append(tags, "npm", "cnm")
	}
	if usmEnabled {
		tags = append(tags, "usm")
	}

	return &healthplatformpayload.Issue{
		Id:          issueID,
		IssueName:   networkProbeIssueNameFor(initErr),
		Title:       which + " eBPF Probe Failed to Initialize",
		Description: which + " is enabled but the eBPF network probe failed to load: " + errStr,
		Category:    "runtime",
		Location:    "system-probe",
		Severity:    healthplatformpayload.IssueSeverity_ISSUE_SEVERITY_HIGH,
		Source:      "system-probe",
		Extra:       extra,
		Tags:        tags,
		Remediation: networkProbeRemediation(initErr),
	}
}

func networkProbeRemediation(initErr error) *healthplatformpayload.Remediation {
	switch {
	case errors.Is(initErr, errNetworkProbeKernelUnsupported):
		return &healthplatformpayload.Remediation{
			Summary: "The running kernel does not meet the minimum version requirements for this feature.",
			Steps: []*healthplatformpayload.RemediationStep{
				{Order: 1, Text: "Check system-probe logs: journalctl -u datadog-agent-sysprobe or /var/log/datadog/system-probe.log"},
				{Order: 2, Text: "Check the kernel version: uname -r (CNM requires >= 4.4, USM requires >= 4.14)"},
				{Order: 3, Text: "Upgrade the host kernel or disable the unsupported feature in system-probe.yaml"},
				{Order: 4, Text: "Restart after fixing: sudo systemctl restart datadog-agent-sysprobe"},
			},
		}
	case errors.Is(initErr, errNetworkProbeUSMUnsupported):
		return &healthplatformpayload.Remediation{
			Summary: "Universal Service Monitoring (USM) requires a newer kernel than the one running.",
			Steps: []*healthplatformpayload.RemediationStep{
				{Order: 1, Text: "Check system-probe logs: journalctl -u datadog-agent-sysprobe or /var/log/datadog/system-probe.log"},
				{Order: 2, Text: "Check the kernel version: uname -r (USM requires >= 4.14)"},
				{Order: 3, Text: "Upgrade the host kernel or disable USM (service_monitoring_config.enabled: false) in datadog.yaml"},
				{Order: 4, Text: "Restart after fixing: sudo systemctl restart datadog-agent-sysprobe"},
			},
		}
	}
	return nil
}
