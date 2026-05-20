// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

//go:build docker

// Package rofspermissions provides a complete module for handling Read-Only Filesystem permission issues specifically
// checking if the Agent has write permissions to all the expected directories.
package rofspermissions

import (
	"github.com/DataDog/datadog-agent/comp/core/config"
	"github.com/DataDog/datadog-agent/comp/healthplatform/issues"
	runnerdef "github.com/DataDog/datadog-agent/comp/healthplatform/runner/def"
	"github.com/DataDog/datadog-agent/pkg/config/env"
)

func init() {
	if env.IsContainerized() {
		issues.RegisterModuleFactory(NewModule)
	}
}

const (
	// IssueType is the template identifier for ROFS permission issues
	IssueType = "read-only-filesystem-error"

	// IssueID is the unique instance id used when reporting this issue
	IssueID = "rofs-permissions"
)

type rofsPermissionsModule struct {
	template *RofsPermissionIssue
	conf     config.Component
}

// NewModule creates a new ROFS permissions issue module
func NewModule(conf config.Component) issues.Module {
	return &rofsPermissionsModule{
		template: NewRofsPermissionIssue(),
		conf:     conf,
	}
}

func (r *rofsPermissionsModule) IssueType() string {
	return IssueType
}

func (r *rofsPermissionsModule) IssueTemplate() issues.IssueTemplate {
	return r.template
}

// BuiltInPeriodicHealthCheck returns nil — filesystem permission checks run once at startup, not periodically.
func (r *rofsPermissionsModule) BuiltInPeriodicHealthCheck() *issues.BuiltInPeriodicHealthCheck {
	return nil
}

// BuiltInStartupHealthCheck runs the filesystem permission check once at agent startup.
func (r *rofsPermissionsModule) BuiltInStartupHealthCheck() *issues.BuiltInStartupHealthCheck {
	return &issues.BuiltInStartupHealthCheck{
		Source: "agent",
		Fn: func() ([]runnerdef.IssueReport, error) {
			return Check(r.conf)
		},
	}
}
