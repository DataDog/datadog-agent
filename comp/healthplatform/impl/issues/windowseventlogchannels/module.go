// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

//go:build windows

// Package windowseventlogchannels provides an issue module for Windows Event Log channel misconfiguration.
// It detects configured channel_path values in win32_event_log integrations that do not exist on the host.
package windowseventlogchannels

import (
	"github.com/DataDog/agent-payload/v5/healthplatform"
	"github.com/DataDog/datadog-agent/comp/core/config"
	"github.com/DataDog/datadog-agent/comp/healthplatform/impl/issues"
)

func init() {
	issues.RegisterModuleFactory(NewModule)
}

const (
	// IssueID is the unique identifier for Windows Event Log channel not-found issues
	IssueID = "windows-eventlog-channel-not-found"

	// CheckID is the unique identifier for the built-in health check
	CheckID = "windows-eventlog-channels"

	// CheckName is the human-readable name for the health check
	CheckName = "Windows Event Log Channels"
)

// windowsEventLogChannelsModule implements issues.Module
type windowsEventLogChannelsModule struct {
	template *WindowsEventLogChannelsIssue
	conf     config.Component
}

// NewModule creates a new Windows Event Log channels issue module
func NewModule(conf config.Component) issues.Module {
	return &windowsEventLogChannelsModule{
		template: NewWindowsEventLogChannelsIssue(),
		conf:     conf,
	}
}

// IssueID returns the unique identifier for this issue type
func (m *windowsEventLogChannelsModule) IssueID() string {
	return IssueID
}

// IssueTemplate returns the template for building complete issues
func (m *windowsEventLogChannelsModule) IssueTemplate() issues.IssueTemplate {
	return m.template
}

// BuiltInCheck returns the built-in health check configuration.
// Once is true so this check runs only once at startup.
func (m *windowsEventLogChannelsModule) BuiltInCheck() *issues.BuiltInCheck {
	return &issues.BuiltInCheck{
		ID:   CheckID,
		Name: CheckName,
		CheckFn: func() (*healthplatform.IssueReport, error) {
			return Check(m.conf)
		},
		Once: true,
	}
}
