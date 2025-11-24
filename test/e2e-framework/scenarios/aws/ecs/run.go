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
	"github.com/DataDog/datadog-agent/test/e2e-framework/scenarios/aws/fakeintake"
	"github.com/pulumi/pulumi-aws/sdk/v6/go/aws/ssm"
	"github.com/pulumi/pulumi/sdk/v3/go/pulumi"
)

func Run(ctx *pulumi.Context) error {
	awsEnv, err := resourcesAws.NewEnvironment(ctx)
	if err != nil {
		return err
	}

	// Create cluster
	ecsOpts := buildClusterOptionsFromConfigMap(awsEnv)
	cluster, err := NewCluster(awsEnv, "ecs", ecsOpts...)
	if err != nil {
		return err
	}

	err = cluster.Export(ctx, nil)
	if err != nil {
		return err
	}

	var apiKeyParam *ssm.Parameter
	var fakeIntake *fakeintakeComp.Fakeintake
	// Create task and service
	if awsEnv.AgentDeploy() {
		if awsEnv.AgentUseFakeintake() {
			fakeIntakeOptions := []fakeintake.Option{}
			if awsEnv.InfraShouldDeployFakeintakeWithLB() {
				fakeIntakeOptions = append(fakeIntakeOptions, fakeintake.WithLoadBalancer())
			}

			if storeType := awsEnv.AgentFakeintakeStoreType(); storeType != "" {
				fakeIntakeOptions = append(fakeIntakeOptions, fakeintake.WithStoreType(storeType))
			}

			if retentionPeriod := awsEnv.AgentFakeintakeRetentionPeriod(); retentionPeriod != "" {
				fakeIntakeOptions = append(fakeIntakeOptions, fakeintake.WithRetentionPeriod(retentionPeriod))
			}

			if fakeIntake, err = fakeintake.NewECSFargateInstance(awsEnv, "ecs", fakeIntakeOptions...); err != nil {
				return err
			}
			if err := fakeIntake.Export(awsEnv.Ctx(), nil); err != nil {
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

		_, err := agent.ECSLinuxDaemonDefinition(awsEnv, "ec2-linux-dd-agent", apiKeyParam.Name, fakeIntake, cluster.ClusterArn)
		if err != nil {
			return err
		}

	}

	// Deploy testing workload
	if awsEnv.TestingWorkloadDeploy() {
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

	// Deploy Fargate Agents
	if awsEnv.TestingWorkloadDeploy() && awsEnv.AgentDeploy() {
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

	return nil
}
