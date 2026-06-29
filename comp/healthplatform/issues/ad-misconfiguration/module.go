// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

// Package admisconfig provides an issue module for autodiscovery misconfigurations.
// This module only provides remediation (no built-in check) as annotation errors
// are reported by external integrations (the container config provider).
package admisconfig

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
	// IssueName is the human-readable issue name for autodiscovery misconfiguration issues.
	IssueName = "Autodiscovery Misconfiguration"
	// Source is the reporting component identifier used in health-platform issues.
	Source = "autodiscovery"
	// AnnotationIssueID is the IssueID prefix for AD annotation misconfiguration issues.
	// External reporters append a per-entity suffix: AnnotationIssueID + ":" + entityName
	AnnotationIssueID = "ad-annotation"
	// TemplateIssueID is the IssueID prefix for AD template resolution failure issues.
	// External reporters append name, service-id, and digest: TemplateIssueID + ":" + name + ":" + serviceID + ":" + digest
	TemplateIssueID = "ad-template"
)

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

func (m *adMisconfigurationModule) IssueName() string {
	return IssueName
}

func (m *adMisconfigurationModule) BuildIssue(context map[string]string) (*healthplatform.Issue, error) {
	return m.template.BuildIssue(context)
}

// BuiltInPeriodicHealthCheck returns nil - annotation errors are reported by the container config provider
func (m *adMisconfigurationModule) BuiltInPeriodicHealthCheck() *runnerdef.BuiltInPeriodicHealthCheck {
	return nil
}

// BuiltInStartupHealthCheck returns nil - no startup-time check for this module
func (m *adMisconfigurationModule) BuiltInStartupHealthCheck() *runnerdef.BuiltInHealthCheck {
	return nil
}
