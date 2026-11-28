// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package missingclcrunners provides an issue module reported when the Cluster
// Agent is dispatching cluster checks but no Cluster Check Runners are
// reporting. This module only provides remediation (no built-in check): the
// condition is detected by the cluster-checks dispatcher, which reports and
// resolves the issue directly.
package missingclcrunners

import (
	"github.com/DataDog/agent-payload/v5/healthplatform"
	"github.com/DataDog/datadog-agent/comp/healthplatform/issues"
	runnerdef "github.com/DataDog/datadog-agent/comp/healthplatform/runner/def"
)

func init() {
	issues.RegisterModuleFactory(NewModule)
}

const (
	// IssueName is the identifier for the missing cluster check runners issue,
	// used as the template registry key and the proto IssueName field.
	IssueName = "Missing Cluster Check Runners"
	// IssueType is the snake_case type key: IssueName lowercased with spaces
	// replaced by underscores.
	IssueType = "missing_cluster_check_runners"
	// IssueID is the unique instance id used when reporting this issue.
	IssueID = "missing-cluster-check-runners"
)

type missingCLCRunnersModule struct {
	template *MissingCLCRunnersIssue
}

// NewModule creates a new missing cluster check runners issue module.
func NewModule(issues.ModuleDeps) issues.Module {
	return &missingCLCRunnersModule{
		template: &MissingCLCRunnersIssue{},
	}
}

func (m *missingCLCRunnersModule) IssueName() string {
	return IssueName
}

func (m *missingCLCRunnersModule) IssueType() string {
	return IssueType
}

func (m *missingCLCRunnersModule) BuildIssue(context map[string]string) (*healthplatform.Issue, error) {
	return m.template.BuildIssue(context)
}

// BuiltInPeriodicHealthCheck returns nil — the condition is detected and
// reported by the cluster-checks dispatcher.
func (m *missingCLCRunnersModule) BuiltInPeriodicHealthCheck() *runnerdef.BuiltInPeriodicHealthCheck {
	return nil
}

// BuiltInStartupHealthCheck returns nil — no startup-time check for this module.
func (m *missingCLCRunnersModule) BuiltInStartupHealthCheck() *runnerdef.BuiltInHealthCheck {
	return nil
}
