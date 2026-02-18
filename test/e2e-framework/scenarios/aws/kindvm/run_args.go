// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

package kindvm

import (
	"fmt"

	"github.com/DataDog/datadog-agent/test/e2e-framework/components/datadog/agentwithoperatorparams"
	"github.com/DataDog/datadog-agent/test/e2e-framework/components/datadog/kubernetesagentparams"
	"github.com/DataDog/datadog-agent/test/e2e-framework/components/datadog/operatorparams"
	kubecomp "github.com/DataDog/datadog-agent/test/e2e-framework/components/kubernetes"
	"github.com/DataDog/datadog-agent/test/e2e-framework/components/kubernetes/cilium"
	"github.com/DataDog/datadog-agent/test/e2e-framework/components/os"
	"github.com/DataDog/datadog-agent/test/e2e-framework/resources/aws"
	"github.com/DataDog/datadog-agent/test/e2e-framework/scenarios/aws/ec2"
	"github.com/DataDog/datadog-agent/test/e2e-framework/scenarios/aws/fakeintake"
	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/utils/optional"
)

const defaultKindName = "kind"
const csiDriverCommitSHA = "d91af776a15382b030035129e3b93dc8620d787e"

// RunParams collects parameters for the Kind-on-VM scenario
type RunParams struct {
	Name                string
	vmOptions           []ec2.VMOption
	agentOptions        []kubernetesagentparams.Option
	fakeintakeOptions   []fakeintake.Option
	ciliumOptions       []cilium.Option
	workloadAppFuncs    []kubecomp.WorkloadAppFunc
	depWorkloadAppFuncs []kubecomp.AgentDependentWorkloadAppFunc

	deployOperator     bool
	operatorDDAOptions []agentwithoperatorparams.Option
	operatorOptions    []operatorparams.Option
	deployDogstatsd    bool
	deployTestWorkload bool
	deployArgoRollout  bool
}

type RunOption = func(*RunParams) error

func GetRunParams(opts ...RunOption) *RunParams {
	p := &RunParams{
		Name:                defaultKindName,
		vmOptions:           []ec2.VMOption{},
		agentOptions:        nil, // nil by default - Agent is only deployed when options are explicitly provided
		fakeintakeOptions:   []fakeintake.Option{},
		workloadAppFuncs:    []kubecomp.WorkloadAppFunc{},
		depWorkloadAppFuncs: []kubecomp.AgentDependentWorkloadAppFunc{},
		operatorOptions:     []operatorparams.Option{},
		operatorDDAOptions:  nil, // nil by default - DDA is only deployed when options are explicitly provided
		deployDogstatsd:     false,
		deployOperator:      false,
	}
	if err := optional.ApplyOptions(p, opts); err != nil {
		panic(fmt.Errorf("unable to apply RunOption, err: %w", err))
	}
	return p
}

// ParamsFromEnvironment maps the environment to default Kind scenario parameters
func ParamsFromEnvironment(e aws.Environment) *RunParams {
	p := &RunParams{
		Name: defaultKindName,
	}

	// VM: pick OS from InfraOSDescriptor
	osDesc := os.DescriptorFromString(e.InfraOSDescriptor(), os.AmazonLinuxECSDefault)
	p.vmOptions = append(p.vmOptions, ec2.WithOS(osDesc))

	// Agent defaults
	if e.AgentDeploy() && !e.AgentDeployWithOperator() {
		p.agentOptions = append(p.agentOptions, kubernetesagentparams.WithNamespace("datadog"))
	}

	// Fakeintake
	if e.AgentDeploy() && e.AgentUseFakeintake() {
		fi := []fakeintake.Option{fakeintake.WithMemory(2048)}
		if e.InfraShouldDeployFakeintakeWithLB() {
			fi = append(fi, fakeintake.WithLoadBalancer())
		}
		if e.AgentUseDualShipping() {
			fi = append(fi, fakeintake.WithoutDDDevForwarding())
		}
		if retention := e.AgentFakeintakeRetentionPeriod(); retention != "" {
			fi = append(fi, fakeintake.WithRetentionPeriod(retention))
		}
		p.fakeintakeOptions = fi
	}

	p.deployOperator = e.AgentDeployWithOperator()
	p.deployDogstatsd = e.DogstatsdDeploy()
	p.deployTestWorkload = e.TestingWorkloadDeploy()
	return p
}

