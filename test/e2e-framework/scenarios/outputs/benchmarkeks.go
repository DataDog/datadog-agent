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
// Kubernetes cluster with a baseline and a comparison installation of the Agent.
type BenchmarkEKSOutputs interface {
	KubernetesClusterOutput() *kubernetes.ClusterOutput
	BaselineAgentOutput() *agent.KubernetesAgentOutput
	ComparisonAgentOutput() *agent.KubernetesAgentOutput
	DisableAgents()
}

// BenchmarkEKS contains the outputs for a benchmarkeks environment.
type BenchmarkEKS struct {
	KubernetesCluster *kubernetes.ClusterOutput
	BaselineAgent     *agent.KubernetesAgentOutput
	ComparisonAgent   *agent.KubernetesAgentOutput
}

// NewBenchmarkEKS creates a new BenchmarkEKS output struct with all fields initialized.
func NewBenchmarkEKS() *BenchmarkEKS {
	return &BenchmarkEKS{
		KubernetesCluster: &kubernetes.ClusterOutput{},
		BaselineAgent:     &agent.KubernetesAgentOutput{},
		ComparisonAgent:   &agent.KubernetesAgentOutput{},
	}
}

// KubernetesClusterOutput returns the Kubernetes cluster output for exporting
func (b *BenchmarkEKS) KubernetesClusterOutput() *kubernetes.ClusterOutput {
	return b.KubernetesCluster
}

// BaselineAgentOutput returns the baseline Agent output for exporting (may be nil)
func (b *BenchmarkEKS) BaselineAgentOutput() *agent.KubernetesAgentOutput {
	return b.BaselineAgent
}

// ComparisonAgentOutput returns the comparison Agent output for exporting (may be nil)
func (b *BenchmarkEKS) ComparisonAgentOutput() *agent.KubernetesAgentOutput {
	return b.ComparisonAgent
}

// DisableAgents marks both Agents as not provisioned (sets them to nil)
func (b *BenchmarkEKS) DisableAgents() {
	b.BaselineAgent = nil
	b.ComparisonAgent = nil
}
