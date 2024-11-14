// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package ecs contains the definition of the AWS ECS environment.
package ecs

import (
	"fmt"

	"github.com/DataDog/test-infra-definitions/common/config"
	"github.com/pulumi/pulumi-aws/sdk/v6/go/aws/ssm"
	"github.com/pulumi/pulumi/sdk/v3/go/pulumi"

	"github.com/DataDog/test-infra-definitions/components/datadog/agent"
	"github.com/DataDog/test-infra-definitions/components/datadog/apps/aspnetsample"
	"github.com/DataDog/test-infra-definitions/components/datadog/apps/cpustress"
	"github.com/DataDog/test-infra-definitions/components/datadog/apps/dogstatsd"
	"github.com/DataDog/test-infra-definitions/components/datadog/apps/nginx"
	"github.com/DataDog/test-infra-definitions/components/datadog/apps/prometheus"
	"github.com/DataDog/test-infra-definitions/components/datadog/apps/redis"
	"github.com/DataDog/test-infra-definitions/components/datadog/apps/tracegen"
	"github.com/DataDog/test-infra-definitions/components/datadog/ecsagentparams"
	fakeintakeComp "github.com/DataDog/test-infra-definitions/components/datadog/fakeintake"
	ecsComp "github.com/DataDog/test-infra-definitions/components/ecs"
	"github.com/DataDog/test-infra-definitions/resources/aws"
	"github.com/DataDog/test-infra-definitions/scenarios/aws/ecs"
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
	ecsOptions        []ecs.Option

	extraConfigParams                 runner.ConfigMap
	infraShouldDeployFakeintakeWithLB bool
	testingWorkload                   bool
	workloadAppFuncs                  []WorkloadAppFunc
	fargateWorkloadAppFuncs           []FargateWorkloadAppFunc
	awsEnv                            *aws.Environment
}

