// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package admissionprobe provides an issue module for admission webhook
// connectivity failures. This module only provides remediation (no built-in
// check) as probe failures are reported by the admission controller probe.
package admissionprobe

import (
	"github.com/DataDog/datadog-agent/comp/healthplatform/impl/issues"
)

func init() {
	issues.RegisterModuleFactory(NewModule)
}

const (
	// IssueID is the unique identifier for admission controller connectivity issues.
	IssueID = "admission-controller-connectivity-failure"
)

type admissionProbeModule struct {
	template *AdmissionProbeIssue
}

// NewModule creates a new admission probe issue module.
func NewModule() issues.Module {
	return &admissionProbeModule{
		template: &AdmissionProbeIssue{},
	}
}

func (m *admissionProbeModule) IssueID() string {
	return IssueID
}

func (m *admissionProbeModule) IssueTemplate() issues.IssueTemplate {
	return m.template
}

// BuiltInCheck returns nil — probe failures are reported by the admission controller probe.
func (m *admissionProbeModule) BuiltInCheck() *issues.BuiltInCheck {
	return nil
}
