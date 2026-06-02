// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

package windowseventlogchannels

import (
	"fmt"

	"github.com/DataDog/agent-payload/v5/healthplatform"
	"google.golang.org/protobuf/types/known/structpb"
)

const (
	// IssueName is the registry key and proto IssueName field for Windows Event Log channel issues.
	IssueName = "windows_eventlog_channel_not_found"

	// IssueIDPrefix is the prefix used when building per-instance issue IDs.
	IssueIDPrefix = "windows-eventlog-channel-not-found"
)

// WindowsEventLogChannelsIssue provides the issue template for missing Windows Event Log channels.
type WindowsEventLogChannelsIssue struct{}

// NewWindowsEventLogChannelsIssue creates a new Windows Event Log channels issue template.
func NewWindowsEventLogChannelsIssue() *WindowsEventLogChannelsIssue {
	return &WindowsEventLogChannelsIssue{}
}

// GetIssueName returns the registry key for this issue type.
func (t *WindowsEventLogChannelsIssue) GetIssueName() string { return IssueName }

// BuildIssue creates a complete issue with metadata and remediation steps.
func (t *WindowsEventLogChannelsIssue) BuildIssue(context map[string]string) (*healthplatform.Issue, error) {
	channelPath := context["channelPath"]
	if channelPath == "" {
		channelPath = "unknown"
	}

	extra, err := structpb.NewStruct(map[string]any{
		"channelPath": channelPath,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to create issue extra: %w", err)
	}

	return &healthplatform.Issue{
		Id:          IssueIDPrefix,
		IssueName:   IssueName,
		Title:       "Windows Event Log Channel Not Found",
		Description: fmt.Sprintf("The Windows Event Log integration is configured with channel %q which does not exist on this host. Events from this channel will not be collected.", channelPath),
		Category:    "configuration",
		Location:    "win32-event-log",
		Severity:    healthplatform.IssueSeverity_ISSUE_SEVERITY_MEDIUM,
		Source:      "integrations",
		Extra:       extra,
		Remediation: &healthplatform.Remediation{
			Summary: "Verify that the channel_path in the Windows Event Log integration configuration exists on this host.",
			Steps: []*healthplatform.RemediationStep{
				{Order: 1, Text: "List all available channels: wevtutil el"},
				{Order: 2, Text: fmt.Sprintf("Confirm channel %q exists in the list above", channelPath)},
				{Order: 3, Text: "Common built-in channels: System, Application, Security, Microsoft-Windows-PowerShell/Operational"},
				{Order: 4, Text: fmt.Sprintf("Fix the channel_path in win32_event_log.d/conf.yaml for %q", channelPath)},
			},
		},
		Tags: []string{"windows-event-log", "configuration", "win32_event_log"},
	}, nil
}
