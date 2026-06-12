// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package awshost contains the definition of the AWS Host environment.
package awshost

import (
	"fmt"
	"testing"

	"github.com/DataDog/datadog-agent/test/e2e-framework/resources/aws"
	"github.com/DataDog/datadog-agent/test/e2e-framework/scenarios/aws/ec2"
	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/environments"
	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/installers"
	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/installers/hostagent"
	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/provisioners"
	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/runner"
	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/utils/optional"

	"github.com/pulumi/pulumi/sdk/v3/go/pulumi"
)

const provisionerBaseID = "aws-ec2vm-"

type ProvisionerParams struct {
	awsEnv            *aws.Environment
	extraConfigParams runner.ConfigMap
	runOptions        []ec2.Option
}

type ProvisionerOption func(*ProvisionerParams) error

func getProvisionerParams(opts ...ProvisionerOption) *ProvisionerParams {
	params := &ProvisionerParams{
		awsEnv:            nil,
		runOptions:        nil,
		extraConfigParams: runner.ConfigMap{},
	}

	err := optional.ApplyOptions(params, opts)
	if err != nil {
		panic(fmt.Errorf("unable to apply ProvisionerOption, err: %w", err))
	}
	return params
}

func WithEnv(env *aws.Environment) ProvisionerOption {
	return func(params *ProvisionerParams) error {
		params.awsEnv = env
		return nil
	}
}

func WithExtraConfigParams(configMap runner.ConfigMap) ProvisionerOption {
	return func(params *ProvisionerParams) error {
		params.extraConfigParams = configMap
		return nil
	}
}

func WithRunOptions(runOptions ...ec2.Option) ProvisionerOption {
	return func(params *ProvisionerParams) error {
		params.runOptions = append(params.runOptions, runOptions...)
		return nil
	}
}

// Provisioner creates a VM environment with an EC2 VM, an ECS Fargate FakeIntake
// and a Host Agent configured to talk to each other.
//
// Agent installation is performed via SSH after Pulumi provisions the VM and
// FakeIntake (PostProvision), rather than inside Pulumi itself. This makes
// agent reconfiguration fast (no Pulumi cycle required for agent-only changes).
//
// FakeIntake and Agent creation can be deactivated by using [WithoutFakeIntake]
// and [WithoutAgent] options inside the ec2 run options.
func Provisioner(opts ...ProvisionerOption) provisioners.TypedProvisioner[environments.Host] {
	// Extract ec2 params outside the Pulumi closure to determine whether agent
	// installation is requested and capture the user-provided agent options.
	params := getProvisionerParams(opts...)
	ec2Params := ec2.GetParams(params.runOptions...)

	// Only route through PostProvision for a regular host-agent install.
	// The updater path (installUpdater) and no-agent path stay on the
	// existing code until Phase 1f.
	usePostProvision := ec2Params.AgentOptions() != nil && !ec2Params.InstallUpdater()
	agentOpts := ec2Params.AgentOptions() // user-provided options, before Pulumi adds fakeintake

	pulumiProv := provisioners.NewTypedPulumiProvisioner(provisionerBaseID+ec2Params.Name, func(ctx *pulumi.Context, env *environments.Host) error {
		// We ALWAYS need to make a deep copy of `params`, as the provisioner can be called multiple times.
		// and it's easy to forget about it, leading to hard to debug issues.
		params := getProvisionerParams(opts...)
		runOpts := params.runOptions

		// When installing via PostProvision, suppress Pulumi agent install so
		// the VM is provisioned without the agent – the SSH installer handles it.
		if usePostProvision {
			runOpts = append(runOpts, ec2.WithoutAgent())
		}
		runParams := ec2.GetParams(runOpts...)

		var awsEnv aws.Environment
		var err error
		if params.awsEnv != nil {
			awsEnv = *params.awsEnv
		} else {
			awsEnv, err = aws.NewEnvironment(ctx)
			if err != nil {
				return err
			}
			params.awsEnv = &awsEnv
		}
		return ec2.Run(ctx, awsEnv, env, runParams)
	}, params.extraConfigParams)

	if !usePostProvision {
		return pulumiProv
	}

	return provisioners.WithPostProvision(pulumiProv, func(t *testing.T, env *environments.Host) {
		hostagent.Install(installers.FromT(t), env, agentOpts...)
	})
}

func ProvisionerNoFakeIntake(opts ...ProvisionerOption) provisioners.TypedProvisioner[environments.Host] {
	opts = append(opts, WithRunOptions(ec2.WithoutFakeIntake()))
	return Provisioner(opts...)
}

func ProvisionerNoAgentNoFakeIntake(opts ...ProvisionerOption) provisioners.TypedProvisioner[environments.Host] {
	opts = append(opts, WithRunOptions(ec2.WithoutAgent(), ec2.WithoutFakeIntake()))
	return Provisioner(opts...)
}
