// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package admissionprobe provides an issue module for admission webhook
// connectivity failures. This module only provides remediation (no built-in
// check) as probe failures are reported by the admission controller probe.
package admissionprobe

import (
	"github.com/DataDog/agent-payload/v5/healthplatform"
	"github.com/DataDog/datadog-agent/comp/healthplatform/issues"
	runnerdef "github.com/DataDog/datadog-agent/comp/healthplatform/runner/def"
)

func init() {
	issues.RegisterModuleFactory(NewModule)
}

const (
	// IssueName is the identifier for admission controller connectivity issues,
	// used as the template registry key and the proto IssueName field.
	IssueName = "Admission Controller Unreachable"
	// IssueType is the snake_case type key for admission controller connectivity
	// issues: IssueName lowercased with spaces replaced by underscores.
	IssueType = "admission_controller_unreachable"
	// IssueID is the unique instance id used when reporting this issue.
	// Note: kept separate from IssueName — probe.go and E2E tests use this value for issue.Id.
	IssueID = "admission-controller-connectivity-failure"
)

type admissionProbeModule struct {
	template *AdmissionProbeIssue
}

// NewModule creates a new admission probe issue module.
func NewModule(issues.ModuleDeps) issues.Module {
	return &admissionProbeModule{
		template: &AdmissionProbeIssue{},
	}
}

func (m *admissionProbeModule) IssueName() string {
	return IssueName
}

func (m *admissionProbeModule) IssueType() string {
	return IssueType
}

func (m *admissionProbeModule) BuildIssue(context map[string]string) (*healthplatform.Issue, error) {
	return m.template.BuildIssue(context)
}

// BuiltInPeriodicHealthCheck returns nil — probe failures are reported by the admission controller probe.
func (m *admissionProbeModule) BuiltInPeriodicHealthCheck() *runnerdef.BuiltInPeriodicHealthCheck {
	return nil
}

// BuiltInStartupHealthCheck returns nil — no startup-time check for this module.
func (m *admissionProbeModule) BuiltInStartupHealthCheck() *runnerdef.BuiltInHealthCheck {
	return nil
}
