// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

//go:build linux

// Package rofspermissions provides a complete module for handling Read-Only Filesystem permission issues specifically
// checking if the Agent as write permissions to all the expected directories.
package rofspermissions

import (
	"github.com/DataDog/agent-payload/v5/healthplatform"
	"github.com/DataDog/datadog-agent/comp/core/config"
	"github.com/DataDog/datadog-agent/comp/healthplatform/impl/issues"
)

func init() {
	issues.RegisterModuleFactory(NewModule)
}

const (
	// IssueID is the unique identifier for ROFS permission issues
	IssueID = "read-only-filesystem-error"

	// CheckID is the unique identifier for the built-in check
	CheckID = "rofs-permissions"

	// CheckName is the human-readable name for the health check
	CheckName = "ROFS Permissions Check"
)

type rofsPermissionsModule struct {
	template *RofsPermissionIssue
	conf     config.Component
}

func NewModule(conf config.Component) issues.Module {
	return &rofsPermissionsModule{
		template: NewRofsPermissionIssue(),
		conf:     conf,
	}
}

func (r *rofsPermissionsModule) IssueID() string {
	return IssueID
}

func (r *rofsPermissionsModule) IssueTemplate() issues.IssueTemplate {
	return r.template
}

func (r *rofsPermissionsModule) BuiltInCheck() *issues.BuiltInCheck {
	return &issues.BuiltInCheck{
		ID:   CheckID,
		Name: CheckName,
		CheckFn: func() (*healthplatform.IssueReport, error) {
			return Check(r.conf)
		},
		// TODO: No way to run once in the current state. Investigate the addition of a 'once'
		// option to run a check just at startup.
		Interval: 0,
	}
}
