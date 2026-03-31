// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

// Package dockerlogsrotation provides a health check for Docker container log rotation issues.
// It detects when the agent is collecting Docker logs via socket-based tailing while
// log rotation is enabled, which can cause log gaps or complete collection failures.
package dockerlogsrotation

import "github.com/DataDog/datadog-agent/comp/healthplatform/impl/issues"

func init() {
	issues.RegisterModuleFactory(NewModule)
}

const (
	// IssueID is the unique identifier for Docker log rotation risk issues
	IssueID = "docker-logs-rotation-risk"

	// CheckID is the unique identifier for the built-in health check
	CheckID = "docker-logs-rotation-config"

	// CheckName is the human-readable name for the health check
	CheckName = "Docker Container Log Rotation Configuration"
)

// module implements issues.Module
type module struct {
	template *Issue
}

// NewModule creates a new Docker logs rotation issue module
func NewModule() issues.Module {
	return &module{
		template: NewIssue(),
	}
}

// IssueID returns the unique identifier for this issue type
func (m *module) IssueID() string {
	return IssueID
}

// IssueTemplate returns the template for building complete issues
func (m *module) IssueTemplate() issues.IssueTemplate {
	return m.template
}

// BuiltInCheck returns the built-in health check configuration
// Interval is 0 to use the default (15 minutes)
func (m *module) BuiltInCheck() *issues.BuiltInCheck {
	return &issues.BuiltInCheck{
		ID:      CheckID,
		Name:    CheckName,
		CheckFn: Check,
	}
}
