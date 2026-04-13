// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

// Package admisconfig provides an issue module for autodiscovery misconfigurations.
// This module only provides remediation (no built-in check) as annotation errors
// are reported by external integrations (the container config provider).
package admisconfig

import (
	"github.com/DataDog/datadog-agent/comp/core/config"
	healthplatform "github.com/DataDog/datadog-agent/comp/healthplatform/def"
	"github.com/DataDog/datadog-agent/comp/healthplatform/impl/issues"
)

func init() {
	issues.RegisterModuleFactory(NewModule)
}

// adMisconfigurationModule implements issues.Module
type adMisconfigurationModule struct {
	template *ADMisconfigurationIssue
}

// NewModule creates a new AD annotation issue module
func NewModule(config.Component) issues.Module {
	return &adMisconfigurationModule{
		template: NewADMisconfigurationIssue(),
	}
}

// IssueID returns the unique identifier for this issue type
func (m *adMisconfigurationModule) IssueID() string {
	return healthplatform.ADMisconfigurationIssueID
}

// IssueTemplate returns the template for building complete issues
func (m *adMisconfigurationModule) IssueTemplate() issues.IssueTemplate {
	return m.template
}

// BuiltInCheck returns nil - annotation errors are reported by the container config provider
func (m *adMisconfigurationModule) BuiltInCheck() *issues.BuiltInCheck {
	return nil
}
