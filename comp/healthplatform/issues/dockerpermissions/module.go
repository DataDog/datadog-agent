// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

// Package dockerpermissions provides a complete issue module for Docker permission problems.
// It includes both detection (built-in health check) and remediation (issue template with fix scripts).
package dockerpermissions

import (
	"github.com/DataDog/datadog-agent/comp/core/config"
	"github.com/DataDog/datadog-agent/comp/healthplatform/issues"
)

func init() {
	issues.RegisterModuleFactory(NewModule)
}

const (
	// IssueType is the template identifier for Docker permission issues
	IssueType = "docker-file-tailing-disabled"

	// IssueID is the unique instance id used when reporting this issue
	IssueID = "docker-socket-permissions"
)

// dockerPermissionsModule implements issues.Module
type dockerPermissionsModule struct {
	template *DockerPermissionIssue
}

// NewModule creates a new Docker permissions issue module
func NewModule(config.Component) issues.Module {
	return &dockerPermissionsModule{
		template: NewDockerPermissionIssue(),
	}
}

func (m *dockerPermissionsModule) IssueType() string {
	return IssueType
}

func (m *dockerPermissionsModule) IssueTemplate() issues.IssueTemplate {
	return m.template
}

// BuiltInPeriodicHealthCheck returns the periodic health check configuration.
// Interval is 0 to use the default (15 minutes).
func (m *dockerPermissionsModule) BuiltInPeriodicHealthCheck() *issues.BuiltInPeriodicHealthCheck {
	return &issues.BuiltInPeriodicHealthCheck{
		Source: "docker",
		Fn:     Check,
	}
}

// BuiltInStartupHealthCheck returns nil — docker permission checks run periodically.
func (m *dockerPermissionsModule) BuiltInStartupHealthCheck() *issues.BuiltInStartupHealthCheck {
	return nil
}
