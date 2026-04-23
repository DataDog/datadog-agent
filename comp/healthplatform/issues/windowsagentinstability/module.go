// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

//go:build windows

// Package windowsagentinstability provides a complete issue module for Windows agent service instability.
// It detects repeated Datadog Agent service crashes or restarts on Windows at startup.
package windowsagentinstability

import (
	"github.com/DataDog/agent-payload/v5/healthplatform"
	"github.com/DataDog/datadog-agent/comp/core/config"
	"github.com/DataDog/datadog-agent/comp/healthplatform/issues"
	runnerdef "github.com/DataDog/datadog-agent/comp/healthplatform/runner/def"
)

func init() {
	issues.RegisterModuleFactory(NewModule)
}

const (
	// CheckID is the unique identifier for the built-in health check.
	CheckID = "windows-agent-crashes"
)

type windowsAgentInstabilityModule struct {
	template *WindowsAgentInstabilityIssue
}

// NewModule creates a new Windows agent instability issue module.
func NewModule(_ config.Component) issues.Module {
	return &windowsAgentInstabilityModule{
		template: NewWindowsAgentInstabilityIssue(),
	}
}

func (m *windowsAgentInstabilityModule) IssueName() string {
	return IssueName
}

func (m *windowsAgentInstabilityModule) BuildIssue(context map[string]string) (*healthplatform.Issue, error) {
	return m.template.BuildIssue(context)
}

func (m *windowsAgentInstabilityModule) BuiltInPeriodicHealthCheck() *runnerdef.BuiltInPeriodicHealthCheck {
	return nil
}

func (m *windowsAgentInstabilityModule) BuiltInStartupHealthCheck() *runnerdef.BuiltInHealthCheck {
	return &runnerdef.BuiltInHealthCheck{
		Source: "agent",
		Fn: func() ([]runnerdef.IssueReport, error) {
			report, err := Check()
			if err != nil || report == nil {
				return nil, err
			}
			return []runnerdef.IssueReport{
				{
					IssueID:   IssueID,
					IssueName: IssueName,
					Source:    "agent",
					Context:   report.Context,
				},
			}, nil
		},
	}
}
