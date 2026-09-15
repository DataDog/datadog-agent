// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

package otelstandalone

import (
	classicECS "github.com/pulumi/pulumi-aws/sdk/v7/go/aws/ecs"
	"github.com/pulumi/pulumi-awsx/sdk/v3/go/awsx/awsx"
	"github.com/pulumi/pulumi-awsx/sdk/v3/go/awsx/ecs"
	"github.com/pulumi/pulumi/sdk/v3/go/pulumi"

	"github.com/DataDog/datadog-agent/test/e2e-framework/common/config"
	"github.com/DataDog/datadog-agent/test/e2e-framework/common/utils"
	ecsComp "github.com/DataDog/datadog-agent/test/e2e-framework/components/ecs"

	"github.com/DataDog/datadog-agent/test/e2e-framework/components/datadog/fakeintake"
	"github.com/DataDog/datadog-agent/test/e2e-framework/resources/aws"
	ecsResources "github.com/DataDog/datadog-agent/test/e2e-framework/resources/aws/ecs"
)

// otelConfigEnvVarName is the environment variable that carries the OTel config
// YAML. The otel-agent binary is started with `--config=env:<otelConfigEnvVarName>`,
// which resolves through the collector's built-in confmap env provider. ECS
// Fargate has no ConfigMap-equivalent primitive to mount a config file from, so
// this is used here instead of the configPath/configDir file mount that
// K8sAppDefinition uses.
const otelConfigEnvVarName = "DOGTEL_E2E_OTEL_CONFIG"

// FargateAppDefinition deploys the Datadog otel-agent as a standalone ECS
// Fargate task, without a co-located core Datadog Agent container. Use this
// with scenecs.WithFargateWorkloadApp to test DD_OTEL_STANDALONE=true behavior
// that is specific to ECS Fargate, analogous to K8sAppDefinition for Kubernetes.
func FargateAppDefinition(e aws.Environment, clusterArn pulumi.StringInput, apiKeySSMParamName pulumi.StringInput, fakeIntake *fakeintake.Fakeintake, otelConfig string, opts ...pulumi.ResourceOption) (*ecsComp.Workload, error) {
	namer := e.Namer.WithPrefix("standalone-otel-agent")
	opts = append(opts, e.WithProviders(config.ProviderAWS, config.ProviderAWSX))

	component := &ecsComp.Workload{}
	if err := e.Ctx().RegisterComponentResource("dd:apps", namer.ResourceName("grp"), component, opts...); err != nil {
		return nil, err
	}
	opts = append(opts, pulumi.Parent(component))

	image := dockerOTelAgentFullImagePath(&e)

	container := &ecs.TaskDefinitionContainerDefinitionArgs{
		Cpu:       pulumi.IntPtr(0),
		Name:      pulumi.String("otel-agent"),
		Image:     pulumi.String(image),
		Essential: pulumi.BoolPtr(true),
		Command: pulumi.StringArray{
			pulumi.String(binaryPath),
			pulumi.String("--config"),
			pulumi.String("env:" + otelConfigEnvVarName),
		},
		Environment: ecs.TaskDefinitionKeyValuePairArray{
			ecs.TaskDefinitionKeyValuePairArgs{
				Name:  pulumi.StringPtr("DD_OTEL_STANDALONE"),
				Value: pulumi.StringPtr("true"),
			},
			ecs.TaskDefinitionKeyValuePairArgs{
				Name:  pulumi.StringPtr("DD_OTELCOLLECTOR_ENABLED"),
				Value: pulumi.StringPtr("true"),
			},
			ecs.TaskDefinitionKeyValuePairArgs{
				Name:  pulumi.StringPtr("ECS_FARGATE"),
				Value: pulumi.StringPtr("true"),
			},
			ecs.TaskDefinitionKeyValuePairArgs{
				Name:  pulumi.StringPtr(otelConfigEnvVarName),
				Value: pulumi.StringPtr(otelConfig),
			},
			// Route the agent serializer (used for dogtelextension liveness metrics)
			// to the fakeintake URL, mirroring K8sAppDefinition.
			ecs.TaskDefinitionKeyValuePairArgs{
				Name:  pulumi.StringPtr("DD_DD_URL"),
				Value: fakeIntake.URL.ToStringOutput(),
			},
		},
		Secrets: ecs.TaskDefinitionSecretArray{
			ecs.TaskDefinitionSecretArgs{
				Name:      pulumi.String("DD_API_KEY"),
				ValueFrom: apiKeySSMParamName,
			},
		},
		PortMappings: ecs.TaskDefinitionPortMappingArray{},
		VolumesFrom:  ecs.TaskDefinitionVolumeFromArray{},
		// Without this the container is a black box: if the otel-agent process
		// fails to start or crashes, ECS only reports the container-level exit
		// code/reason, not why. Route stdout/stderr to Datadog logs via firelens,
		// mirroring the pattern every other Fargate app in this framework uses.
		LogConfiguration: ecsResources.GetFirelensLogConfiguration(pulumi.String("otel-agent"), pulumi.String("standalone-otel-agent"), apiKeySSMParamName),
	}

	taskDef, err := ecs.NewFargateTaskDefinition(e.Ctx(), namer.ResourceName("taskdef"), &ecs.FargateTaskDefinitionArgs{
		Containers: map[string]ecs.TaskDefinitionContainerDefinitionArgs{
			"otel-agent": *container,
			"log_router": *ecsResources.FargateFirelensContainerDefinition(),
		},
		Cpu:    pulumi.StringPtr("512"),
		Memory: pulumi.StringPtr("2048"),
		ExecutionRole: &awsx.DefaultRoleWithPolicyArgs{
			RoleArn: pulumi.StringPtr(e.ECSTaskExecutionRole()),
		},
		TaskRole: &awsx.DefaultRoleWithPolicyArgs{
			RoleArn: pulumi.StringPtr(e.ECSTaskRole()),
		},
		Family: e.CommonNamer().DisplayName(255, pulumi.String("standalone-otel-agent")),
	}, utils.MergeOptions(opts, e.WithProviders(config.ProviderAWS, config.ProviderAWSX))...)
	if err != nil {
		return nil, err
	}

	if _, err := ecs.NewFargateService(e.Ctx(), namer.ResourceName("svc"), &ecs.FargateServiceArgs{
		Name:         e.CommonNamer().DisplayName(255, pulumi.String("standalone-otel-agent")),
		Cluster:      clusterArn,
		DesiredCount: pulumi.IntPtr(1),
		NetworkConfiguration: classicECS.ServiceNetworkConfigurationArgs{
			AssignPublicIp: pulumi.BoolPtr(e.ECSServicePublicIP()),
			SecurityGroups: pulumi.ToStringArray(e.DefaultSecurityGroups()),
			Subnets:        e.RandomSubnets(),
		},
		TaskDefinition:            taskDef.TaskDefinition.Arn(),
		EnableExecuteCommand:      pulumi.BoolPtr(true),
		ContinueBeforeSteadyState: pulumi.BoolPtr(true),
	}, opts...); err != nil {
		return nil, err
	}

	return component, nil
}
