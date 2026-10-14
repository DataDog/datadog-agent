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

// BenchmarkEKS is the environment of the benchmarkeks scenario: an EKS cluster with a
// baseline and a comparison installation of the Agent, each in its own namespace and
// pinned to its own node pools. Use WaitForKubernetesAgentReady to wait for either of them.
type BenchmarkEKS struct {
	// Components
	KubernetesCluster *components.KubernetesCluster
	BaselineAgent     *components.KubernetesAgent
	ComparisonAgent   *components.KubernetesAgent
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

// BaselineAgentOutput implements outputs.BenchmarkEKSOutputs
func (e *BenchmarkEKS) BaselineAgentOutput() *agent.KubernetesAgentOutput {
	if e.BaselineAgent == nil {
		e.BaselineAgent = &components.KubernetesAgent{}
	}
	return &e.BaselineAgent.KubernetesAgentOutput
}

// ComparisonAgentOutput implements outputs.BenchmarkEKSOutputs
func (e *BenchmarkEKS) ComparisonAgentOutput() *agent.KubernetesAgentOutput {
	if e.ComparisonAgent == nil {
		e.ComparisonAgent = &components.KubernetesAgent{}
	}
	return &e.ComparisonAgent.KubernetesAgentOutput
}

// DisableAgents implements outputs.BenchmarkEKSOutputs
func (e *BenchmarkEKS) DisableAgents() {
	e.BaselineAgent = nil
	e.ComparisonAgent = nil
}
