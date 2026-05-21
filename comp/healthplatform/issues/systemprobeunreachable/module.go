// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

// Package systemprobeunreachable provides a complete issue module for detecting when
// NPM or USM is enabled but system-probe is not running or its socket is not reachable.
package systemprobeunreachable

import (
	"github.com/DataDog/datadog-agent/comp/core/config"
	"github.com/DataDog/datadog-agent/comp/healthplatform/issues"
	storedef "github.com/DataDog/datadog-agent/comp/healthplatform/store/def"
)

func init() {
	issues.RegisterModuleFactory(NewModule)
}

const (
	// IssueType is the template type identifier for system-probe unreachable issues
	IssueType = "system-probe-unreachable"

	// IssueID is the unique instance id used when reporting this issue
	IssueID = "system-probe-unreachable"
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

// IssueType returns the template type identifier for this issue type
func (m *systemProbeUnreachableModule) IssueType() string {
	return IssueType
}

// IssueTemplate returns the template for building complete issues
func (m *systemProbeUnreachableModule) IssueTemplate() issues.IssueTemplate {
	return m.template
}

// BuiltInPeriodicHealthCheck returns nil — system-probe reachability is checked once at startup, not periodically.
func (m *systemProbeUnreachableModule) BuiltInPeriodicHealthCheck() *issues.BuiltInPeriodicHealthCheck {
	return nil
}

// BuiltInStartupHealthCheck runs the system-probe reachability check once at agent startup.
func (m *systemProbeUnreachableModule) BuiltInStartupHealthCheck() *issues.BuiltInStartupHealthCheck {
	return &issues.BuiltInStartupHealthCheck{
		Source: "system-probe",
		Fn: func() ([]storedef.IssueReport, error) {
			return Check(m.conf)
		},
	}
}
