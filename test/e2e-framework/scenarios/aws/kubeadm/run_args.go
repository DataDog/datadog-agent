// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

package kubeadm

import (
	"fmt"

	"github.com/DataDog/datadog-agent/test/e2e-framework/components/datadog/kubernetesagentparams"
	kubeComp "github.com/DataDog/datadog-agent/test/e2e-framework/components/kubernetes"
	"github.com/DataDog/datadog-agent/test/e2e-framework/components/os"
	"github.com/DataDog/datadog-agent/test/e2e-framework/resources/aws"
	"github.com/DataDog/datadog-agent/test/e2e-framework/scenarios/aws/ec2"
	"github.com/DataDog/datadog-agent/test/e2e-framework/scenarios/aws/fakeintake"
	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/utils/optional"
)

const defaultKubeadmName = "kubeadm"

// RunParams collects parameters for the kubeadm-on-VM scenario.
type RunParams struct {
	Name              string
	vmOptions         []ec2.VMOption
	agentOptions      []kubernetesagentparams.Option
	fakeintakeOptions []fakeintake.Option

	// deploySBOMWorkloads deploys the SBOM target workloads (node, golang-alpine,
	// ubi9, python) so the Agent has real container images to scan.
	deploySBOMWorkloads bool

	// containerRuntime selects the node CRI. The zero value is containerd.
	containerRuntime kubeComp.ContainerRuntime
}

// RunOption is an option for the kubeadm scenario.
type RunOption = func(*RunParams) error

// GetRunParams applies opts on top of the scenario defaults.
func GetRunParams(opts ...RunOption) *RunParams {
	p := &RunParams{
		Name:              defaultKubeadmName,
		vmOptions:         []ec2.VMOption{},
		agentOptions:      nil, // nil by default - Agent is only deployed when options are explicitly provided
		fakeintakeOptions: []fakeintake.Option{},
	}
	if err := optional.ApplyOptions(p, opts); err != nil {
		panic(fmt.Errorf("unable to apply RunOption, err: %w", err))
	}
	return p
}

// ParamsFromEnvironment maps the environment to default kubeadm scenario parameters.
func ParamsFromEnvironment(e aws.Environment) *RunParams {
	p := &RunParams{
		Name: defaultKubeadmName,
	}

	// This scenario targets RHEL 10 by default, but honors an explicit descriptor.
	osDesc := os.DescriptorFromString(e.InfraOSDescriptor(), os.RedHat10)
	p.vmOptions = append(p.vmOptions, ec2.WithOS(osDesc))

	if e.AgentDeploy() {
		p.agentOptions = append(p.agentOptions, kubernetesagentparams.WithNamespace("datadog"))
	}

	if e.AgentDeploy() && e.AgentUseFakeintake() {
		fi := []fakeintake.Option{fakeintake.WithMemory(2048)}
		if e.AgentUseDualShipping() {
			fi = append(fi, fakeintake.WithoutDDDevForwarding())
		}
		p.fakeintakeOptions = fi
	}

	p.deploySBOMWorkloads = e.TestingWorkloadDeploy()
	return p
}

// WithName sets the scenario name.
func WithName(name string) RunOption {
	return func(p *RunParams) error { p.Name = name; return nil }
}

// WithVMOptions sets VM options.
func WithVMOptions(opts ...ec2.VMOption) RunOption {
	return func(p *RunParams) error { p.vmOptions = append(p.vmOptions, opts...); return nil }
}

// WithAgentOptions sets agent options.
func WithAgentOptions(opts ...kubernetesagentparams.Option) RunOption {
	return func(p *RunParams) error { p.agentOptions = append(p.agentOptions, opts...); return nil }
}

// WithFakeintakeOptions sets fakeintake options.
func WithFakeintakeOptions(opts ...fakeintake.Option) RunOption {
	return func(p *RunParams) error { p.fakeintakeOptions = append(p.fakeintakeOptions, opts...); return nil }
}

// WithoutFakeIntake disables fakeintake creation.
func WithoutFakeIntake() RunOption {
	return func(p *RunParams) error { p.fakeintakeOptions = nil; return nil }
}

// WithDeploySBOMWorkloads deploys the SBOM target workloads.
func WithDeploySBOMWorkloads() RunOption {
	return func(p *RunParams) error { p.deploySBOMWorkloads = true; return nil }
}

// WithContainerRuntime selects the node container runtime (containerd by default, or CRI-O).
func WithContainerRuntime(rt kubeComp.ContainerRuntime) RunOption {
	return func(p *RunParams) error { p.containerRuntime = rt; return nil }
}
