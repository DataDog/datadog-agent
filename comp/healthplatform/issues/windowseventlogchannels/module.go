// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

//go:build windows

// Package windowseventlogchannels detects configured Windows Event Log channel_path values
// in win32_event_log integrations that do not exist on the host.
package windowseventlogchannels

import (
	"github.com/DataDog/agent-payload/v5/healthplatform"
	"github.com/DataDog/datadog-agent/comp/core/config"
	"github.com/DataDog/datadog-agent/comp/healthplatform/issues"
	runnerdef "github.com/DataDog/datadog-agent/comp/healthplatform/runner/def"
)

func init() {
	issues.RegisterModuleFactory(NewModule)
}

type windowsEventLogChannelsModule struct {
	conf config.Component
}

// NewModule creates a new Windows Event Log channels issue module.
func NewModule(conf config.Component) issues.Module {
	return &windowsEventLogChannelsModule{conf: conf}
}

func (m *windowsEventLogChannelsModule) IssueName() string { return IssueName }

func (m *windowsEventLogChannelsModule) BuildIssue(context map[string]string) (*healthplatform.Issue, error) {
	return NewWindowsEventLogChannelsIssue().BuildIssue(context)
}

// BuiltInPeriodicHealthCheck returns nil — this check runs once at startup only.
func (m *windowsEventLogChannelsModule) BuiltInPeriodicHealthCheck() *runnerdef.BuiltInPeriodicHealthCheck {
	return nil
}

// BuiltInStartupHealthCheck runs the Windows Event Log channel check once at agent startup.
func (m *windowsEventLogChannelsModule) BuiltInStartupHealthCheck() *runnerdef.BuiltInHealthCheck {
	return &runnerdef.BuiltInHealthCheck{
		Source: "win32_event_log",
		Fn: func() ([]runnerdef.IssueReport, error) {
			report, err := Check(m.conf)
			if err != nil || report == nil {
				return nil, err
			}
			return []runnerdef.IssueReport{
				{
					IssueID:   IssueID,
					IssueName: IssueName,
					Source:    "win32_event_log",
					Context:   report.Context,
					Tags:      report.Tags,
				},
			}, nil
		},
	}
}
