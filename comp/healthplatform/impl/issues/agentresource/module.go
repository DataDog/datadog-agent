// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

// Package agentresource provides a health platform issue module for agent high resource usage.
// The issue is detected externally by the agentprofiling core check and reported via ReportIssue.
package agentresource

import (
	"github.com/DataDog/datadog-agent/comp/core/config"
	"github.com/DataDog/datadog-agent/comp/healthplatform/impl/issues"
)

func init() {
	issues.RegisterModuleFactory(NewModule)
}

const (
	// IssueID is the unique identifier for agent high resource usage issues
	IssueID = "agent-high-resource-usage"
)

// agentResourceModule implements issues.Module
type agentResourceModule struct {
	template *AgentResourceIssue
}

// NewModule creates a new agent resource issue module
func NewModule(_ config.Component) issues.Module {
	return &agentResourceModule{
		template: NewAgentResourceIssue(),
	}
}

// IssueID returns the unique identifier for this issue type
func (m *agentResourceModule) IssueID() string {
	return IssueID
}

// IssueTemplate returns the template for building complete issues
func (m *agentResourceModule) IssueTemplate() issues.IssueTemplate {
	return m.template
}

// BuiltInCheck returns nil — this issue is reported externally by the agentprofiling core check
func (m *agentResourceModule) BuiltInCheck() *issues.BuiltInCheck {
	return nil
}
