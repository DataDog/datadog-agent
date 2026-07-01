// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

package localmultipassvm

import (
	"github.com/pulumi/pulumi/sdk/v3/go/pulumi"

	"github.com/DataDog/datadog-agent/test/e2e-framework/common/utils"
	"github.com/DataDog/datadog-agent/test/e2e-framework/components/command"
	"github.com/DataDog/datadog-agent/test/e2e-framework/components/datadog/agent"
	"github.com/DataDog/datadog-agent/test/e2e-framework/components/datadog/agentparams"
	"github.com/DataDog/datadog-agent/test/e2e-framework/components/datadog/apps/redis"
	"github.com/DataDog/datadog-agent/test/e2e-framework/components/datadog/fakeintake"
	"github.com/DataDog/datadog-agent/test/e2e-framework/components/docker"
	"github.com/DataDog/datadog-agent/test/e2e-framework/resources/local"
)

// VMRun is the entry point for the scenario when run via pulumi.
func VMRun(ctx *pulumi.Context) error {
	env, err := local.NewEnvironment(ctx)
	if err != nil {
		return err
	}
	return Run(ctx, ParamsFromEnvironment(env))
}

func Run(ctx *pulumi.Context, params *Params) error {
	env, err := local.NewEnvironment(ctx)
	if err != nil {
		return err
	}

	vm, err := NewVM(env, params.Name, params.vmOptions...)
	if err != nil {
		return err
	}
	if err := vm.Export(ctx, nil); err != nil {
		return err
	}

	dockerInstall, err := docker.InstallDocker(vm, pulumi.Parent(vm))
	if err != nil {
		return err
	}
	composeInstall, err := docker.InstallCompose(vm, pulumi.Parent(vm))
	if err != nil {
		return err
	}
	dockerManager, err := docker.NewManager(&env, vm, pulumi.Parent(vm), utils.PulumiDependsOn(dockerInstall, composeInstall))
	if err != nil {
		return err
	}
	if err := dockerManager.Export(ctx, nil); err != nil {
		return err
	}

	if env.TestingWorkloadDeploy() {
		if _, err := dockerManager.ComposeStrUp("workloads", []docker.ComposeInlineManifest{
			redis.DockerComposeManifest,
		}, pulumi.StringMap{}); err != nil {
			return err
		}
	}

	if params.agentOptions != nil {
		agentOptions := []agentparams.Option{
			agentparams.WithLogs(),
			agentparams.WithAgentConfig("logs_config.container_collect_all: true"),
		}

		if params.deployFakeIntake {
			fi, err := fakeintake.NewLocalDockerFakeintake(&env, "fakeintake")
			if err != nil {
				return err
			}
			if err := fi.Export(ctx, nil); err != nil {
				return err
			}
			agentOptions = append(agentOptions, agentparams.WithFakeintake(fi))
		}

		if env.AgentFlavor() != "" {
			agentOptions = append(agentOptions, agentparams.WithFlavor(env.AgentFlavor()))
		}
		agentOptions = append(agentOptions, agentparams.WithHostname(env.VMHostname()))
		if env.AgentConfigPath() != "" {
			configContent, err := env.CustomAgentConfig()
			if err != nil {
				return err
			}
			agentOptions = append(agentOptions, agentparams.WithAgentConfig(configContent))
		}

		agentOptions = append(agentOptions, params.agentOptions...)

		hostAgent, err := agent.NewHostAgent(&env, vm, agentOptions...)
		if err != nil {
			return err
		}
		// dd-agent must be in the docker group to access /var/run/docker.sock for the Docker check.
		usermodCmd, err := vm.OS.Runner().Command(
			env.CommonNamer().ResourceName("dd-agent-docker-group"),
			&command.Args{
				Create: pulumi.String("usermod -a -G docker dd-agent"),
				Sudo:   true,
			},
			pulumi.DependsOn([]pulumi.Resource{hostAgent}),
		)
		if err != nil {
			return err
		}
		if _, err := vm.OS.Runner().Command(
			env.CommonNamer().ResourceName("docker-group-restart-agent"),
			&command.Args{
				Create: pulumi.String("systemctl restart datadog-agent"),
				Sudo:   true,
			},
			pulumi.DependsOn([]pulumi.Resource{usermodCmd}),
		); err != nil {
			return err
		}
	}

	return nil
}
