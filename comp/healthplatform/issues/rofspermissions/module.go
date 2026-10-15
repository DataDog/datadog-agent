// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

//go:build docker

// Package rofspermissions provides a complete module for handling Read-Only Filesystem permission issues specifically
// checking if the Agent has write permissions to all the expected directories.
package rofspermissions

import (
	"context"

	"github.com/DataDog/agent-payload/v5/healthplatform"
	"github.com/DataDog/datadog-agent/comp/core/config"
	hostnameinterface "github.com/DataDog/datadog-agent/comp/core/hostname/hostnameinterface/def"
	"github.com/DataDog/datadog-agent/comp/healthplatform/issueregistry/utils/selfident"
	"github.com/DataDog/datadog-agent/comp/healthplatform/issues"
	runnerdef "github.com/DataDog/datadog-agent/comp/healthplatform/runner/def"
	"github.com/DataDog/datadog-agent/pkg/config/env"
	"github.com/DataDog/datadog-agent/pkg/util/flavor"
)

func init() {
	if env.IsContainerized() {
		issues.RegisterModuleFactory(NewModule)
	}
}

const (
	// IssueName is the identifier for ROFS permission issues,
	// used as the template registry key and the proto IssueName field.
	IssueName = "Read-Only Filesystem Error"

	// IssueType is the snake_case type key for ROFS permission issues:
	// IssueName lowercased with spaces replaced by underscores (hyphens preserved).
	IssueType = "read-only_filesystem_error"

	// IssueID is the issue ID prefix used when reporting this issue.
	// Check appends the reporting agent's resource_id: IssueID + ":" + resource_id.
	IssueID = "rofs-permissions"
)

type rofsPermissionsModule struct {
	template  *RofsPermissionIssue
	conf      config.Component
	hostname  hostnameinterface.Component
	selfIdent *selfident.SelfIdent
}

// NewModule creates a new ROFS permissions issue module
func NewModule(deps issues.ModuleDeps) issues.Module {
	return &rofsPermissionsModule{
		template:  NewRofsPermissionIssue(),
		conf:      deps.Config,
		hostname:  deps.Hostname,
		selfIdent: deps.SelfIdent,
	}
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
			return Check(r.conf, r.instanceIssueID)
		},
	}
}

// instanceIssueID scopes IssueID to the resource_id the outbound report
// carries, so agents on different resources don't all report the same id.
// The hostname is resolved the same way egress resolves it.
func (r *rofsPermissionsModule) instanceIssueID() string {
	var hostname string
	if r.hostname != nil {
		if h, err := r.hostname.Get(context.Background()); err == nil {
			hostname = h
		}
	}
	_, resourceID := issues.ResourceIdentity(flavor.GetFlavor(), r.selfIdent, hostname)
	return IssueID + ":" + resourceID
}
