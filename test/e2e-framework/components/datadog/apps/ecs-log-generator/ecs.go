// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

// Package ecsloggenerator provides a log generator test application for ECS E2E testing.
//
// This package is owned by the ecs-experiences team and provides test infrastructure
// for validating log collection functionality in ECS environments.
//
// Purpose:
//   - Test log collection from container stdout/stderr
//   - Validate multiline log handling (stack traces)
//   - Test log parsing (JSON, structured logs)
//   - Verify log filtering and sampling
//   - Test log-trace correlation
//   - Validate log status remapping and source detection
//
// Do NOT use this for:
//   - Production workloads
//   - Log management product feature testing (use Logs-owned test apps)
//   - Performance benchmarking
//
// See README.md for detailed documentation.
package ecsloggenerator

import (
	"github.com/DataDog/datadog-agent/test/e2e-framework/common/config"
	"github.com/DataDog/datadog-agent/test/e2e-framework/components/datadog/apps"
	ecsComp "github.com/DataDog/datadog-agent/test/e2e-framework/components/ecs"
	"github.com/DataDog/datadog-agent/test/e2e-framework/resources/aws"

	"github.com/pulumi/pulumi-awsx/sdk/v2/go/awsx/awsx"
	"github.com/pulumi/pulumi-awsx/sdk/v2/go/awsx/ecs"
	"github.com/pulumi/pulumi/sdk/v3/go/pulumi"
)

// EcsAppDefinition creates a log generator test application for testing log collection in ECS.
//
// The application emits various log types to validate log pipeline functionality:
//   - Structured JSON logs
//   - Multiline stack traces
//   - Different log levels (DEBUG, INFO, WARN, ERROR)
//   - High-volume logs for sampling tests
//   - Logs with trace correlation context
//
// This is the EC2 deployment variant using bridge networking.
//
// Owned by: ecs-experiences team
// Purpose: ECS E2E test infrastructure
func EcsAppDefinition(e aws.Environment, clusterArn pulumi.StringInput, opts ...pulumi.ResourceOption) (*ecsComp.Workload, error) {
	namer := e.Namer.WithPrefix("ecs-log-generator").WithPrefix("ec2")
	opts = append(opts, e.WithProviders(config.ProviderAWS, config.ProviderAWSX))

	ecsComponent := &ecsComp.Workload{}
	if err := e.Ctx().RegisterComponentResource("dd:apps", namer.ResourceName("grp"), ecsComponent, opts...); err != nil {
		return nil, err
	}

	opts = append(opts, pulumi.Parent(ecsComponent))

	// Create the log generator application
	if _, err := ecs.NewEC2Service(e.Ctx(), namer.ResourceName("server"), &ecs.EC2ServiceArgs{
		Name:                 e.CommonNamer().DisplayName(255, pulumi.String("ecs-log-generator"), pulumi.String("ec2")),
		Cluster:              clusterArn,
		DesiredCount:         pulumi.IntPtr(1),
		EnableExecuteCommand: pulumi.BoolPtr(true),
		TaskDefinitionArgs: &ecs.EC2ServiceTaskDefinitionArgs{
			Containers: map[string]ecs.TaskDefinitionContainerDefinitionArgs{
				// Log generator container
				"log-generator": {
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
						"com.datadoghq.ad.tags":             pulumi.String("[\"ecs_launch_type:ec2\",\"app:log-generator\"]"),
						"com.datadoghq.ad.logs":             pulumi.String("[{\"source\":\"log-generator\",\"service\":\"log-generator\"}]"),
						"com.datadoghq.tags.service":        pulumi.String("log-generator"),
						"com.datadoghq.tags.env":            pulumi.String("test"),
						"com.datadoghq.tags.version":        pulumi.String("1.0"),
						"com.datadoghq.ad.log_processing_rules": pulumi.String("[{\"type\":\"multi_line\",\"name\":\"stack_trace\",\"pattern\":\"^[\\\\s]+at\"}]"),
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
				},
			},
			ExecutionRole: &awsx.DefaultRoleWithPolicyArgs{
				RoleArn: pulumi.StringPtr(e.ECSTaskExecutionRole()),
			},
			TaskRole: &awsx.DefaultRoleWithPolicyArgs{
				RoleArn: pulumi.StringPtr(e.ECSTaskRole()),
			},
			NetworkMode: pulumi.StringPtr("bridge"),
			Family:      e.CommonNamer().DisplayName(255, pulumi.ToStringArray([]string{"ecs-log-generator", "ec2"})...),
		},
	}, opts...); err != nil {
		return nil, err
	}

	return ecsComponent, nil
}
