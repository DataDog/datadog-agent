// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

// Package silentlogpipeline provides an issue module for log sources that are
// configured but not producing logs.
// This module only provides remediation (no built-in check) as silent pipelines
// are reported by external integrations (the logs agent) when a source has been
// inactive beyond a threshold.
package silentlogpipeline

import (
	"github.com/DataDog/datadog-agent/comp/core/config"
	"github.com/DataDog/datadog-agent/comp/healthplatform/impl/issues"
)

func init() {
	issues.RegisterModuleFactory(NewModule)
}

const (
	// IssueID is the unique identifier for silent log pipeline issues
	IssueID = "silent-log-pipeline"
)

// silentLogPipelineModule implements issues.Module
type silentLogPipelineModule struct {
	template *SilentLogPipelineIssue
}

// NewModule creates a new silent log pipeline issue module
func NewModule(config.Component) issues.Module {
	return &silentLogPipelineModule{
		template: NewSilentLogPipelineIssue(),
	}
}

// IssueID returns the unique identifier for this issue type
func (m *silentLogPipelineModule) IssueID() string {
	return IssueID
}

// IssueTemplate returns the template for building complete issues
func (m *silentLogPipelineModule) IssueTemplate() issues.IssueTemplate {
	return m.template
}

// BuiltInCheck returns nil - silent pipelines are reported by the logs agent
func (m *silentLogPipelineModule) BuiltInCheck() *issues.BuiltInCheck {
	return nil
}
