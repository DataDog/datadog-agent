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
	"github.com/DataDog/datadog-agent/comp/core/config"
	"github.com/DataDog/datadog-agent/comp/healthplatform/issues"
	runnerdef "github.com/DataDog/datadog-agent/comp/healthplatform/runner/def"
)

func init() {
	issues.RegisterModuleFactory(NewModule)
}

const (
	// IssueID is the unique instance id prefix used when reporting check failures.
	IssueID = "check-execution-failure"
)

// checkFailureModule implements issues.Module
type checkFailureModule struct {
	template *CheckFailureIssue
}

// NewModule creates a new check failure issue module
func NewModule(config.Component) issues.Module {
	return &checkFailureModule{
		template: NewCheckFailureIssue(),
	}
}

func (m *checkFailureModule) IssueName() string {
	return issueName
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
