// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package e2e

import (
	"github.com/DataDog/datadog-agent/test/new-e2e/pkg/runner"
	"github.com/DataDog/datadog-agent/test/new-e2e/pkg/utils/e2e/client"
	"github.com/DataDog/datadog-agent/test/new-e2e/pkg/utils/e2e/client/agentclientparams"
	"github.com/DataDog/test-infra-definitions/components/datadog/agent"
	"github.com/DataDog/test-infra-definitions/components/datadog/agent/docker"
	"github.com/DataDog/test-infra-definitions/components/datadog/agent/dockerparams"
	"github.com/DataDog/test-infra-definitions/components/datadog/agentparams"
	"github.com/DataDog/test-infra-definitions/components/vm"
	"github.com/DataDog/test-infra-definitions/scenarios/aws"
	"github.com/DataDog/test-infra-definitions/scenarios/aws/vm/ec2params"
	"github.com/DataDog/test-infra-definitions/scenarios/aws/vm/ec2vm"
	"github.com/pulumi/pulumi/sdk/v3/go/pulumi"
)

// StackDefinition contains a Pulumi stack definition
type StackDefinition[Env any] struct {
	envFactory func(ctx *pulumi.Context) (*Env, error)
	configMap  runner.ConfigMap
}

// NewStackDef creates a custom definition
func NewStackDef[Env any](envFactory func(ctx *pulumi.Context) (*Env, error), configMap runner.ConfigMap) *StackDefinition[Env] {
	return &StackDefinition[Env]{envFactory: envFactory, configMap: configMap}
}

// EnvFactoryStackDef creates a custom stack definition
func EnvFactoryStackDef[Env any](envFactory func(ctx *pulumi.Context) (*Env, error)) *StackDefinition[Env] {
	return NewStackDef(envFactory, runner.ConfigMap{})
}

// VMEnv contains a VM environment
type VMEnv struct {
	VM *client.VM
}

// EC2VMStackDef creates a stack definition containing a virtual machine.
// See [ec2vm.Params] for available options.
//
// [ec2vm.Params]: https://pkg.go.dev/github.com/DataDog/test-infra-definitions@main/scenarios/aws/vm/ec2VM#Params
func EC2VMStackDef(options ...ec2params.Option) *StackDefinition[VMEnv] {
	noop := func(vm.VM) (VMEnv, error) { return VMEnv{}, nil }
	return CustomEC2VMStackDef(noop, options...)
}

// CustomEC2VMStackDef creates a custom stack definition containing a virtual machine
func CustomEC2VMStackDef[T any](fct func(vm.VM) (T, error), options ...ec2params.Option) *StackDefinition[VMEnv] {
	return EnvFactoryStackDef(func(ctx *pulumi.Context) (*VMEnv, error) {
		vm, err := ec2vm.NewEc2VM(ctx, options...)
		if err != nil {
			return nil, err
		}
		if _, err = fct(vm); err != nil {
			return nil, err
		}

		return &VMEnv{
			VM: client.NewVM(vm),
		}, nil
	})
}

// AgentEnv contains an Agent VM environment
type AgentEnv struct {
	VM    *client.VM
	Agent *client.Agent
}

// AgentStackDefParam defines the parameters for a stack with a VM and the Datadog Agent
// installed.
// The AgentStackDefParam configuration uses the [Functional options pattern].
//
// The available options are:
//   - [WithVMParams]
//   - [WithAgentParams]
//   - [WithAgentClientParams]
//
// [Functional options pattern]: https://dave.cheney.net/2014/10/17/functional-options-for-friendly-apis
type AgentStackDefParam struct {
	vmParams          []ec2params.Option
	agentClientParams []agentclientparams.Option
	agentParams       []agentparams.Option
}

func newAgentStackDefParam(options ...func(*AgentStackDefParam) error) (*AgentStackDefParam, error) {
	params := &AgentStackDefParam{}
	for _, o := range options {
		err := o(params)
		if err != nil {
			return nil, err
		}
	}
	return params, nil
}

// WithVMParams sets VM parameters
// See [ec2vm.Params] for available options for vmParams.
func WithVMParams(options ...ec2params.Option) func(*AgentStackDefParam) error {
	return func(p *AgentStackDefParam) error {
		p.vmParams = options
		return nil
	}
}

// WithAgentParams sets Agent parameters
// See [agent.Params] for available options for agentParams.
func WithAgentParams(options ...agentparams.Option) func(*AgentStackDefParam) error {
	return func(p *AgentStackDefParam) error {
		p.agentParams = options
		return nil
	}
}

// WithAgentClientParams sets Agent client parameters
// See [agentclientparams.Params] for available options for agentParams
func WithAgentClientParams(options ...agentclientparams.Option) func(*AgentStackDefParam) error {
	return func(p *AgentStackDefParam) error {
		p.agentClientParams = options
		return nil
	}
}

