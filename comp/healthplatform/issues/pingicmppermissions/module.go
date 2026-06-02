// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

//go:build linux

// Package pingicmppermissions detects when the Datadog agent lacks the NET_RAW capability
// required to create raw ICMP sockets for the ping integration.
package pingicmppermissions

import (
	"github.com/DataDog/agent-payload/v5/healthplatform"
	"github.com/DataDog/datadog-agent/comp/core/config"
	"github.com/DataDog/datadog-agent/comp/healthplatform/issues"
	runnerdef "github.com/DataDog/datadog-agent/comp/healthplatform/runner/def"
)

func init() {
	issues.RegisterModuleFactory(NewModule)
}

type pingICMPPermissionsModule struct{}

// NewModule creates a new ping ICMP permissions issue module.
func NewModule(_ config.Component) issues.Module {
	return &pingICMPPermissionsModule{}
}

func (m *pingICMPPermissionsModule) IssueName() string { return IssueName }

func (m *pingICMPPermissionsModule) BuildIssue(context map[string]string) (*healthplatform.Issue, error) {
	return (&PingICMPPermissionsIssue{}).BuildIssue(context)
}

// BuiltInPeriodicHealthCheck returns nil — this check runs once at startup only.
func (m *pingICMPPermissionsModule) BuiltInPeriodicHealthCheck() *runnerdef.BuiltInPeriodicHealthCheck {
	return nil
}

// BuiltInStartupHealthCheck runs the raw ICMP socket check once at agent startup.
func (m *pingICMPPermissionsModule) BuiltInStartupHealthCheck() *runnerdef.BuiltInHealthCheck {
	return &runnerdef.BuiltInHealthCheck{
		Source: "ping",
		Fn:     check,
	}
}
