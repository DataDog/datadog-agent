// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

package agent

import (
	"github.com/pulumi/pulumi-awsx/sdk/v2/go/awsx/ecs"
	"github.com/pulumi/pulumi/sdk/v3/go/pulumi"

	"github.com/DataDog/datadog-agent/test/e2e-framework/common/config"
	"github.com/DataDog/datadog-agent/test/e2e-framework/common/utils"
	"github.com/DataDog/datadog-agent/test/e2e-framework/components/datadog/fakeintake"
)

func ECSFargateLinuxContainerDefinition(e config.Env, image string, apiKeySSMParamName pulumi.StringInput, fakeintake *fakeintake.Fakeintake, logConfig ecs.TaskDefinitionLogConfigurationPtrInput) (*ecs.TaskDefinitionContainerDefinitionArgs, *ecs.TaskDefinitionContainerDefinitionArgs) {
	if image == "" {
		image = dockerAgentFullImagePath(e, "public.ecr.aws/datadog/agent", "", false, false, false, false)
	}

	// Init container copies files to a writeable volume
	initContainer := &ecs.TaskDefinitionContainerDefinitionArgs{
		Cpu:                    pulumi.IntPtr(0),
		Name:                   pulumi.String("init-copy-agent-config"),
		Image:                  pulumi.String(image),
		Essential:              pulumi.BoolPtr(false),
		ReadonlyRootFilesystem: pulumi.BoolPtr(true),
		Command:                pulumi.StringArray{pulumi.String("sh"), pulumi.String("-c"), pulumi.String("cp -R /etc/datadog-agent/* /agent-config/")},
		MountPoints: ecs.TaskDefinitionMountPointArray{
			ecs.TaskDefinitionMountPointArgs{

				ContainerPath: pulumi.StringPtr("/agent-config"),
				SourceVolume:  pulumi.StringPtr("agent-config"),
			},
		},
	}

	agentContainer := &ecs.TaskDefinitionContainerDefinitionArgs{
		Cpu:                    pulumi.IntPtr(0),
		Name:                   pulumi.String("datadog-agent"),
		Image:                  pulumi.String(image),
		Essential:              pulumi.BoolPtr(true),
		ReadonlyRootFilesystem: pulumi.BoolPtr(true),
		DependsOn: ecs.TaskDefinitionContainerDependencyArray{
			ecs.TaskDefinitionContainerDependencyArgs{
				ContainerName: pulumi.String("init-copy-agent-config"),
				Condition:     pulumi.String("SUCCESS"),
			},
		},
		Environment: append(append(ecs.TaskDefinitionKeyValuePairArray{
			ecs.TaskDefinitionKeyValuePairArgs{
				Name:  pulumi.StringPtr("DD_DOGSTATSD_SOCKET"),
				Value: pulumi.StringPtr("/var/run/datadog/dsd.socket"),
			},
			ecs.TaskDefinitionKeyValuePairArgs{
				Name:  pulumi.StringPtr("ECS_FARGATE"),
				Value: pulumi.StringPtr("true"),
			},
			ecs.TaskDefinitionKeyValuePairArgs{
				Name:  pulumi.StringPtr("DD_CHECKS_TAG_CARDINALITY"),
				Value: pulumi.StringPtr("high"),
			},
			ecs.TaskDefinitionKeyValuePairArgs{
				Name:  pulumi.StringPtr("DD_TELEMETRY_ENABLED"),
				Value: pulumi.StringPtr("true"),
			},
			ecs.TaskDefinitionKeyValuePairArgs{
				Name:  pulumi.StringPtr("DD_TELEMETRY_CHECKS"),
				Value: pulumi.StringPtr("*"),
			},
			ecs.TaskDefinitionKeyValuePairArgs{
				Name:  pulumi.StringPtr("DD_ECS_TASK_COLLECTION_ENABLED"),
				Value: pulumi.StringPtr("true"),
			},
			ecs.TaskDefinitionKeyValuePairArgs{
				Name:  pulumi.StringPtr("DD_PROCESS_CONFIG_PROCESS_COLLECTION_ENABLED"),
				Value: pulumi.StringPtr("true"),
			},
		}, ecsFakeintakeAdditionalEndpointsEnv(fakeintake)...), ecsAgentAdditionalEnvFromConfig(e)...),
		Secrets: ecs.TaskDefinitionSecretArray{
			ecs.TaskDefinitionSecretArgs{
				Name:      pulumi.String("DD_API_KEY"),
				ValueFrom: apiKeySSMParamName,
			},
		},
		MountPoints: ecs.TaskDefinitionMountPointArray{
			ecs.TaskDefinitionMountPointArgs{
				ContainerPath: pulumi.StringPtr("/var/run/datadog"),
				SourceVolume:  pulumi.StringPtr("dd-sockets"),
			},
			ecs.TaskDefinitionMountPointArgs{
				ContainerPath: pulumi.StringPtr("/etc/datadog-agent"),
				SourceVolume:  pulumi.StringPtr("agent-config"),
			},
			ecs.TaskDefinitionMountPointArgs{
				ContainerPath: pulumi.StringPtr("/opt/datadog-agent/run"),
				SourceVolume:  pulumi.StringPtr("agent-option"),
			},
			ecs.TaskDefinitionMountPointArgs{
				ContainerPath: pulumi.StringPtr("/tmp"),
				SourceVolume:  pulumi.StringPtr("agent-tmp"),
			},
			ecs.TaskDefinitionMountPointArgs{
				ContainerPath: pulumi.StringPtr("/var/log/datadog"),
				SourceVolume:  pulumi.StringPtr("agent-log"),
			},
		},
		HealthCheck: &ecs.TaskDefinitionHealthCheckArgs{
			Retries:     pulumi.IntPtr(2),
			Command:     pulumi.ToStringArray([]string{"CMD-SHELL", "/probe.sh"}),
			StartPeriod: pulumi.IntPtr(10),
			Interval:    pulumi.IntPtr(30),
			Timeout:     pulumi.IntPtr(5),
		},
		LogConfiguration: logConfig,
		PortMappings:     ecs.TaskDefinitionPortMappingArray{},
		VolumesFrom:      ecs.TaskDefinitionVolumeFromArray{},
		DockerLabels: pulumi.StringMap{
			"com.datadoghq.ad.checks": pulumi.String(utils.JSONMustMarshal(
				map[string]interface{}{
					"openmetrics": map[string]interface{}{
						"init_config": map[string]interface{}{},
						"instances": []map[string]interface{}{
							{
								"openmetrics_endpoint": "http://localhost:5000/telemetry",
								"namespace":            "datadog.agent",
								"metrics": []string{
									".*",
								},
							},
						},
					},
				},
			)),
		},
	}

	return initContainer, agentContainer
}

