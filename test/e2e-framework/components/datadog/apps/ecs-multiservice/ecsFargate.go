// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

package ecsmultiservice

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

// FargateAppDefinition creates a multi-service test application for testing distributed tracing with 3 tiers:
//   - frontend: web service that receives requests and calls backend
//   - backend: API service that processes requests and queries database
//   - database: simulated database service
//
// All services emit traces via the Datadog agent sidecar and produce correlated logs.
// This is the Fargate deployment variant using awsvpc networking and TCP for trace submission.
//
// Owned by: ecs-experiences team
// Purpose: ECS E2E test infrastructure
func FargateAppDefinition(e aws.Environment, clusterArn pulumi.StringInput, apiKeySSMParamName pulumi.StringInput, fakeIntake *fakeintakeComp.Fakeintake, opts ...pulumi.ResourceOption) (*ecsComp.Workload, error) {
	namer := e.Namer.WithPrefix("ecs-multiservice").WithPrefix("fg")

	opts = append(opts, e.WithProviders(config.ProviderAWS, config.ProviderAWSX))

	EcsFargateComponent := &ecsComp.Workload{}
	if err := e.Ctx().RegisterComponentResource("dd:apps", namer.ResourceName("grp"), EcsFargateComponent, opts...); err != nil {
		return nil, err
	}

	opts = append(opts, pulumi.Parent(EcsFargateComponent))

	// Frontend container
	frontendContainer := &ecs.TaskDefinitionContainerDefinitionArgs{
		Name:  pulumi.String("frontend"),
		Image: pulumi.String("ghcr.io/datadog/apps-ecs-multiservice-frontend:" + apps.Version),
		Environment: ecs.TaskDefinitionKeyValuePairArray{
			ecs.TaskDefinitionKeyValuePairArgs{
				Name:  pulumi.StringPtr("DD_SERVICE"),
				Value: pulumi.StringPtr("frontend"),
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
				Name:  pulumi.StringPtr("DD_TRACE_AGENT_URL"),
				Value: pulumi.StringPtr("http://localhost:8126"),
			},
			ecs.TaskDefinitionKeyValuePairArgs{
				Name:  pulumi.StringPtr("BACKEND_URL"),
				Value: pulumi.StringPtr("http://localhost:8081"),
			},
			ecs.TaskDefinitionKeyValuePairArgs{
				Name:  pulumi.StringPtr("DD_LOGS_INJECTION"),
				Value: pulumi.StringPtr("true"),
			},
		},
		DockerLabels: pulumi.StringMap{
			"com.datadoghq.ad.tags": pulumi.String("[\"ecs_launch_type:fargate\",\"tier:frontend\"]"),
			"com.datadoghq.ad.logs": pulumi.String("[{\"source\":\"frontend\",\"service\":\"frontend\"}]"),
		},
		Cpu:       pulumi.IntPtr(256),
		Memory:    pulumi.IntPtr(256),
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
		LogConfiguration: ecsClient.GetFirelensLogConfiguration(pulumi.String("frontend"), pulumi.String("frontend"), apiKeySSMParamName),
	}

	// Backend container
	backendContainer := &ecs.TaskDefinitionContainerDefinitionArgs{
		Name:  pulumi.String("backend"),
		Image: pulumi.String("ghcr.io/datadog/apps-ecs-multiservice-backend:" + apps.Version),
		Environment: ecs.TaskDefinitionKeyValuePairArray{
			ecs.TaskDefinitionKeyValuePairArgs{
				Name:  pulumi.StringPtr("DD_SERVICE"),
				Value: pulumi.StringPtr("backend"),
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
				Name:  pulumi.StringPtr("DD_TRACE_AGENT_URL"),
				Value: pulumi.StringPtr("http://localhost:8126"),
			},
			ecs.TaskDefinitionKeyValuePairArgs{
				Name:  pulumi.StringPtr("DATABASE_URL"),
				Value: pulumi.StringPtr("http://localhost:8082"),
			},
			ecs.TaskDefinitionKeyValuePairArgs{
				Name:  pulumi.StringPtr("DD_LOGS_INJECTION"),
				Value: pulumi.StringPtr("true"),
			},
		},
		DockerLabels: pulumi.StringMap{
			"com.datadoghq.ad.tags": pulumi.String("[\"ecs_launch_type:fargate\",\"tier:backend\"]"),
			"com.datadoghq.ad.logs": pulumi.String("[{\"source\":\"backend\",\"service\":\"backend\"}]"),
		},
		Cpu:       pulumi.IntPtr(256),
		Memory:    pulumi.IntPtr(256),
		Essential: pulumi.BoolPtr(true),
		DependsOn: ecs.TaskDefinitionContainerDependencyArray{
			ecs.TaskDefinitionContainerDependencyArgs{
				ContainerName: pulumi.String("datadog-agent"),
				Condition:     pulumi.String("HEALTHY"),
			},
		},
		PortMappings: ecs.TaskDefinitionPortMappingArray{
			ecs.TaskDefinitionPortMappingArgs{
				ContainerPort: pulumi.IntPtr(8081),
				Protocol:      pulumi.StringPtr("tcp"),
			},
		},
		LogConfiguration: ecsClient.GetFirelensLogConfiguration(pulumi.String("backend"), pulumi.String("backend"), apiKeySSMParamName),
	}

	// Database container
	databaseContainer := &ecs.TaskDefinitionContainerDefinitionArgs{
		Name:  pulumi.String("database"),
		Image: pulumi.String("ghcr.io/datadog/apps-ecs-multiservice-database:" + apps.Version),
		Environment: ecs.TaskDefinitionKeyValuePairArray{
			ecs.TaskDefinitionKeyValuePairArgs{
				Name:  pulumi.StringPtr("DD_SERVICE"),
				Value: pulumi.StringPtr("database"),
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
				Name:  pulumi.StringPtr("DD_TRACE_AGENT_URL"),
				Value: pulumi.StringPtr("http://localhost:8126"),
			},
			ecs.TaskDefinitionKeyValuePairArgs{
				Name:  pulumi.StringPtr("DD_LOGS_INJECTION"),
				Value: pulumi.StringPtr("true"),
			},
		},
		DockerLabels: pulumi.StringMap{
			"com.datadoghq.ad.tags": pulumi.String("[\"ecs_launch_type:fargate\",\"tier:database\"]"),
			"com.datadoghq.ad.logs": pulumi.String("[{\"source\":\"database\",\"service\":\"database\"}]"),
		},
		Cpu:       pulumi.IntPtr(256),
		Memory:    pulumi.IntPtr(256),
		Essential: pulumi.BoolPtr(true),
		DependsOn: ecs.TaskDefinitionContainerDependencyArray{
			ecs.TaskDefinitionContainerDependencyArgs{
				ContainerName: pulumi.String("datadog-agent"),
				Condition:     pulumi.String("HEALTHY"),
			},
		},
		PortMappings: ecs.TaskDefinitionPortMappingArray{
			ecs.TaskDefinitionPortMappingArgs{
				ContainerPort: pulumi.IntPtr(8082),
				Protocol:      pulumi.StringPtr("tcp"),
			},
		},
		LogConfiguration: ecsClient.GetFirelensLogConfiguration(pulumi.String("database"), pulumi.String("database"), apiKeySSMParamName),
	}

	// Create task definition with all three services plus the Datadog agent
	taskDef, err := ecsClient.FargateTaskDefinitionWithAgent(e, "ecs-multiservice-fg", pulumi.String("ecs-multiservice-fg"), 2048, 4096,
		map[string]ecs.TaskDefinitionContainerDefinitionArgs{
			"frontend": *frontendContainer,
			"backend":  *backendContainer,
			"database": *databaseContainer,
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
		Name:         e.CommonNamer().DisplayName(255, pulumi.String("ecs-multiservice"), pulumi.String("fg")),
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