// WithName sets scenario name
func WithName(name string) RunOption { return func(p *RunParams) error { p.Name = name; return nil } }

// WithVMOptions sets VM options
func WithVMOptions(opts ...ec2.VMOption) RunOption {
	return func(p *RunParams) error { p.vmOptions = append(p.vmOptions, opts...); return nil }
}

// WithAgentOptions sets agent options
func WithAgentOptions(opts ...kubernetesagentparams.Option) RunOption {
	return func(p *RunParams) error { p.agentOptions = append(p.agentOptions, opts...); return nil }
}

// WithFakeintakeOptions sets fakeintake options
func WithFakeintakeOptions(opts ...fakeintake.Option) RunOption {
	return func(p *RunParams) error { p.fakeintakeOptions = append(p.fakeintakeOptions, opts...); return nil }
}

// WithoutFakeIntake disables fakeintake creation
func WithoutFakeIntake() RunOption {
	return func(p *RunParams) error { p.fakeintakeOptions = nil; return nil }
}

// WithCiliumOptions sets cilium options
func WithCiliumOptions(opts ...cilium.Option) RunOption {
	return func(p *RunParams) error { p.ciliumOptions = append(p.ciliumOptions, opts...); return nil }
}

// WithDeployOperator enables operator path
func WithDeployOperator() RunOption {
	return func(p *RunParams) error { p.deployOperator = true; return nil }
}

// WithOperatorDDAOptions sets DDA options for operator path.
// When called, the DatadogAgent custom resource will be deployed with these options.
func WithOperatorDDAOptions(opts ...agentwithoperatorparams.Option) RunOption {
	return func(p *RunParams) error {
		if p.operatorDDAOptions == nil {
			p.operatorDDAOptions = opts
		} else {
			p.operatorDDAOptions = append(p.operatorDDAOptions, opts...)
		}
		return nil
	}
}

// WithoutDDA removes the DatadogAgent custom resource deployment.
// Use this to deploy only the operator without a DDA instance.
func WithoutDDA() RunOption {
	return func(p *RunParams) error {
		p.operatorDDAOptions = nil
		return nil
	}
}

// WithDeployDogstatsd enables dogstatsd deployment
func WithDeployDogstatsd() RunOption {
	return func(p *RunParams) error { p.deployDogstatsd = true; return nil }
}

// WithDeployTestWorkload enables test workloads
func WithDeployTestWorkload() RunOption {
	return func(p *RunParams) error { p.deployTestWorkload = true; return nil }
}

// WithWorkloadApp adds a workload app to the environment
func WithWorkloadApp(appFunc kubecomp.WorkloadAppFunc) RunOption {
	return func(p *RunParams) error { p.workloadAppFuncs = append(p.workloadAppFuncs, appFunc); return nil }
}

// WithAgentDependentWorkloadApp adds a workload app to the environment with the agent passed in
func WithAgentDependentWorkloadApp(appFunc kubecomp.AgentDependentWorkloadAppFunc) RunOption {
	return func(p *RunParams) error {
		p.depWorkloadAppFuncs = append(p.depWorkloadAppFuncs, appFunc)
		return nil
	}
}

// WithDeployArgoRollout enables Argo Rollout deployment
func WithDeployArgoRollout() RunOption {
	return func(p *RunParams) error { p.deployArgoRollout = true; return nil }
}

// WithOperatorOptions sets operator options
func WithOperatorOptions(opts ...operatorparams.Option) RunOption {
	return func(p *RunParams) error { p.operatorOptions = append(p.operatorOptions, opts...); return nil }
}
