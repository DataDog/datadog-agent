// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

//go:build linux

// Package pingicmppermissions provides a complete issue module for ping ICMP socket permission problems.
// It includes both detection (built-in health check) and remediation (issue template with fix steps).
package pingicmppermissions

import (
	"github.com/DataDog/datadog-agent/comp/core/config"
	"github.com/DataDog/datadog-agent/comp/healthplatform/impl/issues"
)

func init() {
	issues.RegisterModuleFactory(NewModule)
}

const (
	// IssueID is the unique identifier for ping ICMP permission issues
	IssueID = "ping-icmp-permissions"

	// CheckID is the unique identifier for the built-in health check
	CheckID = "ping-icmp-socket"

	// CheckName is the human-readable name for the health check
	CheckName = "Ping ICMP Socket Permissions"
)

// pingICMPPermissionsModule implements issues.Module
type pingICMPPermissionsModule struct {
	template *PingICMPPermissionsIssue
}

// NewModule creates a new ping ICMP permissions issue module
func NewModule(config.Component) issues.Module {
	return &pingICMPPermissionsModule{
		template: NewPingICMPPermissionsIssue(),
	}
}

// IssueID returns the unique identifier for this issue type
func (m *pingICMPPermissionsModule) IssueID() string {
	return IssueID
}

// IssueTemplate returns the template for building complete issues
func (m *pingICMPPermissionsModule) IssueTemplate() issues.IssueTemplate {
	return m.template
}

// BuiltInCheck returns the built-in health check configuration
func (m *pingICMPPermissionsModule) BuiltInCheck() *issues.BuiltInCheck {
	return &issues.BuiltInCheck{
		ID:      CheckID,
		Name:    CheckName,
		CheckFn: Check,
		Once:    true,
	}
}
