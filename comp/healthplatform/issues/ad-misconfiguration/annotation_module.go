// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

// Package admisconfig provides issue modules for autodiscovery misconfigurations.
// It contains two modules: one for annotation errors and one for template resolution failures.
// Both modules only provide remediation (no built-in checks) as errors are reported by
// external integrations (the container config provider and config manager).
package admisconfig

import (
	"github.com/DataDog/agent-payload/v5/healthplatform"
	"github.com/DataDog/datadog-agent/comp/core/config"
	"github.com/DataDog/datadog-agent/comp/healthplatform/issues"
	runnerdef "github.com/DataDog/datadog-agent/comp/healthplatform/runner/def"
)

func init() {
	issues.RegisterModuleFactory(newAnnotationModule)
}

const (
	// AnnotationIssueName is the human-readable issue name for AD annotation misconfiguration issues.
	AnnotationIssueName = "Autodiscovery Annotation Misconfiguration"
	// AnnotationIssueID is the IssueID prefix for AD annotation misconfiguration issues.
	// External reporters append a per-entity suffix: AnnotationIssueID + ":" + entityName
	AnnotationIssueID = "ad-annotation"
	// Source is the reporting component identifier used in health-platform issues.
	Source = "autodiscovery"
)

type adAnnotationModule struct {
	template *ADAnnotationIssue
}

func newAnnotationModule(config.Component) issues.Module {
	return &adAnnotationModule{template: NewADAnnotationIssue()}
}

func (m *adAnnotationModule) IssueName() string {
	return AnnotationIssueName
}

func (m *adAnnotationModule) BuildIssue(context map[string]string) (*healthplatform.Issue, error) {
	return m.template.BuildIssue(context)
}

func (m *adAnnotationModule) BuiltInPeriodicHealthCheck() *runnerdef.BuiltInPeriodicHealthCheck {
	return nil
}

func (m *adAnnotationModule) BuiltInStartupHealthCheck() *runnerdef.BuiltInHealthCheck {
	return nil
}
