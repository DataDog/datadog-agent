// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

// Package checkfailure provides an issue module for check execution failures.
// This module only provides remediation (no built-in check) as check failures
// are reported by external integrations (the collector).
package checkfailure

import (
	"github.com/DataDog/datadog-agent/comp/core/config"
	"github.com/DataDog/datadog-agent/comp/healthplatform/issues"
)

func init() {
	issues.RegisterModuleFactory(NewModule)
}

const (
	// IssueID is the snake_case issue name for check failures, used as the
	// template registry key and the proto IssueName field.
	IssueID = "check_execution_failure"
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
	return IssueID
}

// IssueTemplate returns the template for building complete issues
func (m *checkFailureModule) IssueTemplate() issues.IssueTemplate {
	return m.template
}

// BuiltInPeriodicHealthCheck returns nil - check failures are reported by external integrations
func (m *checkFailureModule) BuiltInPeriodicHealthCheck() *issues.BuiltInPeriodicHealthCheck {
	return nil
}

// BuiltInStartupHealthCheck returns nil - no startup-time check for this module
func (m *checkFailureModule) BuiltInStartupHealthCheck() *issues.BuiltInStartupHealthCheck {
	return nil
}
