// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

package computerun

import (
	"github.com/DataDog/datadog-agent/test/e2e-framework/components/datadog/agent"
	"github.com/DataDog/datadog-agent/test/e2e-framework/components/datadog/agentparams"
	"github.com/DataDog/datadog-agent/test/e2e-framework/components/os"
	"github.com/DataDog/datadog-agent/test/e2e-framework/resources/azure"
	"github.com/DataDog/datadog-agent/test/e2e-framework/scenarios/azure/compute"
	"github.com/DataDog/datadog-agent/test/e2e-framework/scenarios/azure/fakeintake"

	"github.com/pulumi/pulumi/sdk/v3/go/pulumi"
)

func VMRun(ctx *pulumi.Context) error {
	env, err := azure.NewEnvironment(ctx)
	if err != nil {
		return err
	}

	osDesc := os.DescriptorFromString(env.InfraOSDescriptor(), os.UbuntuDefault)
	vm, err := compute.NewVM(env, "vm", compute.WithImageURN(env.InfraOSImageID(), osDesc, osDesc.Architecture))
	if err != nil {
		return err
	}
	if err := vm.Export(ctx, nil); err != nil {
		return err
	}

	if env.AgentDeploy() {
		agentOptions := []agentparams.Option{}
		if env.AgentUseFakeintake() {
			fakeintake, err := fakeintake.NewVMInstance(env)
			if err != nil {
				return err
			}
			if err := fakeintake.Export(ctx, nil); err != nil {
				return err
			}

			agentOptions = append(agentOptions, agentparams.WithFakeintake(fakeintake))
		}
		if env.AgentFlavor() != "" {
			agentOptions = append(agentOptions, agentparams.WithFlavor(env.AgentFlavor()))
		}
		if env.AgentConfigPath() != "" {
			configContent, err := env.CustomAgentConfig()
			if err != nil {
				return err
			}
			agentOptions = append(agentOptions, agentparams.WithAgentConfig(configContent))
		}
		_, err = agent.NewHostAgent(&env, vm, agentOptions...)
		return err
	}

	return nil
}
