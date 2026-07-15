// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

// Package dockerpermissions provides a complete issue module for Docker permission problems.
// It includes both detection (built-in health check) and remediation (issue template with fix scripts).
package dockerpermissions

import (
	"github.com/DataDog/agent-payload/v5/healthplatform"
	"github.com/DataDog/datadog-agent/comp/healthplatform/issues"
	runnerdef "github.com/DataDog/datadog-agent/comp/healthplatform/runner/def"
)

func init() {
	issues.RegisterModuleFactory(NewModule)
}

const (
	// IssueName is the identifier for Docker permission issues,
	// used as the template registry key and the proto IssueName field.
	IssueName = "Docker File Tailing Disabled"

	// IssueType is the snake_case type key for Docker permission issues:
	// IssueName lowercased with spaces replaced by underscores.
	IssueType = "docker_file_tailing_disabled"

	// IssueID is the unique instance id used when reporting this issue
	IssueID = "docker-socket-permissions"
)

// dockerPermissionsModule implements issues.Module
type dockerPermissionsModule struct {
	template *DockerPermissionIssue
}

// NewModule creates a new Docker permissions issue module
func NewModule(issues.ModuleDeps) issues.Module {
	return &dockerPermissionsModule{
		template: NewDockerPermissionIssue(),
	}
}

func (m *dockerPermissionsModule) IssueName() string {
	return IssueName
}

func (m *dockerPermissionsModule) IssueType() string {
	return IssueType
}

func (m *dockerPermissionsModule) BuildIssue(context map[string]string) (*healthplatform.Issue, error) {
	return m.template.BuildIssue(context)
}

// BuiltInPeriodicHealthCheck returns the periodic health check configuration.
// Interval is 0 to use the default (15 minutes).
func (m *dockerPermissionsModule) BuiltInPeriodicHealthCheck() *runnerdef.BuiltInPeriodicHealthCheck {
	return &runnerdef.BuiltInPeriodicHealthCheck{
		BuiltInHealthCheck: runnerdef.BuiltInHealthCheck{
			Source: "docker",
			Fn:     Check,
		},
	}
}

// BuiltInStartupHealthCheck returns nil — docker permission checks run periodically.
func (m *dockerPermissionsModule) BuiltInStartupHealthCheck() *runnerdef.BuiltInHealthCheck {
	return nil
}
