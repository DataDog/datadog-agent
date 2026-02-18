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
	fakeintakescenario "github.com/DataDog/datadog-agent/test/e2e-framework/scenarios/aws/fakeintake"
	"github.com/DataDog/datadog-agent/test/e2e-framework/scenarios/outputs"

	"github.com/pulumi/pulumi/sdk/v3/go/pulumi"
)

// Run deploys an environment given a pulumi.Context.
// It accepts DockerHostOutputs interface, which is implemented by both:
// - outputs.DockerHost (lightweight, for scenarios without test dependencies)
// - environments.DockerHost (full-featured, for test provisioners)
func Run(ctx *pulumi.Context, awsEnv aws.Environment, env outputs.DockerHostOutputs, params *Params) error {

	host, err := ec2.NewVM(awsEnv, params.Name, params.vmOptions...)
	if err != nil {
		return err
	}
	err = host.Export(ctx, env.RemoteHostOutput())
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
	err = manager.Export(ctx, env.DockerOutput())
	if err != nil {
		return err
	}

	// Create FakeIntake if required
	if params.fakeintakeOptions != nil {
		fakeIntake, err := fakeintakescenario.NewECSFargateInstance(awsEnv, params.Name, params.fakeintakeOptions...)
		if err != nil {
			return err
		}
		err = fakeIntake.Export(ctx, env.FakeIntakeOutput())
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
		// Mark FakeIntake as not provisioned
		env.DisableFakeIntake()
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

		err = agent.Export(ctx, env.DockerAgentOutput())
		if err != nil {
			return err
		}
	} else {
		// Mark Agent as not provisioned
		env.DisableAgent()
	}

	return nil
}

// DockerRun is the entry point for the scenario when run via pulumi.
// It uses outputs.DockerHost which is lightweight and doesn't pull in test dependencies.
func DockerRun(ctx *pulumi.Context) error {

	awsEnv, err := aws.NewEnvironment(ctx)
	if err != nil {
		return err
	}

	env := outputs.NewDockerHost()

	return Run(ctx, awsEnv, env, ParamsFromEnvironment(awsEnv))
}
