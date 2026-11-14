// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

// Package clusteragentunreachable provides an issue module for node agents
// (and cluster check runners) that cannot connect to the Datadog Cluster Agent.
// The Cluster Agent clients record network-level outcomes in
// pkg/util/clusteragent/connectivity; a periodic check turns a sustained
// outage into one issue per host. Nothing is recorded unless a feature talks
// to the Cluster Agent, so agents that don't use it never report the issue.
package clusteragentunreachable

import (
	"time"

	"github.com/DataDog/agent-payload/v5/healthplatform"

	"github.com/DataDog/datadog-agent/comp/healthplatform/issues"
	runnerdef "github.com/DataDog/datadog-agent/comp/healthplatform/runner/def"
	"github.com/DataDog/datadog-agent/pkg/util/clusteragent/connectivity"
)

func init() {
	issues.RegisterModuleFactory(NewModule)
}

const (
	// IssueName is the identifier for Cluster Agent connectivity issues,
	// used as the template registry key and the proto IssueName field.
	IssueName = "Cluster Agent Unreachable"
	// IssueType is IssueName lowercased with spaces replaced by underscores.
	IssueType = "cluster_agent_unreachable"
	// IssueID is the instance id prefix; the check appends a per-host digest.
	IssueID = "cluster-agent-unreachable"
)

// checkSource must be unique: bundle.go only warns when Schedule rejects a dupe.
const checkSource = "cluster-agent-connectivity"

const (
	checkInterval = 30 * time.Second
	// failureThreshold absorbs Cluster Agent rollouts and leader changes, which
	// the cluster checks provider already tolerates for its 60s grace period.
	failureThreshold = time.Minute
)

type clusterAgentUnreachableModule struct {
	checker *checker
}

// NewModule creates the Cluster Agent unreachable issue module.
func NewModule(deps issues.ModuleDeps) issues.Module {
	return &clusterAgentUnreachableModule{checker: newChecker(connectivity.Default, deps.Hostname)}
}

func (m *clusterAgentUnreachableModule) IssueName() string {
	return IssueName
}

func (m *clusterAgentUnreachableModule) IssueType() string {
	return IssueType
}

func (m *clusterAgentUnreachableModule) BuildIssue(context map[string]string) (*healthplatform.Issue, error) {
	return ClusterAgentUnreachableIssue{}.BuildIssue(context)
}

// BuiltInPeriodicHealthCheck runs well under the scheduler's 15-minute default
// so the issue tracks the connection state closely.
func (m *clusterAgentUnreachableModule) BuiltInPeriodicHealthCheck() *runnerdef.BuiltInPeriodicHealthCheck {
	return &runnerdef.BuiltInPeriodicHealthCheck{
		BuiltInHealthCheck: runnerdef.BuiltInHealthCheck{
			Source: checkSource,
			Fn:     m.checker.Check,
		},
		Interval: checkInterval,
	}
}

// BuiltInStartupHealthCheck returns nil: connectivity can change while the agent runs.
func (m *clusterAgentUnreachableModule) BuiltInStartupHealthCheck() *runnerdef.BuiltInHealthCheck {
	return nil
}
