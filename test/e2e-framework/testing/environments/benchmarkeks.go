// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package environments

import (
	"github.com/DataDog/datadog-agent/test/e2e-framework/components/datadog/agent"
	"github.com/DataDog/datadog-agent/test/e2e-framework/components/kubernetes"
	"github.com/DataDog/datadog-agent/test/e2e-framework/scenarios/outputs"
	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/components"
)

// BenchmarkEKS is the environment of the benchmarkeks scenario: an EKS cluster with two
// installations of the Agent, of variants a and b, each in its own namespace and pinned to
// its own node pools. Use WaitForKubernetesAgentReady to wait for either of them.
type BenchmarkEKS struct {
	// Components
	KubernetesCluster *components.KubernetesCluster
	AgentA            *components.KubernetesAgent
	AgentB            *components.KubernetesAgent
}

// Ensure BenchmarkEKS implements the BenchmarkEKSOutputs interface
var _ outputs.BenchmarkEKSOutputs = (*BenchmarkEKS)(nil)

// KubernetesClusterOutput implements outputs.BenchmarkEKSOutputs
func (e *BenchmarkEKS) KubernetesClusterOutput() *kubernetes.ClusterOutput {
	if e.KubernetesCluster == nil {
		e.KubernetesCluster = &components.KubernetesCluster{}
	}
	return &e.KubernetesCluster.ClusterOutput
}

// AgentAOutput implements outputs.BenchmarkEKSOutputs
func (e *BenchmarkEKS) AgentAOutput() *agent.KubernetesAgentOutput {
	if e.AgentA == nil {
		e.AgentA = &components.KubernetesAgent{}
	}
	return &e.AgentA.KubernetesAgentOutput
}

// AgentBOutput implements outputs.BenchmarkEKSOutputs
func (e *BenchmarkEKS) AgentBOutput() *agent.KubernetesAgentOutput {
	if e.AgentB == nil {
		e.AgentB = &components.KubernetesAgent{}
	}
	return &e.AgentB.KubernetesAgentOutput
}

// DisableAgents implements outputs.BenchmarkEKSOutputs
func (e *BenchmarkEKS) DisableAgents() {
	e.AgentA = nil
	e.AgentB = nil
}
