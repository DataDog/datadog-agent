// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

// Package systemprobeunreachable provides a complete issue module for detecting when
// NPM or USM is enabled but system-probe is not running or its socket is not reachable.
package systemprobeunreachable

import (
	"github.com/DataDog/agent-payload/v5/healthplatform"
	"github.com/DataDog/datadog-agent/comp/core/config"
	"github.com/DataDog/datadog-agent/comp/healthplatform/issues"
	runnerdef "github.com/DataDog/datadog-agent/comp/healthplatform/runner/def"
)

func init() {
	issues.RegisterModuleFactory(NewModule)
}

const (
	// IssueName is the identifier for system-probe unreachable issues,
	// used as the template registry key and the proto IssueName field.
	IssueName = "system_probe_unreachable"

	// IssueID is the unique instance id used when reporting this issue
	IssueID = "system-probe-unreachable"
)

// systemProbeUnreachableModule implements issues.Module
type systemProbeUnreachableModule struct {
	template *SystemProbeUnreachableIssue
}

// NewModule creates a new system-probe unreachable issue module
func NewModule(_ config.Component) issues.Module {
	return &systemProbeUnreachableModule{
		template: NewSystemProbeUnreachableIssue(),
	}
}

func (m *systemProbeUnreachableModule) IssueName() string {
	return IssueName
}

func (m *systemProbeUnreachableModule) BuildIssue(context map[string]string) (*healthplatform.Issue, error) {
	return m.template.BuildIssue(context)
}

// BuiltInPeriodicHealthCheck returns nil — system-probe reachability is checked once at startup, not periodically.
func (m *systemProbeUnreachableModule) BuiltInPeriodicHealthCheck() *runnerdef.BuiltInPeriodicHealthCheck {
	return nil
}

// BuiltInStartupHealthCheck runs the system-probe reachability check once at agent startup.
func (m *systemProbeUnreachableModule) BuiltInStartupHealthCheck() *runnerdef.BuiltInHealthCheck {
	return &runnerdef.BuiltInHealthCheck{
		Source: "system-probe",
		Fn:     Check,
	}
}
