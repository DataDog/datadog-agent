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

	resourcesAws "github.com/DataDog/datadog-agent/test/e2e-framework/resources/aws"
	resourcesEcs "github.com/DataDog/datadog-agent/test/e2e-framework/resources/aws/ecs"
	"github.com/DataDog/datadog-agent/test/e2e-framework/scenarios/aws/fakeintake"
	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/environments"
	"github.com/pulumi/pulumi-aws/sdk/v6/go/aws/ssm"
	"github.com/pulumi/pulumi/sdk/v3/go/pulumi"
)

func Run(ctx *pulumi.Context) error {
	awsEnv, err := resourcesAws.NewEnvironment(ctx)
	if err != nil {
		return err
	}

	env, _, _, err := environments.CreateEnv[environments.ECS]()
	if err != nil {
		return err
	}

	params := ParamsFromEnvironment(awsEnv)
	return RunWithEnv(ctx, awsEnv, env, params)
}

// RunWithEnv deploys an ECS environment using provided env and params
func RunWithEnv(ctx *pulumi.Context, awsEnv resourcesAws.Environment, env *environments.ECS, params *RunParams) error {
	// Create cluster
	cluster, err := NewCluster(awsEnv, params.Name, params.ecsOptions...)
	if err != nil {
		return err
	}
	if err := cluster.Export(ctx, &env.ECSCluster.ClusterOutput); err != nil {
		return err
	}

	// Read back cluster params to know if Fargate is enabled
	clusterParams, err := NewParams(params.ecsOptions...)
	if err != nil {
		return err
	}

	var apiKeyParam *ssm.Parameter
	var fakeIntake *fakeintakeComp.Fakeintake

	if params.agentOptions != nil {
		if params.fakeintakeOptions != nil {
			if fakeIntake, err = fakeintake.NewECSFargateInstance(awsEnv, "ecs", params.fakeintakeOptions...); err != nil {
				return err
			}
			if err := fakeIntake.Export(awsEnv.Ctx(), &env.FakeIntake.FakeintakeOutput); err != nil {
				return err
			}
		} else {
			env.FakeIntake = nil
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
		env.FakeIntake = nil
	}

	// Wait for container instances to be ready before deploying EC2 workloads
	// This prevents services from timing out while waiting for instances to register
	if clusterParams.LinuxNodeGroup || clusterParams.LinuxARMNodeGroup || clusterParams.LinuxBottleRocketNodeGroup || clusterParams.WindowsNodeGroup {
		ctx.Log.Info("Waiting for EC2 container instances to register with the cluster...", nil)
		_ = resourcesEcs.WaitForContainerInstances(awsEnv, cluster.ClusterArn, 2)
	}

	// Testing workload
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

	// User-defined EC2 apps
	for _, appFunc := range params.workloadAppFuncs {
		if _, err := appFunc(awsEnv, cluster.ClusterArn); err != nil {
			return err
		}
	}

	// Deploy Fargate test apps when enabled
	if params.testingWorkload && params.agentOptions != nil && clusterParams.FargateCapacityProvider {
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
