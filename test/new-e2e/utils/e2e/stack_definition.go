// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package e2e

import (
	"github.com/DataDog/datadog-agent/test/new-e2e/runner"
	"github.com/DataDog/datadog-agent/test/new-e2e/utils/e2e/client"
	ec2vm "github.com/DataDog/test-infra-definitions/aws/scenarios/vm/ec2VM"
	"github.com/DataDog/test-infra-definitions/datadog/agent"
	"github.com/pulumi/pulumi/sdk/v3/go/pulumi"
)

type StackDefinition[Env any] struct {
	envFactory func(ctx *pulumi.Context) (*Env, error)
	configMap  runner.ConfigMap
}

func NewStackDef[Env any](envFactory func(ctx *pulumi.Context) (*Env, error), configMap runner.ConfigMap) *StackDefinition[Env] {
	return &StackDefinition[Env]{envFactory: envFactory, configMap: configMap}
}

func EnvFactoryStackDef[Env any](envFactory func(ctx *pulumi.Context) (*Env, error)) *StackDefinition[Env] {
	return NewStackDef(envFactory, runner.ConfigMap{})
}

type VMEnv struct {
	VM *client.VM
}

func EC2VMStackDef(options ...func(*ec2vm.Params) error) *StackDefinition[VMEnv] {
	return EnvFactoryStackDef(func(ctx *pulumi.Context) (*VMEnv, error) {
		vm, err := ec2vm.NewEc2VM(ctx, options...)
		if err != nil {
			return nil, err
		}
		return &VMEnv{
			VM: client.NewVM(vm),
		}, nil
	})
}

type AgentEnv struct {
	VM    *client.VM
	Agent *client.Agent
}

type Ec2VMOption = func(*ec2vm.Params) error

func AgentStackDef(vmParams []Ec2VMOption, agentParams ...func(*agent.Params) error) *StackDefinition[AgentEnv] {
	return EnvFactoryStackDef(
		func(ctx *pulumi.Context) (*AgentEnv, error) {
			vm, err := ec2vm.NewEc2VM(ctx, vmParams...)
			if err != nil {
				return nil, err
			}

			installer, err := agent.NewInstaller(vm, agentParams...)
			if err != nil {
				return nil, err
			}
			return &AgentEnv{
				VM:    client.NewVM(vm),
				Agent: client.NewAgent(installer),
			}, nil
		},
	)
}
