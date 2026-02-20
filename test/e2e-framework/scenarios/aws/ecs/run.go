// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

package ecs

import (
	"github.com/DataDog/datadog-agent/test/e2e-framework/common/config"
	"github.com/DataDog/datadog-agent/test/e2e-framework/components/datadog/agent"
	"github.com/DataDog/datadog-agent/test/e2e-framework/components/datadog/apps/aspnetsample"
	"github.com/DataDog/datadog-agent/test/e2e-framework/components/datadog/apps/cpustress"
	"github.com/DataDog/datadog-agent/test/e2e-framework/components/datadog/apps/dogstatsd"
	"github.com/DataDog/datadog-agent/test/e2e-framework/components/datadog/apps/nginx"
	"github.com/DataDog/datadog-agent/test/e2e-framework/components/datadog/apps/prometheus"
	"github.com/DataDog/datadog-agent/test/e2e-framework/components/datadog/apps/redis"
	"github.com/DataDog/datadog-agent/test/e2e-framework/components/datadog/apps/tracegen"
	fakeintakeComp "github.com/DataDog/datadog-agent/test/e2e-framework/components/datadog/fakeintake"

	"github.com/pulumi/pulumi-aws/sdk/v6/go/aws/ssm"
	"github.com/pulumi/pulumi/sdk/v3/go/pulumi"

	resourcesAws "github.com/DataDog/datadog-agent/test/e2e-framework/resources/aws"
	resourcesEcs "github.com/DataDog/datadog-agent/test/e2e-framework/resources/aws/ecs"
	"github.com/DataDog/datadog-agent/test/e2e-framework/scenarios/aws/fakeintake"
	"github.com/DataDog/datadog-agent/test/e2e-framework/scenarios/outputs"
)

// isEC2ProviderSet checks whether at least one EC2 capacity provider is set in the given params
// An EC2 provider is considered set if at least one of its node groups is enabled.
func isEC2ProviderSet(params *Params) bool {
	return params.LinuxNodeGroup || params.LinuxARMNodeGroup || params.WindowsNodeGroup || params.LinuxBottleRocketNodeGroup
}

// Run is the entry point for the scenario when run via pulumi.
// It uses outputs.ECS which is lightweight and doesn't pull in test dependencies.
func Run(ctx *pulumi.Context) error {
	awsEnv, err := resourcesAws.NewEnvironment(ctx)
	if err != nil {
		return err
	}

	env := outputs.NewECS()

	params := ParamsFromEnvironment(awsEnv)
	return RunWithEnv(ctx, awsEnv, env, params)
}

// RunWithEnv deploys an ECS environment using provided env and params.
// It accepts ECSOutputs interface, enabling reuse between provisioners and direct Pulumi runs.
func RunWithEnv(ctx *pulumi.Context, awsEnv resourcesAws.Environment, env outputs.ECSOutputs, params *RunParams) error {
	// Create cluster
	cluster, err := NewCluster(awsEnv, params.Name, params.ecsOptions...)
	if err != nil {
		return err
	}
	if err := cluster.Export(ctx, env.ECSClusterOutput()); err != nil {
		return err
	}

	// Read back cluster params to know if Fargate is enabled
	clusterParams, err := NewParams(params.ecsOptions...)
	if err != nil {
		return err
	}

	var apiKeyParam *ssm.Parameter
	var fakeIntake *fakeintakeComp.Fakeintake

	if awsEnv.AgentDeploy() {
		if params.fakeintakeOptions != nil {
			if fakeIntake, err = fakeintake.NewECSFargateInstance(awsEnv, "ecs", params.fakeintakeOptions...); err != nil {
				return err
			}
			if err := fakeIntake.Export(awsEnv.Ctx(), env.FakeIntakeOutput()); err != nil {
				return err
			}
		} else {
			env.DisableFakeIntake()
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

		if _, err := agent.ECSLinuxDaemonDefinition(awsEnv, "ec2-linux-dd-agent", apiKeyParam.Name, fakeIntake, cluster.ClusterArn, params.agentOptions...); err != nil {
			return err
		}

		// Fargate workloads provided explicitly (only if capacity provider is enabled)
		if clusterParams.FargateCapacityProvider {
			for _, fargateApp := range params.fargateWorkloadAppFuncs {
				if _, err := fargateApp(awsEnv, cluster.ClusterArn, apiKeyParam.Name, fakeIntake); err != nil {
					return err
				}
			}
		}
	} else {
		env.DisableFakeIntake()
	}

	// Wait for container instances to be ready before deploying EC2 workloads
	// This prevents services from timing out while waiting for instances to register
	if isEC2ProviderSet(clusterParams) {
		ctx.Log.Info("Waiting for EC2 container instances to register with the cluster...", nil)
		_ = resourcesEcs.WaitForContainerInstances(awsEnv, cluster.ClusterArn, 2)
	}

	// Testing workload if at least one EC2 node group is present
	if params.testingWorkload && isEC2ProviderSet(clusterParams) {
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

	// User-defined EC2 apps
	for _, appFunc := range params.workloadAppFuncs {
		if _, err := appFunc(awsEnv, cluster.ClusterArn); err != nil {
			return err
		}
	}

	// Deploy Fargate test apps when enabled
	if params.testingWorkload && clusterParams.FargateCapacityProvider {
		if _, err := redis.FargateAppDefinition(awsEnv, cluster.ClusterArn, apiKeyParam.Name, fakeIntake); err != nil {
			return err
		}
		if _, err := nginx.FargateAppDefinition(awsEnv, cluster.ClusterArn, apiKeyParam.Name, fakeIntake); err != nil {
			return err
		}
		if _, err := aspnetsample.FargateAppDefinition(awsEnv, cluster.ClusterArn, apiKeyParam.Name, fakeIntake); err != nil {
			return err
		}
	}

	return nil
}
