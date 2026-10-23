// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

//go:build docker

// Package rofspermissions provides a complete module for handling Read-Only Filesystem permission issues specifically
// checking if the Agent has write permissions to all the expected directories.
package rofspermissions

import (
	"github.com/DataDog/agent-payload/v5/healthplatform"

	"github.com/DataDog/datadog-agent/comp/core/config"
	compdef "github.com/DataDog/datadog-agent/comp/def"
	"github.com/DataDog/datadog-agent/comp/healthplatform/issues"
	runnerdef "github.com/DataDog/datadog-agent/comp/healthplatform/runner/def"
	"github.com/DataDog/datadog-agent/pkg/config/env"
	"github.com/DataDog/datadog-agent/pkg/util/fxutil"
)

// Requires defines the dependencies for the issue module.
type Requires struct {
	compdef.In
	Config config.Component
}

// Provides defines the issue modules contributed to the registry.
type Provides struct {
	compdef.Out
	Module issues.Module `group:"healthplatform_issue"`
}

// Module provides the issue modules to the health platform registry.
func Module() fxutil.Module {
	return fxutil.Component(fxutil.ProvideComponentConstructor(newModule))
}

const (
	// IssueName is the identifier for ROFS permission issues,
	// used as the template registry key and the proto IssueName field.
	IssueName = "Read-Only Filesystem Error"

	// IssueType is the snake_case type key for ROFS permission issues:
	// IssueName lowercased with spaces replaced by underscores (hyphens preserved).
	IssueType = "read-only_filesystem_error"

	// IssueID is the unique instance id used when reporting this issue
	IssueID = "rofs-permissions"
)

type rofsPermissionsModule struct {
	template *RofsPermissionIssue
	conf     config.Component
}

func newModule(reqs Requires) Provides {
	if !env.IsContainerized() {
		return Provides{}
	}
	return Provides{Module: &rofsPermissionsModule{template: NewRofsPermissionIssue(), conf: reqs.Config}}
}

func (r *rofsPermissionsModule) IssueName() string {
	return IssueName
}

func (r *rofsPermissionsModule) IssueType() string {
	return IssueType
}

func (r *rofsPermissionsModule) BuildIssue(context map[string]string) (*healthplatform.Issue, error) {
	return r.template.BuildIssue(context)
}

// BuiltInPeriodicHealthCheck returns nil — filesystem permission checks run once at startup, not periodically.
func (r *rofsPermissionsModule) BuiltInPeriodicHealthCheck() *runnerdef.BuiltInPeriodicHealthCheck {
	return nil
}

// BuiltInStartupHealthCheck runs the filesystem permission check once at agent startup.
func (r *rofsPermissionsModule) BuiltInStartupHealthCheck() *runnerdef.BuiltInHealthCheck {
	return &runnerdef.BuiltInHealthCheck{
		Source: "agent",
		Fn: func() ([]runnerdef.IssueReport, error) {
			return Check(r.conf)
		},
	}
}
