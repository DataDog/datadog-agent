// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

// Package invalidsysprobeconfig reports system-probe.yaml schema violations through the Agent Health Platform.
package invalidsysprobeconfig

import (
	"github.com/DataDog/agent-payload/v5/healthplatform"

	"github.com/DataDog/datadog-agent/comp/core/config"
	"github.com/DataDog/datadog-agent/comp/healthplatform/issues"
	runnerdef "github.com/DataDog/datadog-agent/comp/healthplatform/runner/def"
)

const (
	// IssueName is the human-readable issue name for system-probe configuration-schema violations.
	IssueName = "Invalid System-Probe Config"
	// IssueType is the snake_case type key for system-probe configuration-schema
	// violations: IssueName lowercased with spaces replaced by underscores (hyphens preserved).
	IssueType = "invalid_system-probe_config"
	// IssueID is the stable instance identifier / registry key (kebab-case).
	IssueID = "invalid-system-probe-config"
)

func init() {
	issues.RegisterModuleFactory(NewModule)
}

type invalidSysprobeConfigModule struct {
	datadog config.Component // holds the feature flag (system-probe config has no health_platform keys)
	checker *checker
}

// NewModule captures the configs so the once-only startup check can read them.
func NewModule(deps issues.ModuleDeps) issues.Module {
	return &invalidSysprobeConfigModule{datadog: deps.Config, checker: newChecker(deps.SysProbeConfig, deps.Hostname, deps.SelfIdent)}
}

func (m *invalidSysprobeConfigModule) IssueName() string {
	return IssueName
}

func (m *invalidSysprobeConfigModule) IssueType() string {
	return IssueType
}

func (m *invalidSysprobeConfigModule) BuildIssue(context map[string]string) (*healthplatform.Issue, error) {
	return InvalidSysprobeConfigIssue{}.BuildIssue(context)
}

// BuiltInPeriodicHealthCheck returns nil as schema validation runs only at startup
func (m *invalidSysprobeConfigModule) BuiltInPeriodicHealthCheck() *runnerdef.BuiltInPeriodicHealthCheck {
	return nil
}

// BuiltInStartupHealthCheck runs system-probe schema validation once at agent startup.
// It returns nil when system-probe config isn't bundled (e.g. commands that don't load it),
// so the bundle doesn't resolve a real persisted issue without ever validating.
func (m *invalidSysprobeConfigModule) BuiltInStartupHealthCheck() *runnerdef.BuiltInHealthCheck {
	if m.checker.cfg == nil {
		return nil
	}
	return &runnerdef.BuiltInHealthCheck{
		Source: "system-probe",
		Fn: func() ([]runnerdef.IssueReport, error) {
			if !m.datadog.GetBool("health_platform.invalidsysprobeconfig_check.enabled") {
				return nil, nil
			}
			return m.checker.Run()
		},
	}
}
