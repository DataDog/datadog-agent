// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

// Package windowseventlogchannels provides an issue module for invalid Windows
// Event Log channel configurations. The issue is detected by the win32_event_log
// check itself when EvtSubscribe fails with ERROR_EVT_CHANNEL_NOT_FOUND.
package windowseventlogchannels

import (
	"fmt"

	"github.com/DataDog/agent-payload/v5/healthplatform"
	"google.golang.org/protobuf/types/known/structpb"
)

// WindowsEventLogChannelIssue provides the issue template for an invalid Windows Event Log channel.
type WindowsEventLogChannelIssue struct{}

// NewWindowsEventLogChannelIssue creates a new issue template.
func NewWindowsEventLogChannelIssue() *WindowsEventLogChannelIssue {
	return &WindowsEventLogChannelIssue{}
}

// BuildIssue creates a complete issue with metadata and remediation steps.
// Expected context keys:
//   - "channelPath": the configured channel name that was not found on the host
func (t *WindowsEventLogChannelIssue) BuildIssue(context map[string]string) (*healthplatform.Issue, error) {
	channelPath := context["channelPath"]
	if channelPath == "" {
		channelPath = "unknown"
	}

	extra, err := structpb.NewStruct(map[string]any{
		"channelPath": channelPath,
		"impact":      "Events from this channel will not be collected.",
	})
	if err != nil {
		return nil, fmt.Errorf("error building Windows Event Log channel issue: %w", err)
	}

	return &healthplatform.Issue{
		IssueName:   "windows_eventlog_channel_not_found",
		Title:       "Windows Event Log Channel Not Found",
		Description: fmt.Sprintf("The configured Windows Event Log channel %q does not exist on this host.", channelPath),
		Category:    "configuration",
		Location:    "win32-event-log",
		Severity:    "warning",
		Source:      "win32_event_log",
		Extra:       extra,
		Remediation: &healthplatform.Remediation{
			Summary: "Verify the channel name and update the integration configuration.",
			Steps: []*healthplatform.RemediationStep{
				{Order: 1, Text: "List available channels: run `wevtutil el` in PowerShell"},
				{Order: 2, Text: fmt.Sprintf("Verify that %q appears in the output", channelPath)},
				{Order: 3, Text: "Update `channel_path` in win32_event_log.d/conf.yaml to a valid channel name"},
				{Order: 4, Text: "Restart the Datadog Agent to apply the change"},
			},
		},
		Tags: []string{"windows-event-log", "configuration", "win32_event_log"},
	}, nil
}
