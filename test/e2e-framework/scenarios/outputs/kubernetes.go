// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package outputs

import (
	compout "github.com/DataDog/datadog-agent/test/e2e-framework/components/outputs"
)

// KubernetesOutputs is the interface for Kubernetes environment outputs.
// It is shared across different Kubernetes-based scenarios (EKS, KIND, etc.).
type KubernetesOutputs interface {
	KubernetesClusterOutput() *compout.ClusterOutput
	FakeIntakeOutput() *compout.FakeintakeOutput
	KubernetesAgentOutput() *compout.KubernetesAgentOutput
	DisableFakeIntake()
	DisableAgent()
}

// Kubernetes contains the outputs for a Kubernetes environment.
type Kubernetes struct {
	KubernetesCluster *compout.ClusterOutput
	FakeIntake        *compout.FakeintakeOutput
	Agent             *compout.KubernetesAgentOutput
}

// NewKubernetes creates a new Kubernetes output struct with all fields initialized.
func NewKubernetes() *Kubernetes {
	return &Kubernetes{
		KubernetesCluster: &compout.ClusterOutput{},
		FakeIntake:        &compout.FakeintakeOutput{},
		Agent:             &compout.KubernetesAgentOutput{},
	}
}

// KubernetesClusterOutput returns the Kubernetes cluster output for exporting
func (k *Kubernetes) KubernetesClusterOutput() *compout.ClusterOutput {
	return k.KubernetesCluster
}

// FakeIntakeOutput returns the fakeintake output for exporting (may be nil)
func (k *Kubernetes) FakeIntakeOutput() *compout.FakeintakeOutput {
	return k.FakeIntake
}

// KubernetesAgentOutput returns the Kubernetes agent output for exporting (may be nil)
func (k *Kubernetes) KubernetesAgentOutput() *compout.KubernetesAgentOutput {
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