// AgentStackDef creates a stack definition containing a virtual machine and an Agent.
//
// See [ec2vm.Params] for available options for vmParams.
//
// See [agent.Params] for available options for agentParams.
//
// See [agentclientparams.Params] for available options for agentParams
//
// [ec2vm.Params]: https://pkg.go.dev/github.com/DataDog/test-infra-definitions@main/scenarios/aws/vm/ec2VM#Params
// [agent.Params]: https://pkg.go.dev/github.com/DataDog/test-infra-definitions@main/components/datadog/agent#Params
// [agentclientparams.Params]: https://pkg.go.dev/github.com/DataDog/datadog-agent@main/test/new-e2e/pkg/utils/e2e/client/agentclientparams#Params
func AgentStackDef(options ...func(*AgentStackDefParam) error) *StackDefinition[AgentEnv] {
	return EnvFactoryStackDef(
		func(ctx *pulumi.Context) (*AgentEnv, error) {
			params, err := newAgentStackDefParam(options...)
			if err != nil {
				return nil, err
			}
			vm, err := ec2vm.NewEc2VM(ctx, params.vmParams...)
			if err != nil {
				return nil, err
			}

			installer, err := agent.NewInstaller(vm, params.agentParams...)
			if err != nil {
				return nil, err
			}
			return &AgentEnv{
				VM:    client.NewVM(vm),
				Agent: client.NewAgent(installer, params.agentClientParams...),
			}, nil
		},
	)
}

// AgentStackDefWithDefaultVMAndAgentClient creates a stack definition containing an Ubuntu virtual machine and an Agent.
// The Agent is awaited at TestSuite setup time, any subtest runs with an healthy agent
//
// See [agent.Params] for available options.
//
// [agent.Params]: https://pkg.go.dev/github.com/DataDog/test-infra-definitions@main/components/datadog/agent#Params
func AgentStackDefWithDefaultVMAndAgentClient(options ...agentparams.Option) *StackDefinition[AgentEnv] {
	return AgentStackDef(WithAgentParams(options...))
}

// FakeIntakeEnv contains an environment with the Agent
// installed on a VM and a dedicated fakeintake
type FakeIntakeEnv struct {
	VM         *client.VM
	Agent      *client.Agent
	Fakeintake *client.Fakeintake
}

// FakeIntakeStackDef creates a stack definition containing a virtual machine the Agent and the fake intake.
//
// See [ec2vm.Params] for available options for vmParams.
//
// See [agent.Params] for available options for agentParams.
//
// See [agentclientparams.Params] for available options for agentParams
//
// [ec2vm.Params]: https://pkg.go.dev/github.com/DataDog/test-infra-definitions@main/scenarios/aws/vm/ec2VM#Params
// [agent.Params]: https://pkg.go.dev/github.com/DataDog/test-infra-definitions@main/components/datadog/agent#Params
// [agentclientparams.Params]: https://pkg.go.dev/github.com/DataDog/datadog-agent@main/test/new-e2e/pkg/utils/e2e/client/agentclientparams#Params
func FakeIntakeStackDef(options ...func(*AgentStackDefParam) error) *StackDefinition[FakeIntakeEnv] {
	return EnvFactoryStackDef(
		func(ctx *pulumi.Context) (*FakeIntakeEnv, error) {
			params, err := newAgentStackDefParam(options...)
			if err != nil {
				return nil, err
			}
			vm, err := ec2vm.NewEc2VM(ctx, params.vmParams...)
			if err != nil {
				return nil, err
			}

			fakeintakeExporter, err := aws.NewEcsFakeintake(vm.GetAwsEnvironment())
			if err != nil {
				return nil, err
			}

			agentParameters := append(params.agentParams, agentparams.WithFakeintake(fakeintakeExporter))
			installer, err := agent.NewInstaller(vm, agentParameters...)
			if err != nil {
				return nil, err
			}
			return &FakeIntakeEnv{
				VM:         client.NewVM(vm),
				Agent:      client.NewAgent(installer, params.agentClientParams...),
				Fakeintake: client.NewFakeintake(fakeintakeExporter),
			}, nil
		},
	)
}

// FakeIntakeStackDefWithDefaultVMAndAgentClient creates a stack definition containing an Ubuntu virtual machine, an Agent
// and a fake Datadog intake.
// The Agent is awaited at TestSuite setup time, any subtest runs with an healthy agent
//
// See [agent.Params] for available options.
//
// [agent.Params]: https://pkg.go.dev/github.com/DataDog/test-infra-definitions@main/components/datadog/agent#Params
func FakeIntakeStackDefWithDefaultVMAndAgentClient(options ...agentparams.Option) *StackDefinition[FakeIntakeEnv] {
	return FakeIntakeStackDef(WithAgentParams(options...))
}

// DockerEnv contains an environment with Docker
type DockerEnv struct {
	Docker *client.Docker
}

// DockerStackDef creates a stack definition for Docker.
//
// See [dockerparams.Params] for available options for params.
//
// [dockerparams.Params]: https://pkg.go.dev/github.com/DataDog/test-infra-definitions@main/components/datadog/agent/dockerparams#Params
func DockerStackDef(params ...dockerparams.Option) *StackDefinition[DockerEnv] {
	return EnvFactoryStackDef(
		func(ctx *pulumi.Context) (*DockerEnv, error) {
			docker, err := docker.NewDaemon(ctx, params...)
			if err != nil {
				return nil, err
			}

			return &DockerEnv{
				Docker: client.NewDocker(docker),
			}, nil
		},
	)
}
