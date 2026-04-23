// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

package ntpdrift

import (
	"fmt"
	"runtime"

	"github.com/DataDog/agent-payload/v5/healthplatform"
	"google.golang.org/protobuf/types/known/structpb"
)

// NTPDriftIssue provides the complete issue template (metadata + remediation) for NTP clock drift.
type NTPDriftIssue struct{}

// NewNTPDriftIssue creates a new NTP drift issue template.
func NewNTPDriftIssue() *NTPDriftIssue {
	return &NTPDriftIssue{}
}

// BuildIssue creates a complete issue with metadata and platform-appropriate remediation.
func (t *NTPDriftIssue) BuildIssue(context map[string]string) (*healthplatform.Issue, error) {
	drift := context["drift"]
	if drift == "" {
		drift = "unknown"
	}
	ntpSrv := context["ntpServer"]
	if ntpSrv == "" {
		ntpSrv = ntpServer
	}

	extra, err := structpb.NewStruct(map[string]any{
		"drift":      drift,
		"ntp_server": ntpSrv,
		"threshold":  driftThreshold.String(),
	})
	if err != nil {
		return nil, fmt.Errorf("failed to create issue extra: %w", err)
	}

	return &healthplatform.Issue{
		Id:        IssueID,
		IssueName: "ntp_clock_drift",
		Title:     "System Clock Drift Detected",
		Description: fmt.Sprintf(
			"The system clock is drifting from NTP reference time by %s, which exceeds the %s threshold. "+
				"Clock drift causes metric timestamps to be inaccurate, making it difficult to correlate events "+
				"across hosts and potentially triggering false anomaly alerts in Datadog. "+
				"Affected NTP server: %s.",
			drift, driftThreshold, ntpSrv,
		),
		Category:   "configuration",
		Location:   "system",
		Severity:   "warning",
		DetectedAt: "", // Filled by the health platform
		Source:     "agent",
		Extra:      extra,
		Remediation: &healthplatform.Remediation{
			Summary: "Synchronise the system clock with an NTP server",
			Steps:   remediationSteps(),
		},
		Tags: []string{"ntp", "clock-drift", "timestamps", "configuration", runtime.GOOS},
	}, nil
}

// remediationSteps returns platform-appropriate fix steps.
func remediationSteps() []*healthplatform.RemediationStep {
	switch runtime.GOOS {
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
