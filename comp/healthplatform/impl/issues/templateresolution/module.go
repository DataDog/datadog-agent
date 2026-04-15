// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

// Package templateresolution provides an issue module for autodiscovery template
// resolution failures. This module only provides remediation (no built-in check)
// as template resolution failures are reported by the autodiscovery component.
package templateresolution

import (
	"github.com/DataDog/datadog-agent/comp/core/config"
	"github.com/DataDog/datadog-agent/comp/healthplatform/impl/issues"
)

func init() {
	issues.RegisterModuleFactory(NewModule)
}

const (
	// IssueID is the unique identifier for template resolution failure issues
	IssueID = "autodiscovery-template-resolution-failure"
)

// templateResolutionModule implements issues.Module
type templateResolutionModule struct {
	template *TemplateResolutionIssue
}

// NewModule creates a new template resolution issue module
func NewModule(config.Component) issues.Module {
	return &templateResolutionModule{
		template: NewTemplateResolutionIssue(),
	}
}

// IssueID returns the unique identifier for this issue type
func (m *templateResolutionModule) IssueID() string {
	return IssueID
}

// IssueTemplate returns the template for building complete issues
func (m *templateResolutionModule) IssueTemplate() issues.IssueTemplate {
	return m.template
}

// BuiltInCheck returns nil - template resolution failures are reported by autodiscovery
func (m *templateResolutionModule) BuiltInCheck() *issues.BuiltInCheck {
	return nil
}
