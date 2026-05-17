// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package outputs

import (
	"github.com/DataDog/datadog-agent/test/e2e-framework/components/datadog/agent"
	"github.com/DataDog/datadog-agent/test/e2e-framework/components/datadog/fakeintake"
	"github.com/DataDog/datadog-agent/test/e2e-framework/components/kubernetes"
)

// KubernetesOutputs is the interface for Kubernetes environment outputs.
// It is shared across different Kubernetes-based scenarios (EKS, KIND, etc.).
type KubernetesOutputs interface {
	KubernetesClusterOutput() *kubernetes.ClusterOutput
	FakeIntakeOutput() *fakeintake.FakeintakeOutput
	KubernetesAgentOutput() *agent.KubernetesAgentOutput
	DisableFakeIntake()
	DisableAgent()
}

// Kubernetes contains the outputs for a Kubernetes environment.
type Kubernetes struct {
	KubernetesCluster *kubernetes.ClusterOutput
	FakeIntake        *fakeintake.FakeintakeOutput
	Agent             *agent.KubernetesAgentOutput
}

// NewKubernetes creates a new Kubernetes output struct with all fields initialized.
func NewKubernetes() *Kubernetes {
	return &Kubernetes{
		KubernetesCluster: &kubernetes.ClusterOutput{},
		FakeIntake:        &fakeintake.FakeintakeOutput{},
		Agent:             &agent.KubernetesAgentOutput{},
	}
}

// KubernetesClusterOutput returns the Kubernetes cluster output for exporting
func (k *Kubernetes) KubernetesClusterOutput() *kubernetes.ClusterOutput {
	return k.KubernetesCluster
}

// FakeIntakeOutput returns the fakeintake output for exporting (may be nil)
func (k *Kubernetes) FakeIntakeOutput() *fakeintake.FakeintakeOutput {
	return k.FakeIntake
}

// KubernetesAgentOutput returns the Kubernetes agent output for exporting (may be nil)
func (k *Kubernetes) KubernetesAgentOutput() *agent.KubernetesAgentOutput {
	return k.Agent
}

// DisableFakeIntake marks FakeIntake as not provisioned (sets to nil)
func (k *Kubernetes) DisableFakeIntake() {
	k.FakeIntake = nil
}

// DisableAgent marks Agent as not provisioned (sets to nil)
func (k *Kubernetes) DisableAgent() {
	k.Agent = nil
}