// ECSFargateWindowsContainerDefinition returns the container definition for the Windows agent running on ECS Fargate.
// Firelens is not supported. Logs could be collected if sent to cloudwatch using the `awslogs` driver. See:
// https://docs.aws.amazon.com/AmazonECS/latest/developerguide/tutorial-deploy-fluentbit-on-windows.html
func ECSFargateWindowsContainerDefinition(e config.Env, image string, apiKeySSMParamName pulumi.StringInput, fakeintake *fakeintake.Fakeintake) *ecs.TaskDefinitionContainerDefinitionArgs {
	if image == "" {
		image = dockerAgentFullImagePath(e, "public.ecr.aws/datadog/agent", "", false, false, false, true)
	}
	return &ecs.TaskDefinitionContainerDefinitionArgs{
		Cpu:       pulumi.IntPtr(0),
		Name:      pulumi.String("datadog-agent"),
		Image:     pulumi.String(image),
		Essential: pulumi.BoolPtr(true),
		Environment: append(ecs.TaskDefinitionKeyValuePairArray{
			ecs.TaskDefinitionKeyValuePairArgs{
				Name:  pulumi.StringPtr("ECS_FARGATE"),
				Value: pulumi.StringPtr("true"),
			},
			ecs.TaskDefinitionKeyValuePairArgs{
				Name:  pulumi.StringPtr("DD_CHECKS_TAG_CARDINALITY"),
				Value: pulumi.StringPtr("high"),
			},
		}, ecsFakeintakeAdditionalEndpointsEnv(fakeintake)...),
		Secrets: ecs.TaskDefinitionSecretArray{
			ecs.TaskDefinitionSecretArgs{
				Name:      pulumi.String("DD_API_KEY"),
				ValueFrom: apiKeySSMParamName,
			},
		},
		HealthCheck: &ecs.TaskDefinitionHealthCheckArgs{
			Retries:     pulumi.IntPtr(2),
			Command:     pulumi.ToStringArray([]string{"CMD-SHELL", "agent health"}),
			StartPeriod: pulumi.IntPtr(10),
			Interval:    pulumi.IntPtr(30),
			Timeout:     pulumi.IntPtr(5),
		},
		// Firelens is not compatible with windows containers
		PortMappings:     ecs.TaskDefinitionPortMappingArray{},
		VolumesFrom:      ecs.TaskDefinitionVolumeFromArray{},
		WorkingDirectory: pulumi.String(`C:\`),
	}
}
