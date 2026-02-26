// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

package windows

import (
	"fmt"
	"hash/fnv"
	"math/rand"
	"os"

	"github.com/DataDog/datadog-agent/test/e2e-framework/components/activedirectory"
	"github.com/DataDog/datadog-agent/test/e2e-framework/components/datadog/agent"
	"github.com/DataDog/datadog-agent/test/e2e-framework/components/datadog/agentparams"
	compos "github.com/DataDog/datadog-agent/test/e2e-framework/components/os"
	"github.com/DataDog/datadog-agent/test/e2e-framework/resources/aws"
	"github.com/DataDog/datadog-agent/test/e2e-framework/scenarios/aws/ec2"
	fakeintakescenario "github.com/DataDog/datadog-agent/test/e2e-framework/scenarios/aws/fakeintake"
	"github.com/DataDog/datadog-agent/test/e2e-framework/scenarios/outputs"
	"github.com/DataDog/datadog-agent/test/new-e2e/tests/windows/components/defender"
	"github.com/DataDog/datadog-agent/test/new-e2e/tests/windows/components/fipsmode"
	"github.com/DataDog/datadog-agent/test/new-e2e/tests/windows/components/testsigning"

	"github.com/pulumi/pulumi/sdk/v3/go/pulumi"
)

// RunWithEnv deploys a Windows EC2 environment using provided env and params.
// It accepts WindowsHostOutputs interface, enabling reuse between provisioners and direct Pulumi runs.
func RunWithEnv(ctx *pulumi.Context, awsEnv aws.Environment, env outputs.WindowsHostOutputs, params *RunParams) error {
	// Set the environment for test code access
	env.SetEnvironment(&awsEnv)

	// Use InfraOSDescriptor when set (e.g. -c ddinfra:osDescriptor=...), otherwise pick a Windows Server version (2016–2025) for e2e coverage.
	// In CI, CI_PIPELINE_ID + CI_JOB_NAME are used as seed so retries get the same version.
	osDesc := compos.WindowsServerDefault
	if descStr := awsEnv.InfraOSDescriptor(); descStr != "" {
		osDesc = compos.DescriptorFromString(descStr, compos.WindowsServerDefault)
	} else if os.Getenv("CI_PIPELINE_ID") != "" && os.Getenv("CI_JOB_NAME") != "" {
		versions := compos.WindowsServerVersionsForE2E
		seed := os.Getenv("CI_PIPELINE_ID") + os.Getenv("CI_JOB_NAME")
		idx := pickVersionIndex(versions, seed)
		osDesc = versions[idx]
	}
	params.instanceOptions = append(params.instanceOptions, ec2.WithOS(osDesc))

	host, err := ec2.NewVM(awsEnv, params.Name, params.instanceOptions...)
	if err != nil {
		return err
	}
	if err := host.Export(ctx, env.RemoteHostOutput()); err != nil {
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
		if err := adComp.Export(ctx, env.ActiveDirectoryOutput()); err != nil {
			return err
		}

		if params.agentOptions != nil {
			// Agent install needs to happen after ActiveDirectory setup
			params.agentOptions = append(params.agentOptions,
				agentparams.WithPulumiResourceOptions(
					pulumi.DependsOn(adResources)))
		}
	} else {
		env.DisableActiveDirectory()
	}

	// Create FakeIntake if required
	if params.fakeintakeOptions != nil {
		fi, err := fakeintakescenario.NewECSFargateInstance(awsEnv, params.Name, params.fakeintakeOptions...)
		if err != nil {
			return err
		}
		if err := fi.Export(ctx, env.FakeIntakeOutput()); err != nil {
			return err
		}
		// Normally if FakeIntake is enabled, Agent is enabled, but just in case
		if params.agentOptions != nil {
			// Prepend in case it's overridden by the user
			newOpts := []agentparams.Option{agentparams.WithFakeintake(fi)}
			params.agentOptions = append(newOpts, params.agentOptions...)
		}
	} else {
		env.DisableFakeIntake()
	}

	if params.agentOptions != nil {
		agentOptions := append(params.agentOptions, agentparams.WithTags([]string{fmt.Sprintf("stackid:%s", ctx.Stack())}))
		ag, err := agent.NewHostAgent(&awsEnv, host, agentOptions...)
		if err != nil {
			return err
		}
		if err := ag.Export(ctx, env.AgentOutput()); err != nil {
			return err
		}
		env.SetAgentClientOptions(params.agentClientOptions...)
	} else {
		env.DisableAgent()
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

// pickVersionIndex returns an index in [0, len(versions)). If seed is set, hashes it for deterministic choice; otherwise uses rand.
func pickVersionIndex(versions []compos.Descriptor, seed string) int {
	n := len(versions)
	if seed == "" {
		return rand.Intn(n)
	}
	h := fnv.New64a()
	h.Write([]byte(seed))
	return int(h.Sum64() % uint64(n))
}

// Run is the entry point for the scenario when run via pulumi.
// It uses outputs.WindowsHost which is lightweight and doesn't pull in test dependencies.
func Run(ctx *pulumi.Context) error {
	awsEnv, err := aws.NewEnvironment(ctx)
	if err != nil {
		return err
	}

	env := outputs.NewWindowsHost()

	params := ParamsFromEnvironment(awsEnv)
	return RunWithEnv(ctx, awsEnv, env, params)
}
