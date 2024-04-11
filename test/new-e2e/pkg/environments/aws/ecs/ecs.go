// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package ecs contains the definition of the AWS ECS environment.
package ecs

import (
	"fmt"

	"github.com/pulumi/pulumi-aws/sdk/v6/go/aws/ssm"
	"github.com/pulumi/pulumi/sdk/v3/go/pulumi"

	"github.com/DataDog/test-infra-definitions/common/config"
	"github.com/DataDog/test-infra-definitions/components/datadog/agent"
	"github.com/DataDog/test-infra-definitions/components/datadog/ecsagentparams"
	fakeintakeComp "github.com/DataDog/test-infra-definitions/components/datadog/fakeintake"
	"github.com/DataDog/test-infra-definitions/resources/aws"
	"github.com/DataDog/test-infra-definitions/resources/aws/ecs"
	"github.com/DataDog/test-infra-definitions/scenarios/aws/fakeintake"

	"github.com/DataDog/datadog-agent/test/new-e2e/pkg/e2e"
	"github.com/DataDog/datadog-agent/test/new-e2e/pkg/environments"
	"github.com/DataDog/datadog-agent/test/new-e2e/pkg/runner"
	"github.com/DataDog/datadog-agent/test/new-e2e/pkg/utils/optional"
)

const (
	provisionerBaseID = "aws-ecs-"
	defaultECS        = "ecs"
)

// ProvisionerParams contains all the parameters needed to create the environment
type ProvisionerParams struct {
	name              string
	agentOptions      []ecsagentparams.Option
	fakeintakeOptions []fakeintake.Option

	extraConfigParams                 runner.ConfigMap
	ecsFargate                        bool
	ecsLinuxECSOptimizedNodeGroup     bool
	ecsLinuxECSOptimizedARMNodeGroup  bool
	ecsLinuxBottlerocketNodeGroup     bool
	ecsWindowsNodeGroup               bool
	infraShouldDeployFakeintakeWithLB bool
}

