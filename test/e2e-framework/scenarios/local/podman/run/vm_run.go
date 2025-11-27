// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

package localpodmanrun

import (
	"github.com/DataDog/datadog-agent/test/e2e-framework/components/datadog/agent"
	"github.com/DataDog/datadog-agent/test/e2e-framework/components/datadog/agentparams"
	"github.com/DataDog/datadog-agent/test/e2e-framework/resources/local"
	localpodman "github.com/DataDog/datadog-agent/test/e2e-framework/scenarios/local/podman"

	"github.com/DataDog/datadog-agent/test/e2e-framework/components/datadog/fakeintake"
	"github.com/pulumi/pulumi/sdk/v3/go/pulumi"
)

func VMRun(ctx *pulumi.Context) error {
	env, err := local.NewEnvironment(ctx)
	if err != nil {
		return err
	}

	vm, err := localpodman.NewVM(env, "podman-vm")
	if err != nil {
		return err
	}
	if err := vm.Export(ctx, nil); err != nil {
		return err
	}

	if env.AgentDeploy() {
		agentOptions := []agentparams.Option{}
		if env.AgentUseFakeintake() {
			fakeintake, err := fakeintake.NewLocalDockerFakeintake(&env, "fakeintake")
			if err != nil {
				return err
			}
			err = fakeintake.Export(ctx, nil)
			if err != nil {
				return err
			}
			agentOptions = append(agentOptions, agentparams.WithFakeintake(fakeintake))
		}
		if env.AgentFlavor() != "" {
			agentOptions = append(agentOptions, agentparams.WithFlavor(env.AgentFlavor()))
		}
		agentOptions = append(agentOptions, agentparams.WithHostname("localpodman-vm"))
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