func newProvisionerParams() *ProvisionerParams {
	// We use nil arrays to decide if we should create or not
	return &ProvisionerParams{
		name:              defaultECS,
		agentOptions:      []ecsagentparams.Option{},
		fakeintakeOptions: []fakeintake.Option{},
		ecsOptions:        []ecs.Option{},

		extraConfigParams:                 runner.ConfigMap{},
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

// WithECSOptions sets the options for ECS cluster
func WithECSOptions(opts ...ecs.Option) ProvisionerOption {
	return func(params *ProvisionerParams) error {
		params.ecsOptions = append(params.ecsOptions, opts...)
		return nil
	}
}

// WithTestingWorkload deploys testing workloads for nginx, redis, cpustress, dogstatsd, prometheus and tracegen
func WithTestingWorkload() ProvisionerOption {
	return func(params *ProvisionerParams) error {
		params.testingWorkload = true
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

// WithAwsEnv asks the provisioner to use the given environment, it is created otherwise
func WithAwsEnv(env *aws.Environment) ProvisionerOption {
	return func(params *ProvisionerParams) error {
		params.awsEnv = env
		return nil
	}
}

// WorkloadAppFunc is a function that deploys a workload app to an ECS cluster
type WorkloadAppFunc func(e aws.Environment, clusterArn pulumi.StringInput) (*ecsComp.Workload, error)

// WithWorkloadApp adds a workload app to the environment
func WithWorkloadApp(appFunc WorkloadAppFunc) ProvisionerOption {
	return func(params *ProvisionerParams) error {
		params.workloadAppFuncs = append(params.workloadAppFuncs, appFunc)
		return nil
	}
}

// FargateWorkloadAppFunc is a function that deploys a Fargate workload app to an ECS cluster
type FargateWorkloadAppFunc func(e aws.Environment, clusterArn pulumi.StringInput, apiKeySSMParamName pulumi.StringInput, fakeIntake *fakeintakeComp.Fakeintake) (*ecsComp.Workload, error)

// WithFargateWorkloadApp adds a Fargate workload app to the environment
func WithFargateWorkloadApp(appFunc FargateWorkloadAppFunc) ProvisionerOption {
	return func(params *ProvisionerParams) error {
		params.fargateWorkloadAppFuncs = append(params.fargateWorkloadAppFuncs, appFunc)
		return nil
	}
}

// Run deploys a ECS environment given a pulumi.Context
func Run(ctx *pulumi.Context, env *environments.ECS, params *ProvisionerParams) error {
	var awsEnv aws.Environment
	var err error
	if params.awsEnv != nil {
		awsEnv = *params.awsEnv
	} else {
		awsEnv, err = aws.NewEnvironment(ctx)
		if err != nil {
			return err
		}
	}
	clusterParams, err := ecs.NewParams(params.ecsOptions...)
	if err != nil {
		return err
	}

	// Create cluster
	cluster, err := ecs.NewCluster(awsEnv, params.name, params.ecsOptions...)
	if err != nil {
		return err
	}
	err = cluster.Export(ctx, &env.ECSCluster.ClusterOutput)
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
			if err := fakeIntake.Export(awsEnv.Ctx(), &env.FakeIntake.FakeintakeOutput); err != nil {
				return err
			}
		}

		apiKeyParam, err = ssm.NewParameter(ctx, awsEnv.Namer.ResourceName("agent-apikey"), &ssm.ParameterArgs{
			Name:      awsEnv.CommonNamer().DisplayName(1011, pulumi.String("agent-apikey")),
			Type:      ssm.ParameterTypeSecureString,
			Overwrite: pulumi.Bool(true),
			Value:     awsEnv.AgentAPIKey(),
		}, awsEnv.WithProviders(config.ProviderAWS))
		if err != nil {
			return err
		}

		_, err := agent.ECSLinuxDaemonDefinition(awsEnv, "ec2-linux-dd-agent", apiKeyParam.Name, fakeIntake, cluster.ClusterArn, params.agentOptions...)
		if err != nil {
			return err
		}

		// Deploy Fargate Apps
		if clusterParams.FargateCapacityProvider {
			for _, fargateAppFunc := range params.fargateWorkloadAppFuncs {
				_, err := fargateAppFunc(awsEnv, cluster.ClusterArn, apiKeyParam.Name, fakeIntake)
				if err != nil {
					return err
				}
			}
		}
	}

	if params.testingWorkload {
		if _, err := nginx.EcsAppDefinition(awsEnv, cluster.ClusterArn); err != nil {
			return err
		}

		if _, err := redis.EcsAppDefinition(awsEnv, cluster.ClusterArn); err != nil {
			return err
		}

		if _, err := cpustress.EcsAppDefinition(awsEnv, cluster.ClusterArn); err != nil {
			return err
		}

		if _, err := dogstatsd.EcsAppDefinition(awsEnv, cluster.ClusterArn); err != nil {
			return err
		}

		if _, err := prometheus.EcsAppDefinition(awsEnv, cluster.ClusterArn); err != nil {
			return err
		}

		if _, err := tracegen.EcsAppDefinition(awsEnv, cluster.ClusterArn); err != nil {
			return err
		}
	}

	if clusterParams.FargateCapacityProvider && params.testingWorkload && params.agentOptions != nil {

		if _, err := redis.FargateAppDefinition(awsEnv, cluster.ClusterArn, apiKeyParam.Name, fakeIntake); err != nil {
			return err
		}

		if _, err = nginx.FargateAppDefinition(awsEnv, cluster.ClusterArn, apiKeyParam.Name, fakeIntake); err != nil {
			return err
		}

		if _, err = aspnetsample.FargateAppDefinition(awsEnv, cluster.ClusterArn, apiKeyParam.Name, fakeIntake); err != nil {
			return err
		}
	}
	for _, appFunc := range params.workloadAppFuncs {
		_, err := appFunc(awsEnv, cluster.ClusterArn)
		if err != nil {
			return err
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
