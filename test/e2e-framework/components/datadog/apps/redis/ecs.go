// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

package redis

import (
	"github.com/DataDog/datadog-agent/test/e2e-framework/common/config"
	"github.com/DataDog/datadog-agent/test/e2e-framework/components/datadog/apps"
	ecsComp "github.com/DataDog/datadog-agent/test/e2e-framework/components/ecs"
	"github.com/DataDog/datadog-agent/test/e2e-framework/resources/aws"

	"github.com/pulumi/pulumi-awsx/sdk/v2/go/awsx/awsx"
	"github.com/pulumi/pulumi-awsx/sdk/v2/go/awsx/ecs"
	"github.com/pulumi/pulumi/sdk/v3/go/pulumi"
)

type EcsComponent struct {
	pulumi.ResourceState
}

func EcsAppDefinition(e aws.Environment, clusterArn pulumi.StringInput, opts ...pulumi.ResourceOption) (*ecsComp.Workload, error) {
	namer := e.Namer.WithPrefix("redis").WithPrefix("ec2")
	opts = append(opts, e.WithProviders(config.ProviderAWS, config.ProviderAWSX))

	ecsComponent := &ecsComp.Workload{}
	if err := e.Ctx().RegisterComponentResource("dd:apps", namer.ResourceName("grp"), ecsComponent, opts...); err != nil {
		return nil, err
	}

	opts = append(opts, pulumi.Parent(ecsComponent))

	if _, err := ecs.NewEC2Service(e.Ctx(), namer.ResourceName("server"), &ecs.EC2ServiceArgs{
		Name:                 e.CommonNamer().DisplayName(255, pulumi.String("redis"), pulumi.String("ec2")),
		Cluster:              clusterArn,
		DesiredCount:         pulumi.IntPtr(2),
		EnableExecuteCommand: pulumi.BoolPtr(true),
		TaskDefinitionArgs: &ecs.EC2ServiceTaskDefinitionArgs{
			Containers: map[string]ecs.TaskDefinitionContainerDefinitionArgs{
				"redis": {
					Name:  pulumi.String("redis"),
					Image: pulumi.String("ghcr.io/datadog/redis:" + apps.Version),
					DockerLabels: pulumi.StringMap{
						"com.datadoghq.ad.tags": pulumi.String("[\"ecs_launch_type:ec2\"]"),
					},
					Command: pulumi.StringArray{
						pulumi.String("--loglevel"),
						pulumi.String("verbose"),
					},
					Cpu:    pulumi.IntPtr(100),
					Memory: pulumi.IntPtr(32),
					PortMappings: ecs.TaskDefinitionPortMappingArray{
						ecs.TaskDefinitionPortMappingArgs{
							ContainerPort: pulumi.IntPtr(6379),
							HostPort:      pulumi.IntPtr(6379),
							Protocol:      pulumi.StringPtr("tcp"),
						},
					},
				},
				"query": {
					Name:  pulumi.String("query"),
					Image: pulumi.String("ghcr.io/datadog/apps-redis-client:" + apps.Version),
					Command: pulumi.StringArray{
						pulumi.String("-addr"),
						pulumi.String("redis:6379"),
					},
					Cpu:    pulumi.IntPtr(50),
					Memory: pulumi.IntPtr(32),
					Links:  pulumi.ToStringArray([]string{"redis:redis"}),
				},
			},
			ExecutionRole: &awsx.DefaultRoleWithPolicyArgs{
				RoleArn: pulumi.StringPtr(e.ECSTaskExecutionRole()),
			},
			TaskRole: &awsx.DefaultRoleWithPolicyArgs{
				RoleArn: pulumi.StringPtr(e.ECSTaskRole()),
			},
			NetworkMode: pulumi.StringPtr("bridge"),
			Family:      e.CommonNamer().DisplayName(255, pulumi.ToStringArray([]string{"redis", "ec2"})...),
		},
	}, opts...); err != nil {
		return nil, err
	}

	return ecsComponent, nil
}
