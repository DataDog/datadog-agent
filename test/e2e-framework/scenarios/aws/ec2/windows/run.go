// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

package windows

import (
	"fmt"

	"github.com/DataDog/datadog-agent/test/e2e-framework/components/activedirectory"
	"github.com/DataDog/datadog-agent/test/e2e-framework/components/datadog/agent"
	"github.com/DataDog/datadog-agent/test/e2e-framework/components/datadog/agentparams"
	compos "github.com/DataDog/datadog-agent/test/e2e-framework/components/os"
	"github.com/DataDog/datadog-agent/test/e2e-framework/resources/aws"
	"github.com/DataDog/datadog-agent/test/e2e-framework/scenarios/aws/ec2"
	"github.com/DataDog/datadog-agent/test/e2e-framework/scenarios/aws/fakeintake"
	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/environments"
	"github.com/DataDog/datadog-agent/test/new-e2e/tests/windows/components/defender"
	"github.com/DataDog/datadog-agent/test/new-e2e/tests/windows/components/fipsmode"
	"github.com/DataDog/datadog-agent/test/new-e2e/tests/windows/components/testsigning"

	"github.com/pulumi/pulumi/sdk/v3/go/pulumi"
)

// RunWithEnv deploys a Windows EC2 environment using provided env and params
func RunWithEnv(ctx *pulumi.Context, awsEnv aws.Environment, env *environments.WindowsHost, params *RunParams) error {
	env.Environment = &awsEnv

	// Force Windows OS
	params.instanceOptions = append(params.instanceOptions, ec2.WithOS(compos.WindowsServerDefault))

	host, err := ec2.NewVM(awsEnv, params.Name, params.instanceOptions...)
	if err != nil {
		return err
	}
	if err := host.Export(ctx, &env.RemoteHost.HostOutput); err != nil {
		return err
	}

	if params.defenderoptions != nil {
		def, err := defender.NewDefender(awsEnv.CommonEnvironment, host, params.defenderoptions...)
		if err != nil {
			return err
		}
		// TestSigning setup needs to happen after Windows Defender setup
		params.testsigningOptions = append(params.testsigningOptions,
			testsigning.WithPulumiResourceOptions(
				pulumi.DependsOn(def.Resources)))
	}

	if params.testsigningOptions != nil {
		ts, err := testsigning.NewTestSigning(awsEnv.CommonEnvironment, host, params.testsigningOptions...)
		if err != nil {
			return err
		}
		// Active Directory setup needs to happen after TestSigning setup
		params.activeDirectoryOptions = append(params.activeDirectoryOptions,
			activedirectory.WithPulumiResourceOptions(
				pulumi.DependsOn(ts.Resources)))
	}

	if params.activeDirectoryOptions != nil {
		adComp, adResources, err := activedirectory.NewActiveDirectory(ctx, &awsEnv, host, params.activeDirectoryOptions...)
		if err != nil {
			return err
		}
		if err := adComp.Export(ctx, &env.ActiveDirectory.Output); err != nil {
			return err
		}

		if params.agentOptions != nil {
			// Agent install needs to happen after ActiveDirectory setup
			params.agentOptions = append(params.agentOptions,
				agentparams.WithPulumiResourceOptions(
					pulumi.DependsOn(adResources)))
		}
	} else {
		// Suite inits all fields by default, so we need to explicitly set it to nil
		env.ActiveDirectory = nil
	}

	// Create FakeIntake if required
	if params.fakeintakeOptions != nil {
		fi, err := fakeintake.NewECSFargateInstance(awsEnv, params.Name, params.fakeintakeOptions...)
		if err != nil {
			return err
		}
		if err := fi.Export(ctx, &env.FakeIntake.FakeintakeOutput); err != nil {
			return err
		}
		// Normally if FakeIntake is enabled, Agent is enabled, but just in case
		if params.agentOptions != nil {
			// Prepend in case it's overridden by the user
			newOpts := []agentparams.Option{agentparams.WithFakeintake(fi)}
			params.agentOptions = append(newOpts, params.agentOptions...)
		}
	} else {
		env.FakeIntake = nil
	}

	if params.agentOptions != nil {
		agentOptions := append(params.agentOptions, agentparams.WithTags([]string{fmt.Sprintf("stackid:%s", ctx.Stack())}))
		ag, err := agent.NewHostAgent(&awsEnv, host, agentOptions...)
		if err != nil {
			return err
		}
		if err := ag.Export(ctx, &env.Agent.HostAgentOutput); err != nil {
			return err
		}
		env.Agent.ClientOptions = params.agentClientOptions
	} else {
		env.Agent = nil
	}

	if params.fipsModeOptions != nil {
		fips, err := fipsmode.New(awsEnv.CommonEnvironment, host, params.fipsModeOptions...)
		if err != nil {
			return err
		}
		// Ensure Agent setup happens after FIPS mode setup when both are requested
		params.agentOptions = append(params.agentOptions,
			agentparams.WithPulumiResourceOptions(
				pulumi.DependsOn(fips.Resources)))
	}

	return nil
}

// Run is a convenience wrapper that creates env and runs the scenario
func Run(ctx *pulumi.Context) error {
	awsEnv, err := aws.NewEnvironment(ctx)
	if err != nil {
		return err
	}

	env, _, _, err := environments.CreateEnv[environments.WindowsHost]()
	if err != nil {
		return err
	}

	params := ParamsFromEnvironment(awsEnv)
	return RunWithEnv(ctx, awsEnv, env, params)
}
