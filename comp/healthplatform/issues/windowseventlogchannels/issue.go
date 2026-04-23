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
	// IssueName is the registry key and the proto IssueName field for Windows Event Log channel issues.
	IssueName = "windows_eventlog_channel_not_found"

	// IssueID is the unique instance ID used when reporting this issue.
	IssueID = "windows-eventlog-channel-not-found"
)

// WindowsEventLogChannelsIssue provides the issue template for missing Windows Event Log channels.
type WindowsEventLogChannelsIssue struct{}

// NewWindowsEventLogChannelsIssue creates a new Windows Event Log channels issue template.
func NewWindowsEventLogChannelsIssue() *WindowsEventLogChannelsIssue {
	return &WindowsEventLogChannelsIssue{}
}

// IssueName returns the registry key for this issue type.
func (t *WindowsEventLogChannelsIssue) IssueName() string { return IssueName }

// BuildIssue creates a complete issue with metadata and remediation steps.
func (t *WindowsEventLogChannelsIssue) BuildIssue(context map[string]string) (*healthplatform.Issue, error) {
	invalidChannels := context["invalidChannels"]
	if invalidChannels == "" {
		invalidChannels = "unknown"
	}

	configFile := context["configFile"]
	if configFile == "" {
		configFile = "win32_event_log.d/conf.yaml"
	}

	extra, err := structpb.NewStruct(map[string]any{
		"invalidChannels": invalidChannels,
		"configFile":      configFile,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to create issue extra: %w", err)
	}

	return &healthplatform.Issue{
		Id:          IssueID,
		IssueName:   IssueName,
		Title:       "Windows Event Log Channels Not Found",
		Description: fmt.Sprintf("The Windows Event Log integration is configured with channel(s) that do not exist: %s. Events from these channels will not be collected.", invalidChannels),
		Category:    "configuration",
		Location:    "win32-event-log",
		Severity:    healthplatform.IssueSeverity_ISSUE_SEVERITY_MEDIUM,
		Source:      "integrations",
		Extra:       extra,
		Remediation: &healthplatform.Remediation{
			Summary: "Verify that the channel_path values in the Windows Event Log integration configuration match channels that exist on this host.",
			Steps: []*healthplatform.RemediationStep{
				{Order: 1, Text: "List all available channels on this host: wevtutil el"},
				{Order: 2, Text: fmt.Sprintf("Verify channel names in conf.d/%s", configFile)},
				{Order: 3, Text: "Common built-in channels: System, Application, Security, Microsoft-Windows-PowerShell/Operational"},
				{Order: 4, Text: fmt.Sprintf("Fix any typos in the channel_path values for: %s", invalidChannels)},
			},
		},
		Tags: []string{"windows-event-log", "configuration", "win32_event_log"},
	}, nil
}
