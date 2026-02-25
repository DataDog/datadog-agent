// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

package ec2

import (
	"errors"

	"github.com/DataDog/datadog-agent/test/e2e-framework/common/utils"
	"github.com/DataDog/datadog-agent/test/e2e-framework/components/datadog/agent"
	"github.com/DataDog/datadog-agent/test/e2e-framework/components/datadog/agentparams"
	"github.com/DataDog/datadog-agent/test/e2e-framework/components/datadog/apps/dogstatsd"
	"github.com/DataDog/datadog-agent/test/e2e-framework/components/datadog/dockeragentparams"
	"github.com/DataDog/datadog-agent/test/e2e-framework/components/datadog/updater"
	"github.com/DataDog/datadog-agent/test/e2e-framework/components/docker"
	"github.com/DataDog/datadog-agent/test/e2e-framework/components/os"
	"github.com/DataDog/datadog-agent/test/e2e-framework/resources/aws"
	"github.com/DataDog/datadog-agent/test/e2e-framework/scenarios/aws/fakeintake"

	"github.com/pulumi/pulumi/sdk/v3/go/pulumi"
)

// VMRun is the Pulumi run function that creates a VM with an optional agent.
func VMRun(ctx *pulumi.Context) error {
	env, err := aws.NewEnvironment(ctx)
	if err != nil {
		return err
	}

	osDesc := os.DescriptorFromString(env.InfraOSDescriptor(), os.AmazonLinuxECSDefault)
	args := []VMOption{WithAMI(env.InfraOSImageID(), osDesc, osDesc.Architecture)}
	if env.InfraOSImageIDUseLatest() {
		args = append(args, WithLatestAMI())
	}
	vm, err := NewVM(env, "vm", args...)
	if err != nil {
		return err
	}
	if err := vm.Export(ctx, nil); err != nil {
		return err
	}

	if env.AgentDeploy() {
		agentOptions := []agentparams.Option{}
		if env.AgentUseFakeintake() {
			fakeIntakeOptions := []fakeintake.Option{}

			if env.InfraShouldDeployFakeintakeWithLB() {
				fakeIntakeOptions = append(fakeIntakeOptions, fakeintake.WithLoadBalancer())
			}

			if storeType := env.AgentFakeintakeStoreType(); storeType != "" {
				fakeIntakeOptions = append(fakeIntakeOptions, fakeintake.WithStoreType(storeType))
			}

			if retentionPeriod := env.AgentFakeintakeRetentionPeriod(); retentionPeriod != "" {
				fakeIntakeOptions = append(fakeIntakeOptions, fakeintake.WithRetentionPeriod(retentionPeriod))
			}

			fakeintake, err := fakeintake.NewECSFargateInstance(env, vm.Name(), fakeIntakeOptions...)
			if err != nil {
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

		agent, err := agent.NewHostAgent(&env, vm, agentOptions...)
		if err != nil {
			return err
		}

		return agent.Export(ctx, nil)
	}

	if env.UpdaterDeploy() {
		if env.AgentDeploy() {
			return errors.New("cannot deploy both agent and updater installers, updater installs the agent")
		}

		_, err := updater.NewHostUpdater(&env, vm)
		return err
	}

	return nil
}

// VMRunWithDocker is the Pulumi run function that creates a VM with Docker and an optional agent.
func VMRunWithDocker(ctx *pulumi.Context) error {
	env, err := aws.NewEnvironment(ctx)
	if err != nil {
		return err
	}

	// If no OS is provided, we default to AmazonLinuxECS as it ships with Docker pre-installed
	osDesc := os.DescriptorFromString(env.InfraOSDescriptor(), os.AmazonLinuxECSDefault)
	vm, err := NewVM(env, "vm", WithAMI(env.InfraOSImageID(), osDesc, osDesc.Architecture))
	if err != nil {
		return err
	}
	if err := vm.Export(ctx, nil); err != nil {
		return err
	}

	installEcrCredsHelperCmd, err := InstallECRCredentialsHelper(env, vm)
	if err != nil {
		return err
	}

	manager, err := docker.NewManager(&env, vm, utils.PulumiDependsOn(installEcrCredsHelperCmd))
	if err != nil {
		return err
	}
	if err := manager.Export(ctx, nil); err != nil {
		return err
	}

	if env.AgentDeploy() {
		agentOptions := make([]dockeragentparams.Option, 0)
		if env.AgentFullImagePath() != "" {
			agentOptions = append(agentOptions, dockeragentparams.WithFullImagePath(env.AgentFullImagePath()))
		} else if env.AgentVersion() != "" {
			agentOptions = append(agentOptions, dockeragentparams.WithImageTag(env.AgentVersion()))
		}

		if env.AgentJMX() {
			agentOptions = append(agentOptions, dockeragentparams.WithJMX())
		}

		if env.AgentFIPS() {
			agentOptions = append(agentOptions, dockeragentparams.WithFIPS())
		}

		if env.AgentUseFakeintake() {
			fakeIntakeOptions := []fakeintake.Option{}

			if env.InfraShouldDeployFakeintakeWithLB() {
				fakeIntakeOptions = append(fakeIntakeOptions, fakeintake.WithLoadBalancer())
			}

			if storeType := env.AgentFakeintakeStoreType(); storeType != "" {
				fakeIntakeOptions = append(fakeIntakeOptions, fakeintake.WithStoreType(storeType))
			}

			if retentionPeriod := env.AgentFakeintakeRetentionPeriod(); retentionPeriod != "" {
				fakeIntakeOptions = append(fakeIntakeOptions, fakeintake.WithRetentionPeriod(retentionPeriod))
			}

			fakeintake, err := fakeintake.NewECSFargateInstance(env, vm.Name(), fakeIntakeOptions...)
			if err != nil {
				return err
			}

			if err := fakeintake.Export(env.Ctx(), nil); err != nil {
				return err
			}

			agentOptions = append(agentOptions, dockeragentparams.WithFakeintake(fakeintake))
		}

		if env.TestingWorkloadDeploy() {
			agentOptions = append(agentOptions, dockeragentparams.WithExtraComposeManifest(dogstatsd.DockerComposeManifest.Name, dogstatsd.DockerComposeManifest.Content))
			agentOptions = append(agentOptions, dockeragentparams.WithEnvironmentVariables(pulumi.StringMap{"HOST_IP": vm.Address}))
		}

		dockerAgent, err := agent.NewDockerAgent(&env, vm, manager, agentOptions...)
		if err != nil {
			return err
		}
		if err := dockerAgent.Export(env.Ctx(), nil); err != nil {
			return err
		}
	}

	return nil
}
