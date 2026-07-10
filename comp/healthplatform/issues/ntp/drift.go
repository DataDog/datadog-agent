// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

// Package ntp provides the health-platform issue templates for the ntp check:
// clock drift (DriftIssue) and unreachable NTP servers (UnreachableIssue).
// Detection lives in the existing pkg/collector/corechecks/net/ntp check, which
// already queries NTP servers and computes the clock offset on every run; that
// check reports/resolves these issues directly via store.ReportIssue (Path B),
// so this package has no init()/module registration and is not blank-imported
// in bundle.go.
package ntp

import (
	"fmt"
	"runtime"

	"github.com/DataDog/agent-payload/v5/healthplatform"
	"google.golang.org/protobuf/types/known/structpb"
)

const (
	// DriftIssueName is the identifier for NTP clock drift issues,
	// used as the template registry key and the proto IssueName field.
	DriftIssueName = "NTP Clock Drift"

	// DriftIssueID is the unique instance id used when reporting this issue.
	DriftIssueID = "ntp-clock-drift"
)

// Context keys read by DriftIssue.BuildIssue.
const (
	contextKeyDrift     = "drift"
	contextKeyNTPServer = "ntpServer"
	contextKeyThreshold = "threshold"
)

// DriftIssue provides the complete issue template (metadata + remediation) for NTP clock drift.
type DriftIssue struct{}

// NewDriftIssue creates a new NTP drift issue template.
func NewDriftIssue() *DriftIssue {
	return &DriftIssue{}
}

// BuildIssue creates a complete issue with metadata and platform-appropriate remediation.
func (t *DriftIssue) BuildIssue(context map[string]string) (*healthplatform.Issue, error) {
	drift := context[contextKeyDrift]
	if drift == "" {
		drift = "unknown"
	}
	ntpSrv := context[contextKeyNTPServer]
	if ntpSrv == "" {
		ntpSrv = "unknown"
	}
	threshold := context[contextKeyThreshold]
	if threshold == "" {
		threshold = "unknown"
	}

	extra, err := structpb.NewStruct(map[string]any{
		"drift":      drift,
		"ntp_server": ntpSrv,
		"threshold":  threshold,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to create issue extra: %w", err)
	}

	return &healthplatform.Issue{
		IssueName: DriftIssueName,
		Title:     "System Clock Drift Detected",
		Description: fmt.Sprintf(
			"The system clock is drifting from NTP reference time by %s, which exceeds the %s threshold. "+
				"Clock drift causes metric timestamps to be inaccurate, making it difficult to correlate events "+
				"across hosts and potentially triggering false anomaly alerts in Datadog. "+
				"Affected NTP server: %s.",
			drift, threshold, ntpSrv,
		),
		Category:   "integration",
		Location:   "system",
		Severity:   healthplatform.IssueSeverity_ISSUE_SEVERITY_MEDIUM,
		DetectedAt: "", // Filled by the health platform
		Source:     "ntp",
		Extra:      extra,
		Remediation: &healthplatform.Remediation{
			Summary: "Synchronise the system clock with an NTP server",
			Steps:   driftRemediationSteps(runtime.GOOS),
		},
		Tags: []string{"ntp", "clock-drift", "timestamps", runtime.GOOS},
	}, nil
}

// driftRemediationSteps returns platform-appropriate fix steps for osName (a runtime.GOOS value).
func driftRemediationSteps(osName string) []*healthplatform.RemediationStep {
	switch osName {
	case "windows":
		return []*healthplatform.RemediationStep{
			{Order: 1, Text: "Open an elevated Command Prompt or PowerShell."},
			{Order: 2, Text: "Trigger an immediate time synchronisation: w32tm /resync /force"},
			{Order: 3, Text: "Verify the offset: w32tm /query /status"},
			{Order: 4, Text: "Ensure the Windows Time service is running and set to automatic: sc config w32time start= auto && net start w32time"},
		}
	default: // linux, darwin
		return []*healthplatform.RemediationStep{
			{Order: 1, Text: "If using chrony (recommended): sudo chronyc makestep"},
			{Order: 2, Text: "If using ntpd: sudo ntpdate -u pool.ntp.org"},
			{Order: 3, Text: "Verify the time is now synchronised: chronyc tracking  OR  timedatectl status"},
			{Order: 4, Text: "Ensure an NTP daemon is enabled: sudo systemctl enable --now chronyd  OR  sudo systemctl enable --now ntp"},
			{Order: 5, Text: "If running in a VM, check that the hypervisor time-sync feature is enabled for your platform."},
		}
	}
}
