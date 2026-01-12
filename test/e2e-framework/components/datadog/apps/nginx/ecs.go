// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

package nginx

import (
	"github.com/DataDog/datadog-agent/test/e2e-framework/common/config"
	"github.com/DataDog/datadog-agent/test/e2e-framework/common/utils"
	"github.com/DataDog/datadog-agent/test/e2e-framework/components/datadog/apps"
	ecsComp "github.com/DataDog/datadog-agent/test/e2e-framework/components/ecs"
	"github.com/DataDog/datadog-agent/test/e2e-framework/resources/aws"
	classicECS "github.com/pulumi/pulumi-aws/sdk/v6/go/aws/ecs"
	"github.com/pulumi/pulumi-awsx/sdk/v2/go/awsx/awsx"
	"github.com/pulumi/pulumi-awsx/sdk/v2/go/awsx/ecs"
	"github.com/pulumi/pulumi/sdk/v3/go/pulumi"
)

func EcsAppDefinition(e aws.Environment, clusterArn pulumi.StringInput, opts ...pulumi.ResourceOption) (*ecsComp.Workload, error) {
	namer := e.Namer.WithPrefix("nginx").WithPrefix("ec2")
	opts = append(opts, e.WithProviders(config.ProviderAWS, config.ProviderAWSX))

	ecsComponent := &ecsComp.Workload{}
	if err := e.Ctx().RegisterComponentResource("dd:apps", namer.ResourceName("grp"), ecsComponent, opts...); err != nil {
		return nil, err
	}

	opts = append(opts, pulumi.Parent(ecsComponent))

	if _, err := ecs.NewEC2Service(e.Ctx(), namer.ResourceName("server"), &ecs.EC2ServiceArgs{
		Name:                 e.CommonNamer().DisplayName(255, pulumi.String("nginx"), pulumi.String("ec2")),
		Cluster:              clusterArn,
		DesiredCount:         pulumi.IntPtr(2),
		EnableExecuteCommand: pulumi.BoolPtr(true),
		TaskDefinitionArgs: &ecs.EC2ServiceTaskDefinitionArgs{
			Containers: map[string]ecs.TaskDefinitionContainerDefinitionArgs{
				"nginx": {
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
						"com.datadoghq.ad.tags": pulumi.String("[\"ecs_launch_type:ec2\"]"),
					},
					Cpu:    pulumi.IntPtr(100),
					Memory: pulumi.IntPtr(96),
					MountPoints: ecs.TaskDefinitionMountPointArray{
						ecs.TaskDefinitionMountPointArgs{
							SourceVolume:  pulumi.StringPtr("cache"),
							ContainerPath: pulumi.StringPtr("/var/cache/nginx"),
						},
						ecs.TaskDefinitionMountPointArgs{
							SourceVolume:  pulumi.StringPtr("var-run"),
							ContainerPath: pulumi.StringPtr("/var/run"),
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
						Interval: pulumi.Int(30),
						Retries:  pulumi.Int(3),
						Timeout:  pulumi.Int(5),
					},
				},
				"query": {
					Name:  pulumi.String("query"),
					Image: pulumi.String("ghcr.io/datadog/apps-http-client:" + apps.Version),
					Command: pulumi.StringArray{
						pulumi.String("-url"),
						pulumi.String("http://nginx"),
					},
					Cpu:    pulumi.IntPtr(50),
					Memory: pulumi.IntPtr(32),
					Links:  pulumi.ToStringArray([]string{"nginx:nginx"}),
				},
			},
			ExecutionRole: &awsx.DefaultRoleWithPolicyArgs{
				RoleArn: pulumi.StringPtr(e.ECSTaskExecutionRole()),
			},
			TaskRole: &awsx.DefaultRoleWithPolicyArgs{
				RoleArn: pulumi.StringPtr(e.ECSTaskRole()),
			},
			NetworkMode: pulumi.StringPtr("bridge"),
			Family:      e.CommonNamer().DisplayName(255, pulumi.ToStringArray([]string{"nginx", "ec2"})...),
			Volumes: classicECS.TaskDefinitionVolumeArray{
				classicECS.TaskDefinitionVolumeArgs{
					Name: pulumi.String("cache"),
					DockerVolumeConfiguration: classicECS.TaskDefinitionVolumeDockerVolumeConfigurationArgs{
						Scope: pulumi.StringPtr("task"),
					},
				},
				classicECS.TaskDefinitionVolumeArgs{
					Name: pulumi.String("var-run"),
					DockerVolumeConfiguration: classicECS.TaskDefinitionVolumeDockerVolumeConfigurationArgs{
						Scope: pulumi.StringPtr("task"),
					},
				},
			},
		},
	}, opts...); err != nil {
		return nil, err
	}

	return ecsComponent, nil
}
