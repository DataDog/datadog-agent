// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

//go:build windows

// Package windowseventlogchannels provides the issue template for Windows Event Log
// channels that are configured but do not exist on the host.
package windowseventlogchannels

import (
	"github.com/DataDog/agent-payload/v5/healthplatform"
	"github.com/DataDog/datadog-agent/comp/core/config"
	"github.com/DataDog/datadog-agent/comp/healthplatform/issues"
	runnerdef "github.com/DataDog/datadog-agent/comp/healthplatform/runner/def"
)

func init() {
	issues.RegisterModuleFactory(NewModule)
}

type windowsEventLogChannelsModule struct{}

// NewModule creates a new Windows Event Log channels issue module.
func NewModule(_ config.Component) issues.Module {
	return &windowsEventLogChannelsModule{}
}

func (m *windowsEventLogChannelsModule) IssueName() string { return IssueName }

func (m *windowsEventLogChannelsModule) BuildIssue(context map[string]string) (*healthplatform.Issue, error) {
	return NewWindowsEventLogChannelsIssue().BuildIssue(context)
}

// BuiltInPeriodicHealthCheck returns nil — detection happens inline in the check.
func (m *windowsEventLogChannelsModule) BuiltInPeriodicHealthCheck() *runnerdef.BuiltInPeriodicHealthCheck {
	return nil
}

// BuiltInStartupHealthCheck returns nil — detection happens inline in the check.
func (m *windowsEventLogChannelsModule) BuiltInStartupHealthCheck() *runnerdef.BuiltInHealthCheck {
	return nil
}
