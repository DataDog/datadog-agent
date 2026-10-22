// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package outputs

import (
	"github.com/DataDog/datadog-agent/test/e2e-framework/components/datadog/agent"
	"github.com/DataDog/datadog-agent/test/e2e-framework/components/kubernetes"
)

// BenchmarkEKSOutputs is the interface for the outputs of the benchmarkeks scenario: a
// Kubernetes cluster with two installations of the Agent, of variants a and b.
type BenchmarkEKSOutputs interface {
	KubernetesClusterOutput() *kubernetes.ClusterOutput
	AgentAOutput() *agent.KubernetesAgentOutput
	AgentBOutput() *agent.KubernetesAgentOutput
	DisableAgents()
}

// BenchmarkEKS contains the outputs for a benchmarkeks environment.
type BenchmarkEKS struct {
	KubernetesCluster *kubernetes.ClusterOutput
	AgentA            *agent.KubernetesAgentOutput
	AgentB            *agent.KubernetesAgentOutput
}

// NewBenchmarkEKS creates a new BenchmarkEKS output struct with all fields initialized.
func NewBenchmarkEKS() *BenchmarkEKS {
	return &BenchmarkEKS{
		KubernetesCluster: &kubernetes.ClusterOutput{},
		AgentA:            &agent.KubernetesAgentOutput{},
		AgentB:            &agent.KubernetesAgentOutput{},
	}
}

// KubernetesClusterOutput returns the Kubernetes cluster output for exporting
func (b *BenchmarkEKS) KubernetesClusterOutput() *kubernetes.ClusterOutput {
	return b.KubernetesCluster
}

// AgentAOutput returns the output of the Agent of variant a for exporting (may be nil)
func (b *BenchmarkEKS) AgentAOutput() *agent.KubernetesAgentOutput {
	return b.AgentA
}

// AgentBOutput returns the output of the Agent of variant b for exporting (may be nil)
func (b *BenchmarkEKS) AgentBOutput() *agent.KubernetesAgentOutput {
	return b.AgentB
}

// DisableAgents marks both Agents as not provisioned (sets them to nil)
func (b *BenchmarkEKS) DisableAgents() {
	b.AgentA = nil
	b.AgentB = nil
}
