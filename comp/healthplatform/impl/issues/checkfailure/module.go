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
	"github.com/DataDog/datadog-agent/comp/healthplatform/impl/issues"
)

func init() {
	issues.RegisterModuleFactory(NewModule)
}

const (
	// IssueID is the unique identifier for check failure issues
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

// IssueID returns the unique identifier for this issue type
func (m *checkFailureModule) IssueID() string {
	return IssueID
}

// IssueTemplate returns the template for building complete issues
func (m *checkFailureModule) IssueTemplate() issues.IssueTemplate {
	return m.template
}

// BuiltInCheck returns nil - check failures are reported by external integrations
func (m *checkFailureModule) BuiltInCheck() *issues.BuiltInCheck {
	return nil
}
