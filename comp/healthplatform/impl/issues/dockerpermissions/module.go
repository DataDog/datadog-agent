// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

// Package dockerpermissions provides a complete issue module for Docker permission problems.
// It includes both detection (built-in health check) and remediation (issue template with fix scripts).
package dockerpermissions

import (
	"github.com/DataDog/datadog-agent/comp/healthplatform/impl/issues"
)

func init() {
	issues.RegisterModuleFactory(NewModule)
}

const (
	// IssueID is the unique identifier for Docker permission issues
	IssueID = "docker-file-tailing-disabled"

	// CheckID is the unique identifier for the built-in health check
	CheckID = "docker-socket-permissions"

	// CheckName is the human-readable name for the health check
	CheckName = "Docker Socket Permissions"
)

// dockerPermissionsModule implements issues.Module
type dockerPermissionsModule struct {
	template *DockerPermissionIssue
}

// NewModule creates a new Docker permissions issue module
func NewModule() issues.Module {
	return &dockerPermissionsModule{
		template: NewDockerPermissionIssue(),
	}
}

// IssueID returns the unique identifier for this issue type
func (m *dockerPermissionsModule) IssueID() string {
	return IssueID
}

// IssueTemplate returns the template for building complete issues
func (m *dockerPermissionsModule) IssueTemplate() issues.IssueTemplate {
	return m.template
}

// BuiltInCheck returns the built-in health check configuration
// Interval is 0 to use the default (15 minutes)
func (m *dockerPermissionsModule) BuiltInCheck() *issues.BuiltInCheck {
	return &issues.BuiltInCheck{
		ID:      CheckID,
		Name:    CheckName,
		CheckFn: Check,
	}
}
