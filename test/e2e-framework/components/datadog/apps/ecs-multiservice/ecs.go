// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

// Package ecsmultiservice provides a multi-service test application for ECS E2E testing.
//
// This package is owned by the ecs-experiences team and provides test infrastructure
// for validating distributed tracing functionality in ECS environments.
//
// Purpose:
//   - Test multi-service trace propagation across ECS containers
//   - Validate trace-log correlation in ECS deployments
//   - Verify ECS metadata enrichment on traces
//   - Test both ECS EC2 (daemon mode) and ECS Fargate (sidecar mode)
//
// Architecture:
//
//	Frontend (port 8080) → Backend (port 8080) → Database (port 8080)
//	All services emit traces with Datadog tracing libraries
//
// Do NOT use this for:
//   - Production workloads
//   - APM product feature testing (use APM-owned test apps)
//   - Performance benchmarking
//
// See README.md for detailed documentation.
package ecsmultiservice

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

// EcsAppDefinition creates a multi-service test application for testing distributed tracing with 3 tiers:
//   - frontend: web service that receives requests and calls backend
//   - backend: API service that processes requests and queries database
//   - database: simulated database service
//
// All services emit traces with Datadog tracing and produce correlated logs.
// This is the EC2 deployment variant using bridge networking and UDS for trace submission.
//
// Owned by: ecs-experiences team
// Purpose: ECS E2E test infrastructure
func EcsAppDefinition(e aws.Environment, clusterArn pulumi.StringInput, opts ...pulumi.ResourceOption) (*ecsComp.Workload, error) {
	namer := e.Namer.WithPrefix("ecs-multiservice").WithPrefix("ec2")
	opts = append(opts, e.WithProviders(config.ProviderAWS, config.ProviderAWSX))

	ecsComponent := &ecsComp.Workload{}
	if err := e.Ctx().RegisterComponentResource("dd:apps", namer.ResourceName("grp"), ecsComponent, opts...); err != nil {
		return nil, err
	}

	opts = append(opts, pulumi.Parent(ecsComponent))

	// Create the multi-service application
	if _, err := ecs.NewEC2Service(e.Ctx(), namer.ResourceName("server"), &ecs.EC2ServiceArgs{
		Name:                 e.CommonNamer().DisplayName(255, pulumi.String("ecs-multiservice"), pulumi.String("ec2")),
		Cluster:              clusterArn,
		DesiredCount:         pulumi.IntPtr(1),
		EnableExecuteCommand: pulumi.BoolPtr(true),
		TaskDefinitionArgs: &ecs.EC2ServiceTaskDefinitionArgs{
			Containers: map[string]ecs.TaskDefinitionContainerDefinitionArgs{
				// Frontend service
				"frontend": {
					Name:  pulumi.String("frontend"),
					Image: pulumi.String("ghcr.io/datadog/apps-multiservice-frontend:" + apps.Version),
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
							Value: pulumi.StringPtr("unix:///var/run/datadog/apm.socket"),
						},
						ecs.TaskDefinitionKeyValuePairArgs{
							Name:  pulumi.StringPtr("BACKEND_URL"),
							Value: pulumi.StringPtr("http://backend:8080"),
						},
						ecs.TaskDefinitionKeyValuePairArgs{
							Name:  pulumi.StringPtr("DD_LOGS_INJECTION"),
							Value: pulumi.StringPtr("true"),
						},
					},
					DockerLabels: pulumi.StringMap{
						"com.datadoghq.ad.tags": pulumi.String("[\"ecs_launch_type:ec2\",\"tier:frontend\"]"),
						"com.datadoghq.ad.logs": pulumi.String("[{\"source\":\"frontend\",\"service\":\"frontend\"}]"),
					},
					Cpu:    pulumi.IntPtr(100),
					Memory: pulumi.IntPtr(128),
					PortMappings: ecs.TaskDefinitionPortMappingArray{
						ecs.TaskDefinitionPortMappingArgs{
							ContainerPort: pulumi.IntPtr(8080),
							HostPort:      pulumi.IntPtr(8080),
							Protocol:      pulumi.StringPtr("tcp"),
						},
					},
					Links: pulumi.ToStringArray([]string{"backend:backend"}),
					MountPoints: ecs.TaskDefinitionMountPointArray{
						ecs.TaskDefinitionMountPointArgs{
							SourceVolume:  pulumi.StringPtr("apmsocketpath"),
							ContainerPath: pulumi.StringPtr("/var/run/datadog"),
							ReadOnly:      pulumi.BoolPtr(true),
						},
					},
				},
				// Backend service
				"backend": {
					Name:  pulumi.String("backend"),
					Image: pulumi.String("ghcr.io/datadog/apps-multiservice-backend:" + apps.Version),
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
							Value: pulumi.StringPtr("unix:///var/run/datadog/apm.socket"),
						},
						ecs.TaskDefinitionKeyValuePairArgs{
							Name:  pulumi.StringPtr("DATABASE_URL"),
							Value: pulumi.StringPtr("http://database:8080"),
						},
						ecs.TaskDefinitionKeyValuePairArgs{
							Name:  pulumi.StringPtr("DD_LOGS_INJECTION"),
							Value: pulumi.StringPtr("true"),
						},
					},
					DockerLabels: pulumi.StringMap{
						"com.datadoghq.ad.tags": pulumi.String("[\"ecs_launch_type:ec2\",\"tier:backend\"]"),
						"com.datadoghq.ad.logs": pulumi.String("[{\"source\":\"backend\",\"service\":\"backend\"}]"),
					},
					Cpu:    pulumi.IntPtr(100),
					Memory: pulumi.IntPtr(128),
					PortMappings: ecs.TaskDefinitionPortMappingArray{
						ecs.TaskDefinitionPortMappingArgs{
							ContainerPort: pulumi.IntPtr(8080),
							Protocol:      pulumi.StringPtr("tcp"),
						},
					},
					Links: pulumi.ToStringArray([]string{"database:database"}),
					MountPoints: ecs.TaskDefinitionMountPointArray{
						ecs.TaskDefinitionMountPointArgs{
							SourceVolume:  pulumi.StringPtr("apmsocketpath"),
							ContainerPath: pulumi.StringPtr("/var/run/datadog"),
							ReadOnly:      pulumi.BoolPtr(true),
						},
					},
				},
				// Database service
				"database": {
					Name:  pulumi.String("database"),
					Image: pulumi.String("ghcr.io/datadog/apps-multiservice-database:" + apps.Version),
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
							Value: pulumi.StringPtr("unix:///var/run/datadog/apm.socket"),
						},
						ecs.TaskDefinitionKeyValuePairArgs{
							Name:  pulumi.StringPtr("DD_LOGS_INJECTION"),
							Value: pulumi.StringPtr("true"),
						},
					},
					DockerLabels: pulumi.StringMap{
						"com.datadoghq.ad.tags": pulumi.String("[\"ecs_launch_type:ec2\",\"tier:database\"]"),
						"com.datadoghq.ad.logs": pulumi.String("[{\"source\":\"database\",\"service\":\"database\"}]"),
					},
					Cpu:    pulumi.IntPtr(100),
					Memory: pulumi.IntPtr(128),
					PortMappings: ecs.TaskDefinitionPortMappingArray{
						ecs.TaskDefinitionPortMappingArgs{
							ContainerPort: pulumi.IntPtr(8080),
							Protocol:      pulumi.StringPtr("tcp"),
						},
					},
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
			NetworkMode: pulumi.StringPtr("bridge"),
			Family:      e.CommonNamer().DisplayName(255, pulumi.ToStringArray([]string{"ecs-multiservice", "ec2"})...),
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

	return ecsComponent, nil
}
