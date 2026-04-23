// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

//go:build windows

// Package windowsagentinstability provides a complete issue module for Windows agent service instability.
// It detects repeated Datadog Agent service crashes or restarts on Windows at startup.
package windowsagentinstability

import (
	"github.com/DataDog/datadog-agent/comp/core/config"
	"github.com/DataDog/datadog-agent/comp/healthplatform/impl/issues"
)

func init() {
	issues.RegisterModuleFactory(NewModule)
}

const (
	// IssueID is the unique identifier for Windows agent instability issues
	IssueID = "windows-agent-service-instability"

	// CheckID is the unique identifier for the built-in health check
	CheckID = "windows-agent-crashes"

	// CheckName is the human-readable name for the health check
	CheckName = "Windows Agent Service Crash Check"
)

// windowsAgentInstabilityModule implements issues.Module
type windowsAgentInstabilityModule struct {
	template *WindowsAgentInstabilityIssue
}

// NewModule creates a new Windows agent instability issue module
func NewModule(config.Component) issues.Module {
	return &windowsAgentInstabilityModule{
		template: NewWindowsAgentInstabilityIssue(),
	}
}

// IssueID returns the unique identifier for this issue type
func (m *windowsAgentInstabilityModule) IssueID() string {
	return IssueID
}

// IssueTemplate returns the template for building complete issues
func (m *windowsAgentInstabilityModule) IssueTemplate() issues.IssueTemplate {
	return m.template
}

// BuiltInCheck returns the built-in health check configuration.
// Once is true because this is a startup check — it reads recent crash history at agent start.
func (m *windowsAgentInstabilityModule) BuiltInCheck() *issues.BuiltInCheck {
	return &issues.BuiltInCheck{
		ID:      CheckID,
		Name:    CheckName,
		CheckFn: Check,
		Once:    true,
	}
}
