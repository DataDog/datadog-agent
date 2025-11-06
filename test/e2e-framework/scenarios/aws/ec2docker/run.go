// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

package ec2docker

import (
	"github.com/DataDog/datadog-agent/test/e2e-framework/common/utils"
	"github.com/DataDog/datadog-agent/test/e2e-framework/components/datadog/agent"
	"github.com/DataDog/datadog-agent/test/e2e-framework/components/datadog/apps/dogstatsd"
	"github.com/DataDog/datadog-agent/test/e2e-framework/components/datadog/apps/redis"
	"github.com/DataDog/datadog-agent/test/e2e-framework/components/datadog/dockeragentparams"
	"github.com/DataDog/datadog-agent/test/e2e-framework/components/docker"
	"github.com/DataDog/datadog-agent/test/e2e-framework/resources/aws"
	"github.com/DataDog/datadog-agent/test/e2e-framework/scenarios/aws/ec2"
	"github.com/DataDog/datadog-agent/test/e2e-framework/scenarios/aws/fakeintake"
	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/environments"

	"github.com/pulumi/pulumi/sdk/v3/go/pulumi"
)

func Run(ctx *pulumi.Context, awsEnv aws.Environment, env *environments.DockerHost, params *Params) error {

	host, err := ec2.NewVM(awsEnv, params.Name, params.vmOptions...)
	if err != nil {
		return err
	}
	err = host.Export(ctx, &env.RemoteHost.HostOutput)
	if err != nil {
		return err
	}

	// install the ECR credentials helper
	// required to get pipeline agent images
	installEcrCredsHelperCmd, err := ec2.InstallECRCredentialsHelper(awsEnv, host)
	if err != nil {
		return err
	}

	manager, err := docker.NewManager(&awsEnv, host, utils.PulumiDependsOn(installEcrCredsHelperCmd))
	if err != nil {
		return err
	}
	err = manager.Export(ctx, &env.Docker.ManagerOutput)
	if err != nil {
		return err
	}

	// Create FakeIntake if required
	if params.fakeintakeOptions != nil {
		fakeIntake, err := fakeintake.NewECSFargateInstance(awsEnv, params.Name, params.fakeintakeOptions...)
		if err != nil {
			return err
		}
		err = fakeIntake.Export(ctx, &env.FakeIntake.FakeintakeOutput)
		if err != nil {
			return err
		}

		// Normally if FakeIntake is enabled, Agent is enabled, but just in case
		if params.agentOptions != nil {
			// Prepend in case it's overridden by the user
			newOpts := []dockeragentparams.Option{dockeragentparams.WithFakeintake(fakeIntake)}
			params.agentOptions = append(newOpts, params.agentOptions...)
		}
	} else {
		// Suite inits all fields by default, so we need to explicitly set it to nil
		env.FakeIntake = nil
	}

	for _, hook := range params.preAgentInstallHooks {
		res, err := hook(&awsEnv, host)
		if err != nil {
			return err
		}
		if res != nil {
			params.agentOptions = append(params.agentOptions, dockeragentparams.WithPulumiDependsOn(utils.PulumiDependsOn(res)))
		}
	}

	// Create Agent if required
	if params.agentOptions != nil {
		params.agentOptions = append(params.agentOptions, dockeragentparams.WithTags([]string{"stackid:" + ctx.Stack()}))
		if params.testingWorkload {
			params.agentOptions = append(params.agentOptions, dockeragentparams.WithExtraComposeManifest(redis.DockerComposeManifest.Name, redis.DockerComposeManifest.Content))
			params.agentOptions = append(params.agentOptions, dockeragentparams.WithExtraComposeManifest(dogstatsd.DockerComposeManifest.Name, dogstatsd.DockerComposeManifest.Content))
			params.agentOptions = append(params.agentOptions, dockeragentparams.WithEnvironmentVariables(pulumi.StringMap{"HOST_IP": host.Address}))
		}
		agent, err := agent.NewDockerAgent(&awsEnv, host, manager, params.agentOptions...)
		if err != nil {
			return err
		}

		err = agent.Export(ctx, &env.Agent.DockerAgentOutput)
		if err != nil {
			return err
		}
	} else {
		// Suite inits all fields by default, so we need to explicitly set it to nil
		env.Agent = nil
	}

	return nil
}

func DockerRun(ctx *pulumi.Context) error {

	awsEnv, err := aws.NewEnvironment(ctx)
	if err != nil {
		return err
	}

	env, _, _, err := environments.CreateEnv[environments.DockerHost]()
	if err != nil {
		return err
	}

	return Run(ctx, awsEnv, env, ParamsFromEnvironment(awsEnv))
}
