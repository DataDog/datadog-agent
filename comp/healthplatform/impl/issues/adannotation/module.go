// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

// Package adannotation provides an issue module for autodiscovery annotation misconfigurations.
// This module only provides remediation (no built-in check) as annotation errors
// are reported by external integrations (the container config provider).
package adannotation

import (
	"github.com/DataDog/datadog-agent/comp/core/config"
	"github.com/DataDog/datadog-agent/comp/healthplatform/impl/issues"
)

func init() {
	issues.RegisterModuleFactory(NewModule)
}

const (
	// IssueID is the unique identifier for AD annotation misconfiguration issues
	IssueID = "ad-annotation-misconfiguration"
)

// adAnnotationModule implements issues.Module
type adAnnotationModule struct {
	template *ADAnnotationIssue
}

// NewModule creates a new AD annotation issue module
func NewModule(config.Component) issues.Module {
	return &adAnnotationModule{
		template: NewADAnnotationIssue(),
	}
}

// IssueID returns the unique identifier for this issue type
func (m *adAnnotationModule) IssueID() string {
	return IssueID
}

// IssueTemplate returns the template for building complete issues
func (m *adAnnotationModule) IssueTemplate() issues.IssueTemplate {
	return m.template
}

// BuiltInCheck returns nil - annotation errors are reported by the container config provider
func (m *adAnnotationModule) BuiltInCheck() *issues.BuiltInCheck {
	return nil
}