func newProvisionerParams() *ProvisionerParams {
	// We use nil arrays to decide if we should create or not
	return &ProvisionerParams{
		name:              defaultECS,
		agentOptions:      []ecsagentparams.Option{},
		fakeintakeOptions: []fakeintake.Option{},

		extraConfigParams:                 runner.ConfigMap{},
		ecsFargate:                        false,
		ecsLinuxECSOptimizedNodeGroup:     false,
		ecsLinuxECSOptimizedARMNodeGroup:  false,
		ecsLinuxBottlerocketNodeGroup:     false,
		ecsWindowsNodeGroup:               false,
		infraShouldDeployFakeintakeWithLB: false,
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

// WithAgentOptions sets the options for the Docker Agent
func WithAgentOptions(opts ...ecsagentparams.Option) ProvisionerOption {
	return func(params *ProvisionerParams) error {
		params.agentOptions = append(params.agentOptions, opts...)
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

// WithFakeIntakeOptions sets the options for the FakeIntake
func WithFakeIntakeOptions(opts ...fakeintake.Option) ProvisionerOption {
	return func(params *ProvisionerParams) error {
		params.fakeintakeOptions = append(params.fakeintakeOptions, opts...)
		return nil
	}
}

// WithECSFargateCapacityProvider enable Fargate ECS
func WithECSFargateCapacityProvider() ProvisionerOption {
	return func(params *ProvisionerParams) error {
		params.ecsFargate = true
		return nil
	}
}

// WithECSLinuxECSOptimizedNodeGroup enable aws/ecs/linuxECSOptimizedNodeGroup
func WithECSLinuxECSOptimizedNodeGroup() ProvisionerOption {
	return func(params *ProvisionerParams) error {
		params.ecsLinuxECSOptimizedNodeGroup = true
		return nil
	}
}

// WithECSLinuxECSOptimizedARMNodeGroup enable aws/ecs/linuxECSOptimizedARMNodeGroup
func WithECSLinuxECSOptimizedARMNodeGroup() ProvisionerOption {
	return func(params *ProvisionerParams) error {
		params.ecsLinuxECSOptimizedARMNodeGroup = true
		return nil
	}
}

// WithECSLinuxBottlerocketNodeGroup enable aws/ecs/linuxBottlerocketNodeGroup
func WithECSLinuxBottlerocketNodeGroup() ProvisionerOption {
	return func(params *ProvisionerParams) error {
		params.ecsLinuxBottlerocketNodeGroup = true
		return nil
	}
}

// WithECSWindowsNodeGroup enable aws/ecs/windowsLTSCNodeGroup
func WithECSWindowsNodeGroup() ProvisionerOption {
	return func(params *ProvisionerParams) error {
		params.ecsWindowsNodeGroup = true
		return nil
	}
}

// WithInfraShouldDeployFakeintakeWithLB enable load balancer on Fakeintake
func WithInfraShouldDeployFakeintakeWithLB() ProvisionerOption {
	return func(params *ProvisionerParams) error {
		params.infraShouldDeployFakeintakeWithLB = true
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

// Run deploys a ECS environment given a pulumi.Context
func Run(ctx *pulumi.Context, env *environments.ECS, params *ProvisionerParams) error {
	var awsEnv aws.Environment
	var err error
	if env.AwsEnvironment != nil {
		awsEnv = *env.AwsEnvironment
	} else {
		awsEnv, err = aws.NewEnvironment(ctx)
		if err != nil {
			return err
		}
	}
	// Create cluster
	ecsCluster, err := ecs.CreateEcsCluster(awsEnv, params.name)
	if err != nil {
		return err
	}

	// Export cluster’s properties
	ctx.Export("ecs-cluster-name", ecsCluster.Name)
	ctx.Export("ecs-cluster-arn", ecsCluster.Arn)
	env.ClusterName = ecsCluster.Name
	env.ClusterArn = ecsCluster.Arn

	// Handle capacity providers
	capacityProviders := pulumi.StringArray{}
	if params.ecsFargate {
		capacityProviders = append(capacityProviders, pulumi.String("FARGATE"))
	}

	linuxNodeGroupPresent := false
	if params.ecsLinuxECSOptimizedNodeGroup {
		cpName, err := ecs.NewECSOptimizedNodeGroup(awsEnv, ecsCluster.Name, false)
		if err != nil {
			return err
		}

		capacityProviders = append(capacityProviders, cpName)
		linuxNodeGroupPresent = true
	}

	if params.ecsLinuxECSOptimizedARMNodeGroup {
		cpName, err := ecs.NewECSOptimizedNodeGroup(awsEnv, ecsCluster.Name, true)
		if err != nil {
			return err
		}

		capacityProviders = append(capacityProviders, cpName)
		linuxNodeGroupPresent = true
	}

	if params.ecsLinuxBottlerocketNodeGroup {
		cpName, err := ecs.NewBottlerocketNodeGroup(awsEnv, ecsCluster.Name)
		if err != nil {
			return err
		}

		capacityProviders = append(capacityProviders, cpName)
		linuxNodeGroupPresent = true
	}

	if params.ecsWindowsNodeGroup {
		cpName, err := ecs.NewWindowsNodeGroup(awsEnv, ecsCluster.Name)
		if err != nil {
			return err
		}

		capacityProviders = append(capacityProviders, cpName)
	}

	// Associate capacity providers
	_, err = ecs.NewClusterCapacityProvider(awsEnv, ctx.Stack(), ecsCluster.Name, capacityProviders)
	if err != nil {
		return err
	}

	var apiKeyParam *ssm.Parameter
	var fakeIntake *fakeintakeComp.Fakeintake
	// Create task and service
	if params.agentOptions != nil {
		if params.fakeintakeOptions != nil {
			fakeIntakeOptions := []fakeintake.Option{}
			fakeIntakeOptions = append(fakeIntakeOptions, params.fakeintakeOptions...)
			if awsEnv.InfraShouldDeployFakeintakeWithLB() {
				fakeIntakeOptions = append(fakeIntakeOptions, fakeintake.WithLoadBalancer())
			}

			if fakeIntake, err = fakeintake.NewECSFargateInstance(awsEnv, "ecs", fakeIntakeOptions...); err != nil {
				return err
			}
			if err := fakeIntake.Export(awsEnv.Ctx, &env.FakeIntake.FakeintakeOutput); err != nil {
				return err
			}
		}
		apiKeyParam, err = ssm.NewParameter(ctx, awsEnv.Namer.ResourceName("agent-apikey"), &ssm.ParameterArgs{
			Name:      awsEnv.CommonNamer.DisplayName(1011, pulumi.String("agent-apikey")),
			Type:      ssm.ParameterTypeSecureString,
			Overwrite: pulumi.Bool(true),
			Value:     awsEnv.AgentAPIKey(),
		}, awsEnv.WithProviders(config.ProviderAWS))
		if err != nil {
			return err
		}

		// Deploy EC2 Agent
		if linuxNodeGroupPresent {
			agentDaemon, err := agent.ECSLinuxDaemonDefinition(awsEnv, "ec2-linux-dd-agent", apiKeyParam.Name, fakeIntake, ecsCluster.Arn, params.agentOptions...)
			if err != nil {
				return err
			}

			ctx.Export("agent-ec2-linux-task-arn", agentDaemon.TaskDefinition.Arn())
			ctx.Export("agent-ec2-linux-task-family", agentDaemon.TaskDefinition.Family())
			ctx.Export("agent-ec2-linux-task-version", agentDaemon.TaskDefinition.Revision())
		}
	}

	return nil
}

// Provisioner creates a VM environment with an EC2 VM with Docker, an ECS Fargate FakeIntake and a Docker Agent configured to talk to each other.
// FakeIntake and Agent creation can be deactivated by using [WithoutFakeIntake] and [WithoutAgent] options.
func Provisioner(opts ...ProvisionerOption) e2e.TypedProvisioner[environments.ECS] {
	// We need to build params here to be able to use params.name in the provisioner name
	params := GetProvisionerParams(opts...)
	provisioner := e2e.NewTypedPulumiProvisioner(provisionerBaseID+params.name, func(ctx *pulumi.Context, env *environments.ECS) error {
		return Run(ctx, env, params)
	}, params.extraConfigParams)

	return provisioner
}
