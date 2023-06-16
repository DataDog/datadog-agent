// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package e2e

import (
	"github.com/DataDog/datadog-agent/test/new-e2e/runner"
	"github.com/DataDog/datadog-agent/test/new-e2e/utils/e2e/client"
	"github.com/DataDog/test-infra-definitions/components/datadog/agent"
	"github.com/DataDog/test-infra-definitions/components/datadog/agentparams"
	"github.com/DataDog/test-infra-definitions/components/vm"
	"github.com/DataDog/test-infra-definitions/scenarios/aws/ecs"
	"github.com/DataDog/test-infra-definitions/scenarios/aws/vm/ec2params"
	"github.com/DataDog/test-infra-definitions/scenarios/aws/vm/ec2vm"
	"github.com/pulumi/pulumi/sdk/v3/go/pulumi"
)

type StackDefinition[Env any] struct {
	envFactory func(ctx *pulumi.Context) (*Env, error)
	configMap  runner.ConfigMap
}

func NewStackDef[Env any](envFactory func(ctx *pulumi.Context) (*Env, error), configMap runner.ConfigMap) *StackDefinition[Env] {
	return &StackDefinition[Env]{envFactory: envFactory, configMap: configMap}
}

// EnvFactoryStackDef creates a custom stack definition
func EnvFactoryStackDef[Env any](envFactory func(ctx *pulumi.Context) (*Env, error)) *StackDefinition[Env] {
	return NewStackDef(envFactory, runner.ConfigMap{})
}

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

type AgentEnv struct {
	VM         *client.VM
	Agent      *client.Agent
	Fakeintake *client.Fakeintake
}

// AgentStackDef creates a stack definition containing a virtual machine and an Agent.
//
// See [ec2vm.Params] for available options for vmParams.
//
// See [agent.Params] for available options for agentParams.
//
// [ec2vm.Params]: https://pkg.go.dev/github.com/DataDog/test-infra-definitions@main/scenarios/aws/vm/ec2VM#Params
// [agent.Params]: https://pkg.go.dev/github.com/DataDog/test-infra-definitions@main/components/datadog/agent#Params
func AgentStackDef(vmParams []ec2params.Option, agentParameters ...agentparams.Option) *StackDefinition[AgentEnv] {
	return EnvFactoryStackDef(
		func(ctx *pulumi.Context) (*AgentEnv, error) {
			vm, err := ec2vm.NewEc2VM(ctx, vmParams...)
			if err != nil {
				return nil, err
			}

			fakeintakeExporter, err := ecs.NewEcsFakeintake(vm.Infra)
			if err != nil {
				return nil, err
			}

			agentParameters = append(agentParameters, agentparams.WithFakeintake(fakeintakeExporter))
			installer, err := agent.NewInstaller(vm, agentParameters...)
			if err != nil {
				return nil, err
			}
			return &AgentEnv{
				VM:         client.NewVM(vm),
				Agent:      client.NewAgent(installer),
				Fakeintake: client.NewFakeintake(fakeintakeExporter),
			}, nil
		},
	)
}
