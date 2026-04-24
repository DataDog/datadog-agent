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
	"github.com/DataDog/datadog-agent/comp/healthplatform/impl/issues"
)

func init() {
	issues.RegisterModuleFactory(NewModule)
}

const (
	// IssueID is the unique identifier for system-probe unreachable issues
	IssueID = "system-probe-unreachable"

	// CheckID is the unique identifier for the built-in check
	CheckID = "system-probe-reachability"

	// CheckName is the human-readable name for the health check
	CheckName = "System Probe Reachability Check"
)

// systemProbeUnreachableModule implements issues.Module
type systemProbeUnreachableModule struct {
	template *SystemProbeUnreachableIssue
	conf     config.Component
}

// NewModule creates a new system-probe unreachable issue module
func NewModule(conf config.Component) issues.Module {
	return &systemProbeUnreachableModule{
		template: NewSystemProbeUnreachableIssue(),
		conf:     conf,
	}
}

// IssueID returns the unique identifier for this issue type
func (m *systemProbeUnreachableModule) IssueID() string {
	return IssueID
}

// IssueTemplate returns the template for building complete issues
func (m *systemProbeUnreachableModule) IssueTemplate() issues.IssueTemplate {
	return m.template
}

// BuiltInCheck returns the built-in health check configuration
func (m *systemProbeUnreachableModule) BuiltInCheck() *issues.BuiltInCheck {
	return &issues.BuiltInCheck{
		ID:   CheckID,
		Name: CheckName,
		CheckFn: func() (*healthplatform.IssueReport, error) {
			return Check(m.conf)
		},
		Once: true,
	}
}
