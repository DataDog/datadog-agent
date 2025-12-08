// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

package ec2

import (
	"fmt"

	"github.com/DataDog/datadog-agent/test/e2e-framework/common/utils"
	"github.com/DataDog/datadog-agent/test/e2e-framework/components/datadog/agent"
	"github.com/DataDog/datadog-agent/test/e2e-framework/components/datadog/agentparams"
	"github.com/DataDog/datadog-agent/test/e2e-framework/components/datadog/updater"
	"github.com/DataDog/datadog-agent/test/e2e-framework/components/docker"
	"github.com/DataDog/datadog-agent/test/e2e-framework/resources/aws"
	"github.com/DataDog/datadog-agent/test/e2e-framework/scenarios/aws/fakeintake"
	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/environments"

	"github.com/pulumi/pulumi/sdk/v3/go/pulumi"
)

// Run deploys a environment given a pulumi.Context
func Run(ctx *pulumi.Context, awsEnv aws.Environment, env *environments.Host, params *Params) error {

	host, err := NewVM(awsEnv, params.Name, params.instanceOptions...)
	if err != nil {
		return err
	}
	err = host.Export(ctx, &env.RemoteHost.HostOutput)
	if err != nil {
		return err
	}

	if params.installDocker {
		// install the ECR credentials helper
		// required to get pipeline agent images or other internally hosted images
		installEcrCredsHelperCmd, err := InstallECRCredentialsHelper(awsEnv, host)
		if err != nil {
			return err
		}

		dockerManager, err := docker.NewManager(&awsEnv, host, utils.PulumiDependsOn(installEcrCredsHelperCmd))

		if err != nil {
			return err
		}
		if params.agentOptions != nil {
			// Agent install needs to be serial with the docker
			// install because they both use the apt lock, and
			// can cause each others' installs to fail if run
			// at the same time.
			params.agentOptions = append(params.agentOptions,
				agentparams.WithPulumiResourceOptions(
					utils.PulumiDependsOn(dockerManager)))
		}
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
			newOpts := []agentparams.Option{agentparams.WithFakeintake(fakeIntake)}
			params.agentOptions = append(newOpts, params.agentOptions...)
		}
	} else {
		// Suite inits all fields by default, so we need to explicitly set it to nil
		env.FakeIntake = nil
	}
	if !params.installUpdater {
		// Suite inits all fields by default, so we need to explicitly set it to nil
		env.Updater = nil
	}

	// Create Agent if required
	if params.installUpdater && params.agentOptions != nil {
		updater, err := updater.NewHostUpdater(&awsEnv, host, params.agentOptions...)
		if err != nil {
			return err
		}

		err = updater.Export(ctx, &env.Updater.HostUpdaterOutput)
		if err != nil {
			return err
		}
		// todo: add agent once updater installs agent on bootstrap
		env.Agent = nil
	} else if params.agentOptions != nil {
		agentOptions := append(params.agentOptions, agentparams.WithTags([]string{fmt.Sprintf("stackid:%s", ctx.Stack())}))
		agent, err := agent.NewHostAgent(&awsEnv, host, agentOptions...)
		if err != nil {
			return err
		}

		err = agent.Export(ctx, &env.Agent.HostAgentOutput)
		if err != nil {
			return err
		}

		env.Agent.ClientOptions = params.agentClientOptions
	} else {
		// Suite inits all fields by default, so we need to explicitly set it to nil
		env.Agent = nil
	}

	return nil
}

func VMRun(ctx *pulumi.Context) error {
	awsEnv, err := aws.NewEnvironment(ctx)
	if err != nil {
		return err
	}

	env, _, _, err := environments.CreateEnv[environments.Host]()
	if err != nil {
		return err
	}

	return Run(ctx, awsEnv, env, ParamsFromEnvironment(awsEnv))
}
