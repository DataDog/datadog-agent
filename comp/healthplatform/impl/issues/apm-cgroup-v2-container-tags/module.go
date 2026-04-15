// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

// Package apmcgroupv2 detects when APM trace container ID resolution may fail due to cgroup v2.
package apmcgroupv2

import "github.com/DataDog/datadog-agent/comp/healthplatform/impl/issues"

func init() { issues.RegisterModuleFactory(NewModule) }

const (
	// IssueID is the unique identifier for APM cgroup v2 container tag issues
	IssueID = "apm-cgroup-v2-container-tags-missing"

	// CheckID is the unique identifier for the built-in health check
	CheckID = "apm-cgroup-v2-container-id"

	// CheckName is the human-readable name for the health check
	CheckName = "APM Container ID Resolution on cgroup v2"
)

// apmCgroupV2Module implements issues.Module
type apmCgroupV2Module struct {
	template *Issue
}

// NewModule creates a new APM cgroup v2 container tags issue module
func NewModule() issues.Module {
	return &apmCgroupV2Module{
		template: NewIssue(),
	}
}

// IssueID returns the unique identifier for this issue type
func (m *apmCgroupV2Module) IssueID() string {
	return IssueID
}

// IssueTemplate returns the template for building complete issues
func (m *apmCgroupV2Module) IssueTemplate() issues.IssueTemplate {
	return m.template
}

// BuiltInCheck returns nil: the cgroup v2 issue is detected in-workflow by the trace
// agent's container ID provider (pkg/trace/api) via ReportIssue(), not by a background poll.
func (m *apmCgroupV2Module) BuiltInCheck() *issues.BuiltInCheck {
	return nil
}
