// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

package nginx

import (
	"github.com/DataDog/datadog-agent/test/e2e-framework/common/config"
	"github.com/DataDog/datadog-agent/test/e2e-framework/common/utils"
	"github.com/DataDog/datadog-agent/test/e2e-framework/components/datadog/apps"
	fakeintakeComp "github.com/DataDog/datadog-agent/test/e2e-framework/components/datadog/fakeintake"
	ecsComp "github.com/DataDog/datadog-agent/test/e2e-framework/components/ecs"
	"github.com/DataDog/datadog-agent/test/e2e-framework/resources/aws"
	ecsClient "github.com/DataDog/datadog-agent/test/e2e-framework/resources/aws/ecs"
	classicECS "github.com/pulumi/pulumi-aws/sdk/v6/go/aws/ecs"
	"github.com/pulumi/pulumi-awsx/sdk/v2/go/awsx/ecs"
	"github.com/pulumi/pulumi/sdk/v3/go/pulumi"
)

func FargateAppDefinition(e aws.Environment, clusterArn pulumi.StringInput, apiKeySSMParamName pulumi.StringInput, fakeIntake *fakeintakeComp.Fakeintake, opts ...pulumi.ResourceOption) (*ecsComp.Workload, error) {
	namer := e.Namer.WithPrefix("nginx").WithPrefix("fg")

	opts = append(opts, e.WithProviders(config.ProviderAWS, config.ProviderAWSX))

	EcsFargateComponent := &ecsComp.Workload{}
	if err := e.Ctx().RegisterComponentResource("dd:apps", namer.ResourceName("grp"), EcsFargateComponent, opts...); err != nil {
		return nil, err
	}

	opts = append(opts, pulumi.Parent(EcsFargateComponent))

	serverContainer := &ecs.TaskDefinitionContainerDefinitionArgs{
		Name:  pulumi.String("nginx"),
		Image: pulumi.String("ghcr.io/datadog/apps-nginx-server:" + apps.Version),
		DockerLabels: pulumi.StringMap{
			"com.datadoghq.ad.checks": pulumi.String(utils.JSONMustMarshal(
				map[string]interface{}{
					"nginx": map[string]interface{}{
						"init_config": map[string]interface{}{},
						"instances": []map[string]interface{}{
							{
								"nginx_status_url": "http://%%host%%/nginx_status",
							},
						},
					},
				},
			)),
			"com.datadoghq.ad.tags": pulumi.String("[\"ecs_launch_type:fargate\"]"),
		},
		Cpu:       pulumi.IntPtr(100),
		Memory:    pulumi.IntPtr(96),
		Essential: pulumi.BoolPtr(true),
		DependsOn: ecs.TaskDefinitionContainerDependencyArray{
			ecs.TaskDefinitionContainerDependencyArgs{
				ContainerName: pulumi.String("datadog-agent"),
				Condition:     pulumi.String("HEALTHY"),
			},
		},
		PortMappings: ecs.TaskDefinitionPortMappingArray{
			ecs.TaskDefinitionPortMappingArgs{
				ContainerPort: pulumi.IntPtr(80),
				HostPort:      pulumi.IntPtr(80),
				Protocol:      pulumi.StringPtr("tcp"),
			},
		},
		HealthCheck: ecs.TaskDefinitionHealthCheckArgs{
			Command: pulumi.StringArray{
				pulumi.String("CMD-SHELL"),
				pulumi.String("apk add curl && curl --fail http://localhost || exit 1"),
			},
		},
		LogConfiguration: ecsClient.GetFirelensLogConfiguration(pulumi.String("nginx"), pulumi.String("nginx"), apiKeySSMParamName),
	}

	queryContainer := &ecs.TaskDefinitionContainerDefinitionArgs{
		Name:  pulumi.String("query"),
		Image: pulumi.String("ghcr.io/datadog/apps-http-client:" + apps.Version),
		Command: pulumi.StringArray{
			pulumi.String("-url"),
			pulumi.String("http://localhost"),
		},
		Cpu:       pulumi.IntPtr(50),
		Memory:    pulumi.IntPtr(32),
		Essential: pulumi.BoolPtr(true),
	}

	serverTaskDef, err := ecsClient.FargateTaskDefinitionWithAgent(e, "nginx-fg", pulumi.String("nginx-fg"), 1024, 2048,
		map[string]ecs.TaskDefinitionContainerDefinitionArgs{
			"nginx": *serverContainer,
			"query": *queryContainer,
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
		Name:         e.CommonNamer().DisplayName(255, pulumi.String("nginx"), pulumi.String("fg")),
		DesiredCount: pulumi.IntPtr(1),
		NetworkConfiguration: classicECS.ServiceNetworkConfigurationArgs{
			AssignPublicIp: pulumi.BoolPtr(e.ECSServicePublicIP()),
			SecurityGroups: pulumi.ToStringArray(e.DefaultSecurityGroups()),
			Subnets:        e.RandomSubnets(),
		},
		TaskDefinition:            serverTaskDef.TaskDefinition.Arn(),
		EnableExecuteCommand:      pulumi.BoolPtr(true),
		ContinueBeforeSteadyState: pulumi.BoolPtr(true),
	}, opts...); err != nil {
		return nil, err
	}

	return EcsFargateComponent, nil
}
