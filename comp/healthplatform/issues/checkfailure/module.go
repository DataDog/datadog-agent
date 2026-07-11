// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

// Package checkfailure provides an issue module for check execution failures.
// This module only provides remediation (no built-in check) as check failures
// are reported by external integrations (the collector).
package checkfailure

import (
	"github.com/DataDog/agent-payload/v5/healthplatform"
	"github.com/DataDog/datadog-agent/comp/healthplatform/issues"
	runnerdef "github.com/DataDog/datadog-agent/comp/healthplatform/runner/def"
)

func init() {
	issues.RegisterModuleFactory(NewModule)
}

const (
	// IssueName is the human-readable issue name for check execution failures.
	IssueName = "Check Execution Failure"
	// IssueID is the unique instance id prefix used when reporting check failures (kebab-case).
	IssueID = "check-execution-failure"
)

// checkFailureModule implements issues.Module
type checkFailureModule struct {
	template *CheckFailureIssue
}

// NewModule creates a new check failure issue module
func NewModule(issues.ModuleDeps) issues.Module {
	return &checkFailureModule{
		template: NewCheckFailureIssue(),
	}
}

func (m *checkFailureModule) IssueName() string {
	return IssueName
}

func (m *checkFailureModule) BuildIssue(context map[string]string) (*healthplatform.Issue, error) {
	return m.template.BuildIssue(context)
}

// BuiltInPeriodicHealthCheck returns nil - check failures are reported by external integrations
func (m *checkFailureModule) BuiltInPeriodicHealthCheck() *runnerdef.BuiltInPeriodicHealthCheck {
	return nil
}

// BuiltInStartupHealthCheck returns nil - no startup-time check for this module
func (m *checkFailureModule) BuiltInStartupHealthCheck() *runnerdef.BuiltInHealthCheck {
	return nil
}
