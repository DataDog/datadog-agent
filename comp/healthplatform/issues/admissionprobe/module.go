// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package admissionprobe provides an issue module for admission webhook
// connectivity failures. This module only provides remediation (no built-in
// check) as probe failures are reported by the admission controller probe.
package admissionprobe

import (
	"github.com/DataDog/datadog-agent/comp/core/config"
	"github.com/DataDog/datadog-agent/comp/healthplatform/issues"
)

func init() {
	issues.RegisterModuleFactory(NewModule)
}

const (
	// IssueType is the template type identifier for admission controller connectivity issues.
	IssueType = "admission-controller-connectivity-failure"
	// IssueID is the unique instance id used when reporting this issue.
	IssueID = IssueType
)

type admissionProbeModule struct {
	template *AdmissionProbeIssue
}

// NewModule creates a new admission probe issue module.
func NewModule(config.Component) issues.Module {
	return &admissionProbeModule{
		template: &AdmissionProbeIssue{},
	}
}

func (m *admissionProbeModule) IssueType() string {
	return IssueType
}

func (m *admissionProbeModule) IssueTemplate() issues.IssueTemplate {
	return m.template
}

// BuiltInPeriodicHealthCheck returns nil — probe failures are reported by the admission controller probe.
func (m *admissionProbeModule) BuiltInPeriodicHealthCheck() *issues.BuiltInPeriodicHealthCheck {
	return nil
}

// BuiltInStartupHealthCheck returns nil — no startup-time check for this module.
func (m *admissionProbeModule) BuiltInStartupHealthCheck() *issues.BuiltInStartupHealthCheck {
	return nil
}
