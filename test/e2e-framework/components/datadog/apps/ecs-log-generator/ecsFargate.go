// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

package ecsloggenerator

import (
	"github.com/DataDog/datadog-agent/test/e2e-framework/common/config"
	"github.com/DataDog/datadog-agent/test/e2e-framework/components/datadog/apps"
	fakeintakeComp "github.com/DataDog/datadog-agent/test/e2e-framework/components/datadog/fakeintake"
	ecsComp "github.com/DataDog/datadog-agent/test/e2e-framework/components/ecs"
	"github.com/DataDog/datadog-agent/test/e2e-framework/resources/aws"
	ecsClient "github.com/DataDog/datadog-agent/test/e2e-framework/resources/aws/ecs"

	classicECS "github.com/pulumi/pulumi-aws/sdk/v6/go/aws/ecs"
	"github.com/pulumi/pulumi-awsx/sdk/v2/go/awsx/ecs"
	"github.com/pulumi/pulumi/sdk/v3/go/pulumi"
)

// FargateAppDefinition creates a log generator test application for testing log collection in ECS Fargate.
//
// The application emits various log types to validate log pipeline functionality:
//   - Structured JSON logs
//   - Multiline stack traces
//   - Different log levels (DEBUG, INFO, WARN, ERROR)
//   - High-volume logs for sampling tests
//   - Logs with trace correlation context
//
// This is the Fargate deployment variant using awsvpc networking and Firelens for log routing.
//
// Owned by: ecs-experiences team
// Purpose: ECS E2E test infrastructure
func FargateAppDefinition(e aws.Environment, clusterArn pulumi.StringInput, apiKeySSMParamName pulumi.StringInput, fakeIntake *fakeintakeComp.Fakeintake, opts ...pulumi.ResourceOption) (*ecsComp.Workload, error) {
	namer := e.Namer.WithPrefix("ecs-log-generator").WithPrefix("fg")

	opts = append(opts, e.WithProviders(config.ProviderAWS, config.ProviderAWSX))

	EcsFargateComponent := &ecsComp.Workload{}
	if err := e.Ctx().RegisterComponentResource("dd:apps", namer.ResourceName("grp"), EcsFargateComponent, opts...); err != nil {
		return nil, err
	}

	opts = append(opts, pulumi.Parent(EcsFargateComponent))

	// Log generator container
	logGeneratorContainer := &ecs.TaskDefinitionContainerDefinitionArgs{
		Name:  pulumi.String("log-generator"),
		Image: pulumi.String("ghcr.io/datadog/apps-ecs-log-generator:" + apps.Version),
		Environment: ecs.TaskDefinitionKeyValuePairArray{
			// Log configuration
			ecs.TaskDefinitionKeyValuePairArgs{
				Name:  pulumi.StringPtr("LOG_LEVEL"),
				Value: pulumi.StringPtr("INFO"),
			},
			ecs.TaskDefinitionKeyValuePairArgs{
				Name:  pulumi.StringPtr("LOG_FORMAT"),
				Value: pulumi.StringPtr("json"), // json, text, or mixed
			},
			ecs.TaskDefinitionKeyValuePairArgs{
				Name:  pulumi.StringPtr("LOG_RATE"),
				Value: pulumi.StringPtr("10"), // logs per second
			},
			ecs.TaskDefinitionKeyValuePairArgs{
				Name:  pulumi.StringPtr("EMIT_MULTILINE"),
				Value: pulumi.StringPtr("true"), // emit stack traces
			},
			ecs.TaskDefinitionKeyValuePairArgs{
				Name:  pulumi.StringPtr("EMIT_ERRORS"),
				Value: pulumi.StringPtr("true"), // emit ERROR level logs
			},
			// Datadog configuration
			ecs.TaskDefinitionKeyValuePairArgs{
				Name:  pulumi.StringPtr("DD_SERVICE"),
				Value: pulumi.StringPtr("log-generator"),
			},
			ecs.TaskDefinitionKeyValuePairArgs{
				Name:  pulumi.StringPtr("DD_ENV"),
				Value: pulumi.StringPtr("test"),
			},
			ecs.TaskDefinitionKeyValuePairArgs{
				Name:  pulumi.StringPtr("DD_VERSION"),
				Value: pulumi.StringPtr("1.0"),
			},
			ecs.TaskDefinitionKeyValuePairArgs{
				Name:  pulumi.StringPtr("DD_LOGS_INJECTION"),
				Value: pulumi.StringPtr("true"), // Enable trace correlation
			},
		},
		DockerLabels: pulumi.StringMap{
			"com.datadoghq.ad.tags":             pulumi.String("[\"ecs_launch_type:fargate\",\"app:log-generator\"]"),
			"com.datadoghq.ad.logs":             pulumi.String("[{\"source\":\"log-generator\",\"service\":\"log-generator\"}]"),
			"com.datadoghq.tags.service":        pulumi.String("log-generator"),
			"com.datadoghq.tags.env":            pulumi.String("test"),
			"com.datadoghq.tags.version":        pulumi.String("1.0"),
			"com.datadoghq.ad.log_processing_rules": pulumi.String("[{\"type\":\"multi_line\",\"name\":\"stack_trace\",\"pattern\":\"^[\\\\s]+at\"}]"),
		},
		Cpu:       pulumi.IntPtr(256),
		Memory:    pulumi.IntPtr(512),
		Essential: pulumi.BoolPtr(true),
		DependsOn: ecs.TaskDefinitionContainerDependencyArray{
			ecs.TaskDefinitionContainerDependencyArgs{
				ContainerName: pulumi.String("datadog-agent"),
				Condition:     pulumi.String("HEALTHY"),
			},
		},
		PortMappings: ecs.TaskDefinitionPortMappingArray{
			ecs.TaskDefinitionPortMappingArgs{
				ContainerPort: pulumi.IntPtr(8080),
				Protocol:      pulumi.StringPtr("tcp"),
			},
		},
		LogConfiguration: ecsClient.GetFirelensLogConfiguration(pulumi.String("log-generator"), pulumi.String("log-generator"), apiKeySSMParamName),
	}

	// Create task definition with log generator and Datadog agent
	taskDef, err := ecsClient.FargateTaskDefinitionWithAgent(e, "ecs-log-generator-fg", pulumi.String("ecs-log-generator-fg"), 1024, 2048,
		map[string]ecs.TaskDefinitionContainerDefinitionArgs{
			"log-generator": *logGeneratorContainer,
		},
		apiKeySSMParamName,
		fakeIntake,
		"",
		opts...)
	if err != nil {
		return nil, err
	}

	if _, err := ecs.NewFargateService(e.Ctx(), namer.ResourceName("server"), &ecs.FargateServiceArgs{
		Cluster:      clusterArn,
		Name:         e.CommonNamer().DisplayName(255, pulumi.String("ecs-log-generator"), pulumi.String("fg")),
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

	return EcsFargateComponent, nil
}
