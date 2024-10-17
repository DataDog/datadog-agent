// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package awsdocker contains the definition of the AWS Docker environment.
package awsdocker

import (
	"fmt"

	"github.com/DataDog/datadog-agent/test/new-e2e/pkg/e2e"
	"github.com/DataDog/datadog-agent/test/new-e2e/pkg/environments"
	"github.com/DataDog/datadog-agent/test/new-e2e/pkg/runner"
	"github.com/DataDog/datadog-agent/test/new-e2e/pkg/utils/optional"

	"github.com/DataDog/test-infra-definitions/common/utils"
	"github.com/DataDog/test-infra-definitions/components/datadog/agent"
	"github.com/DataDog/test-infra-definitions/components/datadog/apps/dogstatsd"
	"github.com/DataDog/test-infra-definitions/components/datadog/dockeragentparams"
	"github.com/DataDog/test-infra-definitions/components/docker"
	"github.com/DataDog/test-infra-definitions/resources/aws"
	"github.com/DataDog/test-infra-definitions/scenarios/aws/ec2"
	"github.com/DataDog/test-infra-definitions/scenarios/aws/fakeintake"

	"github.com/pulumi/pulumi/sdk/v3/go/pulumi"
)

const (
	provisionerBaseID = "aws-ec2docker-"
	defaultVMName     = "dockervm"
)

// ProvisionerParams contains all the parameters needed to create the environment
type ProvisionerParams struct {
	name string

	vmOptions         []ec2.VMOption
	agentOptions      []dockeragentparams.Option
	fakeintakeOptions []fakeintake.Option
	extraConfigParams runner.ConfigMap
	testingWorkload   bool
}

func newProvisionerParams() *ProvisionerParams {
	// We use nil arrays to decide if we should create or not
	return &ProvisionerParams{
		name:              defaultVMName,
		vmOptions:         []ec2.VMOption{},
		agentOptions:      []dockeragentparams.Option{},
		fakeintakeOptions: []fakeintake.Option{},
		extraConfigParams: runner.ConfigMap{},
	}
}

// GetProvisionerParams return ProvisionerParams from options opts setup
func GetProvisionerParams(opts ...ProvisionerOption) *ProvisionerParams {
	params := newProvisionerParams()
	err := optional.ApplyOptions(params, opts)
	if err != nil {
		panic(fmt.Errorf("unable to apply ProvisionerOption, err: %w", err))
	}
	return params
}

// ProvisionerOption is a function that modifies the ProvisionerParams
type ProvisionerOption func(*ProvisionerParams) error

// WithName sets the name of the provisioner
func WithName(name string) ProvisionerOption {
	return func(params *ProvisionerParams) error {
		params.name = name
		return nil
	}
}

// WithEC2VMOptions sets the options for the EC2 VM
func WithEC2VMOptions(opts ...ec2.VMOption) ProvisionerOption {
	return func(params *ProvisionerParams) error {
		params.vmOptions = append(params.vmOptions, opts...)
		return nil
	}
}

// WithAgentOptions sets the options for the Docker Agent
func WithAgentOptions(opts ...dockeragentparams.Option) ProvisionerOption {
	return func(params *ProvisionerParams) error {
		params.agentOptions = append(params.agentOptions, opts...)
		return nil
	}
}

// WithFakeIntakeOptions sets the options for the FakeIntake
func WithFakeIntakeOptions(opts ...fakeintake.Option) ProvisionerOption {
	return func(params *ProvisionerParams) error {
		params.fakeintakeOptions = append(params.fakeintakeOptions, opts...)
		return nil
	}
}

// WithExtraConfigParams sets the extra config params for the environment
func WithExtraConfigParams(configMap runner.ConfigMap) ProvisionerOption {
	return func(params *ProvisionerParams) error {
		params.extraConfigParams = configMap
		return nil
	}
}

// WithoutFakeIntake deactivates the creation of the FakeIntake
func WithoutFakeIntake() ProvisionerOption {
	return func(params *ProvisionerParams) error {
		params.fakeintakeOptions = nil
		return nil
	}
}

// WithoutAgent deactivates the creation of the Docker Agent
func WithoutAgent() ProvisionerOption {
	return func(params *ProvisionerParams) error {
		params.agentOptions = nil
		return nil
	}
}

// WithTestingWorkload enables testing workload
func WithTestingWorkload() ProvisionerOption {
	return func(params *ProvisionerParams) error {
		params.testingWorkload = true
		return nil
	}
}

// RunParams contains parameters for the run function
type RunParams struct {
	Environment       *aws.Environment
	ProvisionerParams *ProvisionerParams
}

// Run deploys a docker environment given a pulumi.Context
func Run(ctx *pulumi.Context, env *environments.DockerHost, runParams RunParams) error {
	var awsEnv aws.Environment
	if runParams.Environment == nil {
		var err error
		awsEnv, err = aws.NewEnvironment(ctx)
		if err != nil {
			return err
		}
	} else {
		awsEnv = *runParams.Environment
	}

	params := runParams.ProvisionerParams

	host, err := ec2.NewVM(awsEnv, params.name, params.vmOptions...)
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
		fakeIntake, err := fakeintake.NewECSFargateInstance(awsEnv, params.name, params.fakeintakeOptions...)
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

	// Create Agent if required
	if params.agentOptions != nil {
		if params.testingWorkload {
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

// Provisioner creates a VM environment with an EC2 VM with Docker, an ECS Fargate FakeIntake and a Docker Agent configured to talk to each other.
// FakeIntake and Agent creation can be deactivated by using [WithoutFakeIntake] and [WithoutAgent] options.
func Provisioner(opts ...ProvisionerOption) e2e.TypedProvisioner[environments.DockerHost] {
	// We need to build params here to be able to use params.name in the provisioner name
	params := GetProvisionerParams(opts...)
	provisioner := e2e.NewTypedPulumiProvisioner(provisionerBaseID+params.name, func(ctx *pulumi.Context, env *environments.DockerHost) error {
		return Run(ctx, env, RunParams{ProvisionerParams: params})
	}, params.extraConfigParams)

	return provisioner
}
