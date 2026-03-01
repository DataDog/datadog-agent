// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

package tracegen

import (
	"github.com/DataDog/datadog-agent/test/e2e-framework/common/config"
	"github.com/DataDog/datadog-agent/test/e2e-framework/components/datadog/apps"
	ecsComp "github.com/DataDog/datadog-agent/test/e2e-framework/components/ecs"
	"github.com/DataDog/datadog-agent/test/e2e-framework/resources/aws"
	classicECS "github.com/pulumi/pulumi-aws/sdk/v6/go/aws/ecs"
	"github.com/pulumi/pulumi-awsx/sdk/v2/go/awsx/awsx"
	"github.com/pulumi/pulumi-awsx/sdk/v2/go/awsx/ecs"
	"github.com/pulumi/pulumi/sdk/v3/go/pulumi"
)

type EcsComponent struct {
	pulumi.ResourceState
}

func EcsAppDefinition(e aws.Environment, clusterArn pulumi.StringInput, opts ...pulumi.ResourceOption) (*ecsComp.Workload, error) {
	namer := e.Namer.WithPrefix("tracegen")
	opts = append(opts, e.WithProviders(config.ProviderAWS, config.ProviderAWSX))

	ecsComponent := &ecsComp.Workload{}
	if err := e.Ctx().RegisterComponentResource("dd:apps", namer.ResourceName("grp"), ecsComponent, opts...); err != nil {
		return nil, err
	}

	opts = append(opts, pulumi.Parent(ecsComponent))

	if _, err := ecs.NewEC2Service(e.Ctx(), namer.ResourceName("uds"), &ecs.EC2ServiceArgs{
		Name:                 e.CommonNamer().DisplayName(255, pulumi.String("tracegen-uds")),
		Cluster:              clusterArn,
		DesiredCount:         pulumi.IntPtr(1),
		EnableExecuteCommand: pulumi.BoolPtr(true),
		TaskDefinitionArgs: &ecs.EC2ServiceTaskDefinitionArgs{
			Containers: map[string]ecs.TaskDefinitionContainerDefinitionArgs{
				"tracegen": {
					Name:  pulumi.String("tracegen"),
					Image: pulumi.String("ghcr.io/datadog/apps-tracegen:" + apps.Version),
					Environment: ecs.TaskDefinitionKeyValuePairArray{
						ecs.TaskDefinitionKeyValuePairArgs{
							Name:  pulumi.StringPtr("DD_TRACE_AGENT_URL"),
							Value: pulumi.StringPtr("unix:///var/run/datadog/apm.socket"),
						},
						ecs.TaskDefinitionKeyValuePairArgs{
							Name:  pulumi.StringPtr("DD_SERVICE"),
							Value: pulumi.StringPtr("tracegen-test-service"),
						},
						ecs.TaskDefinitionKeyValuePairArgs{
							Name:  pulumi.StringPtr("DD_ENV"),
							Value: pulumi.StringPtr("e2e-test"),
						},
						ecs.TaskDefinitionKeyValuePairArgs{
							Name:  pulumi.StringPtr("DD_VERSION"),
							Value: pulumi.StringPtr("1.0"),
						},
						ecs.TaskDefinitionKeyValuePairArgs{
							Name:  pulumi.StringPtr("DD_LOGS_INJECTION"),
							Value: pulumi.StringPtr("true"),
						},
					},
					DockerLabels: pulumi.StringMap{
						"com.datadoghq.ad.tags":      pulumi.String("[\"ecs_launch_type:ec2\"]"),
						"com.datadoghq.ad.logs":      pulumi.String("[{\"source\":\"tracegen\",\"service\":\"tracegen-test-service\"}]"),
						"com.datadoghq.tags.service": pulumi.String("tracegen-test-service"),
						"com.datadoghq.tags.env":     pulumi.String("e2e-test"),
						"com.datadoghq.tags.version": pulumi.String("1.0"),
					},
					Cpu:    pulumi.IntPtr(10),
					Memory: pulumi.IntPtr(32),
					MountPoints: ecs.TaskDefinitionMountPointArray{
						ecs.TaskDefinitionMountPointArgs{
							SourceVolume:  pulumi.StringPtr("apmsocketpath"),
							ContainerPath: pulumi.StringPtr("/var/run/datadog"),
							ReadOnly:      pulumi.BoolPtr(true),
						},
					},
				},
			},
			ExecutionRole: &awsx.DefaultRoleWithPolicyArgs{
				RoleArn: pulumi.StringPtr(e.ECSTaskExecutionRole()),
			},
			TaskRole: &awsx.DefaultRoleWithPolicyArgs{
				RoleArn: pulumi.StringPtr(e.ECSTaskRole()),
			},
			NetworkMode: pulumi.StringPtr("none"),
			Family:      e.CommonNamer().DisplayName(255, pulumi.String("tracegen-uds-ec2")),
			Volumes: classicECS.TaskDefinitionVolumeArray{
				classicECS.TaskDefinitionVolumeArgs{
					Name:     pulumi.String("apmsocketpath"),
					HostPath: pulumi.StringPtr("/var/run/datadog"),
				},
			},
		},
	}, opts...); err != nil {
		return nil, err
	}

	if _, err := ecs.NewEC2Service(e.Ctx(), namer.ResourceName("tcp"), &ecs.EC2ServiceArgs{
		Name:                 e.CommonNamer().DisplayName(255, pulumi.String("tracegen-tcp")),
		Cluster:              clusterArn,
		DesiredCount:         pulumi.IntPtr(1),
		EnableExecuteCommand: pulumi.BoolPtr(true),
		TaskDefinitionArgs: &ecs.EC2ServiceTaskDefinitionArgs{
			Containers: map[string]ecs.TaskDefinitionContainerDefinitionArgs{
				"tracegen": {
					Name:  pulumi.String("tracegen"),
					Image: pulumi.String("ghcr.io/datadog/apps-tracegen:" + apps.Version),
					Environment: ecs.TaskDefinitionKeyValuePairArray{
						ecs.TaskDefinitionKeyValuePairArgs{
							Name:  pulumi.StringPtr("ECS_AGENT_HOST"),
							Value: pulumi.StringPtr("true"),
						},
						ecs.TaskDefinitionKeyValuePairArgs{
							Name:  pulumi.StringPtr("DD_SERVICE"),
							Value: pulumi.StringPtr("tracegen-test-service"),
						},
						ecs.TaskDefinitionKeyValuePairArgs{
							Name:  pulumi.StringPtr("DD_ENV"),
							Value: pulumi.StringPtr("e2e-test"),
						},
						ecs.TaskDefinitionKeyValuePairArgs{
							Name:  pulumi.StringPtr("DD_VERSION"),
							Value: pulumi.StringPtr("1.0"),
						},
						ecs.TaskDefinitionKeyValuePairArgs{
							Name:  pulumi.StringPtr("DD_LOGS_INJECTION"),
							Value: pulumi.StringPtr("true"),
						},
					},
					DockerLabels: pulumi.StringMap{
						"com.datadoghq.ad.tags":      pulumi.String("[\"ecs_launch_type:ec2\"]"),
						"com.datadoghq.ad.logs":      pulumi.String("[{\"source\":\"tracegen\",\"service\":\"tracegen-test-service\"}]"),
						"com.datadoghq.tags.service": pulumi.String("tracegen-test-service"),
						"com.datadoghq.tags.env":     pulumi.String("e2e-test"),
						"com.datadoghq.tags.version": pulumi.String("1.0"),
					},
					Cpu:    pulumi.IntPtr(10),
					Memory: pulumi.IntPtr(32),
				},
			},
			ExecutionRole: &awsx.DefaultRoleWithPolicyArgs{
				RoleArn: pulumi.StringPtr(e.ECSTaskExecutionRole()),
			},
			TaskRole: &awsx.DefaultRoleWithPolicyArgs{
				RoleArn: pulumi.StringPtr(e.ECSTaskRole()),
			},
			NetworkMode: pulumi.StringPtr("bridge"),
			Family:      e.CommonNamer().DisplayName(255, pulumi.String("tracegen-tcp-ec2")),
		},
	}, opts...); err != nil {
		return nil, err
	}

	return ecsComponent, nil
}
