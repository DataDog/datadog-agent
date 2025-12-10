// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

// Package ecschaos provides a chaos testing application for ECS E2E testing.
//
// This package is owned by the ecs-experiences team and provides test infrastructure
// for validating agent resilience and error handling in ECS environments.
//
// Purpose:
//   - Test agent behavior under resource pressure (memory leaks, CPU spikes)
//   - Validate agent recovery from failures (crashes, restarts)
//   - Test handling of high cardinality data
//   - Verify agent behavior during network issues
//   - Validate graceful degradation under stress
//
// Do NOT use this for:
//   - Production workloads
//   - Performance benchmarking
//   - Load testing actual applications
//
// See README.md for detailed documentation.
package ecschaos

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

// EcsAppDefinition creates a chaos testing application for testing agent resilience in ECS.
//
// The application simulates various failure scenarios:
//   - Memory leaks (gradual memory consumption)
//   - CPU spikes (high CPU utilization bursts)
//   - Network timeouts (slow or failing requests)
//   - Application crashes (random process termination)
//   - High cardinality metrics (unique tag combinations)
//
// This is the EC2 deployment variant using bridge networking.
//
// Owned by: ecs-experiences team
// Purpose: ECS E2E test infrastructure
func EcsAppDefinition(e aws.Environment, clusterArn pulumi.StringInput, opts ...pulumi.ResourceOption) (*ecsComp.Workload, error) {
	namer := e.Namer.WithPrefix("ecs-chaos").WithPrefix("ec2")
	opts = append(opts, e.WithProviders(config.ProviderAWS, config.ProviderAWSX))

	ecsComponent := &ecsComp.Workload{}
	if err := e.Ctx().RegisterComponentResource("dd:apps", namer.ResourceName("grp"), ecsComponent, opts...); err != nil {
		return nil, err
	}

	opts = append(opts, pulumi.Parent(ecsComponent))

	// Create the chaos application
	if _, err := ecs.NewEC2Service(e.Ctx(), namer.ResourceName("server"), &ecs.EC2ServiceArgs{
		Name:                 e.CommonNamer().DisplayName(255, pulumi.String("ecs-chaos"), pulumi.String("ec2")),
		Cluster:              clusterArn,
		DesiredCount:         pulumi.IntPtr(1),
		EnableExecuteCommand: pulumi.BoolPtr(true),
		TaskDefinitionArgs: &ecs.EC2ServiceTaskDefinitionArgs{
			Containers: map[string]ecs.TaskDefinitionContainerDefinitionArgs{
				// Chaos container
				"chaos": {
					Name:  pulumi.String("chaos"),
					Image: pulumi.String("ghcr.io/datadog/apps-ecs-chaos:" + apps.Version),
					Environment: ecs.TaskDefinitionKeyValuePairArray{
						// Chaos configuration
						ecs.TaskDefinitionKeyValuePairArgs{
							Name:  pulumi.StringPtr("CHAOS_MODE"),
							Value: pulumi.StringPtr("normal"), // normal, memory_leak, cpu_spike, crash, high_cardinality, network_timeout
						},
						ecs.TaskDefinitionKeyValuePairArgs{
							Name:  pulumi.StringPtr("MEMORY_LEAK_RATE"),
							Value: pulumi.StringPtr("1"), // MB per second
						},
						ecs.TaskDefinitionKeyValuePairArgs{
							Name:  pulumi.StringPtr("CPU_SPIKE_INTERVAL"),
							Value: pulumi.StringPtr("60"), // seconds between spikes
						},
						ecs.TaskDefinitionKeyValuePairArgs{
							Name:  pulumi.StringPtr("CRASH_INTERVAL"),
							Value: pulumi.StringPtr("300"), // seconds between crashes (0 = disabled)
						},
						ecs.TaskDefinitionKeyValuePairArgs{
							Name:  pulumi.StringPtr("HIGH_CARDINALITY_TAGS"),
							Value: pulumi.StringPtr("100"), // number of unique tags
						},
						ecs.TaskDefinitionKeyValuePairArgs{
							Name:  pulumi.StringPtr("METRIC_EMISSION_RATE"),
							Value: pulumi.StringPtr("10"), // metrics per second
						},
						ecs.TaskDefinitionKeyValuePairArgs{
							Name:  pulumi.StringPtr("LARGE_PAYLOAD_SIZE"),
							Value: pulumi.StringPtr("0"), // KB (0 = normal size)
						},
						ecs.TaskDefinitionKeyValuePairArgs{
							Name:  pulumi.StringPtr("NETWORK_TIMEOUT_RATE"),
							Value: pulumi.StringPtr("0"), // percentage of requests that timeout (0-100)
						},
						// Datadog configuration
						ecs.TaskDefinitionKeyValuePairArgs{
							Name:  pulumi.StringPtr("DD_SERVICE"),
							Value: pulumi.StringPtr("chaos"),
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
						"com.datadoghq.ad.tags": pulumi.String("[\"ecs_launch_type:ec2\",\"app:chaos\"]"),
						"com.datadoghq.ad.logs": pulumi.String("[{\"source\":\"chaos\",\"service\":\"chaos\"}]"),
					},
					Cpu:    pulumi.IntPtr(200),
					Memory: pulumi.IntPtr(512),
					PortMappings: ecs.TaskDefinitionPortMappingArray{
						ecs.TaskDefinitionPortMappingArgs{
							ContainerPort: pulumi.IntPtr(8080),
							HostPort:      pulumi.IntPtr(8080),
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
					// Health check with longer grace period for chaos scenarios
					HealthCheck: &ecs.TaskDefinitionHealthCheckArgs{
						Command: pulumi.StringArray{
							pulumi.String("CMD-SHELL"),
							pulumi.String("curl -f http://localhost:8080/health || exit 1"),
						},
						Interval:    pulumi.IntPtr(30),
						Timeout:     pulumi.IntPtr(5),
						Retries:     pulumi.IntPtr(5),
						StartPeriod: pulumi.IntPtr(60),
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
			Family:      e.CommonNamer().DisplayName(255, pulumi.ToStringArray([]string{"ecs-chaos", "ec2"})...),
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
