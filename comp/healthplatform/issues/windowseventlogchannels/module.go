// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

package windowseventlogchannels

import (
	"github.com/DataDog/datadog-agent/comp/core/config"
	"github.com/DataDog/datadog-agent/comp/healthplatform/issues"
)

func init() {
	issues.RegisterModuleFactory(NewModule)
}

const (
	// IssueID is the unique type identifier for Windows Event Log channel-not-found issues.
	IssueID = "windows-eventlog-channel-not-found"
)

type windowsEventLogChannelModule struct {
	template *WindowsEventLogChannelIssue
}

// NewModule creates a new Windows Event Log channel issue module.
func NewModule(config.Component) issues.Module {
	return &windowsEventLogChannelModule{
		template: NewWindowsEventLogChannelIssue(),
	}
}

func (m *windowsEventLogChannelModule) IssueID() string {
	return IssueID
}

func (m *windowsEventLogChannelModule) IssueTemplate() issues.IssueTemplate {
	return m.template
}

// BuiltInHealthCheck returns nil — the issue is reported by the win32_event_log
// check itself when EvtSubscribe fails with ERROR_EVT_CHANNEL_NOT_FOUND.
func (m *windowsEventLogChannelModule) BuiltInHealthCheck() *issues.BuiltInHealthCheck {
	return nil
}
