// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

// Package networkprobefailure provides an issue module for NPM/USM eBPF probe
// initialization failures. Detection is performed by system-probe itself (which
// reports via ReportHealthIssue gRPC); this module only registers the remediation
// template used by the core agent's health platform registry.
package networkprobefailure

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
	// IssueName is the identifier for network probe initialization failures,
	// used as the template registry key and the proto IssueName field.
	IssueName = "network_probe_init_failure"

	// IssueID is the unique instance id used when reporting this issue.
	// There is at most one network tracer per system-probe instance.
	IssueID = "network-probe-init-failure"
)

type networkProbeFailureModule struct {
	template *NetworkProbeFailureIssue
}

// NewModule creates a new network probe failure issue module.
func NewModule(config.Component) issues.Module {
	return &networkProbeFailureModule{template: NewNetworkProbeFailureIssue()}
}

func (m *networkProbeFailureModule) IssueName() string {
	return IssueName
}

func (m *networkProbeFailureModule) BuildIssue(context map[string]string) (*healthplatform.Issue, error) {
	return m.template.BuildIssue(context)
}

// BuiltInPeriodicHealthCheck returns nil — detection is done by system-probe via gRPC.
func (m *networkProbeFailureModule) BuiltInPeriodicHealthCheck() *runnerdef.BuiltInPeriodicHealthCheck {
	return nil
}

// BuiltInStartupHealthCheck returns nil — detection is done by system-probe via gRPC.
func (m *networkProbeFailureModule) BuiltInStartupHealthCheck() *runnerdef.BuiltInHealthCheck {
	return nil
}
