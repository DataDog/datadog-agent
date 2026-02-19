// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

package agent

import (
	"github.com/DataDog/datadog-agent/test/e2e-framework/common/config"
	"github.com/DataDog/datadog-agent/test/e2e-framework/common/utils"
	"github.com/DataDog/datadog-agent/test/e2e-framework/components/datadog/ecsagentparams"
	"github.com/DataDog/datadog-agent/test/e2e-framework/components/datadog/fakeintake"
	"github.com/DataDog/datadog-agent/test/e2e-framework/resources/aws"

	classicECS "github.com/pulumi/pulumi-aws/sdk/v6/go/aws/ecs"
	"github.com/pulumi/pulumi-awsx/sdk/v2/go/awsx/awsx"
	"github.com/pulumi/pulumi-awsx/sdk/v2/go/awsx/ecs"
	"github.com/pulumi/pulumi/sdk/v3/go/pulumi"
)

func ECSLinuxDaemonDefinition(e aws.Environment, name string, apiKeySSMParamName pulumi.StringInput, fakeintake *fakeintake.Fakeintake, clusterArn pulumi.StringInput, options ...ecsagentparams.Option) (*ecs.EC2Service, error) {
	params, err := ecsagentparams.NewParams(&e, options...)
	if err != nil {
		return nil, err
	}

	return ecs.NewEC2Service(e.Ctx(), e.Namer.ResourceName(name), &ecs.EC2ServiceArgs{
		Name:               e.CommonNamer().DisplayName(255, pulumi.String(name)),
		Cluster:            clusterArn,
		SchedulingStrategy: pulumi.StringPtr("DAEMON"),
		PlacementConstraints: classicECS.ServicePlacementConstraintArray{
			classicECS.ServicePlacementConstraintArgs{
				Type:       pulumi.String("memberOf"),
				Expression: pulumi.StringPtr("attribute:ecs.os-type == linux"),
			},
		},
		EnableExecuteCommand: pulumi.BoolPtr(true),
		TaskDefinitionArgs: &ecs.EC2ServiceTaskDefinitionArgs{
			Containers: map[string]ecs.TaskDefinitionContainerDefinitionArgs{
				"datadog-agent": ecsLinuxAgentSingleContainerDefinition(&e, apiKeySSMParamName, fakeintake, params),
			},
			ExecutionRole: &awsx.DefaultRoleWithPolicyArgs{
				RoleArn: pulumi.StringPtr(e.ECSTaskExecutionRole()),
			},
			TaskRole: &awsx.DefaultRoleWithPolicyArgs{
				RoleArn: pulumi.StringPtr(e.ECSTaskRole()),
			},
			NetworkMode: pulumi.StringPtr(params.NetworkMode),
			PidMode:     pulumi.StringPtr("host"),
			Family:      e.CommonNamer().DisplayName(255, pulumi.String("datadog-agent-ec2")),
			Volumes: classicECS.TaskDefinitionVolumeArray{
				classicECS.TaskDefinitionVolumeArgs{
					HostPath: pulumi.StringPtr("/var/run/docker.sock"),
					Name:     pulumi.String("docker_sock"),
				},
				classicECS.TaskDefinitionVolumeArgs{
					HostPath: pulumi.StringPtr("/proc"),
					Name:     pulumi.String("proc"),
				},
				classicECS.TaskDefinitionVolumeArgs{
					HostPath: pulumi.StringPtr("/sys/fs/cgroup"),
					Name:     pulumi.String("cgroup"),
				},
				classicECS.TaskDefinitionVolumeArgs{
					HostPath: pulumi.StringPtr("/opt/datadog-agent/run"),
					Name:     pulumi.String("dd-logpointdir"),
				},
				classicECS.TaskDefinitionVolumeArgs{
					HostPath: pulumi.StringPtr("/var/run/datadog"),
					Name:     pulumi.String("dd-sockets"),
				},
				classicECS.TaskDefinitionVolumeArgs{
					HostPath: pulumi.StringPtr("/sys/kernel/debug"),
					Name:     pulumi.String("debug"),
				},
			},
		},
	}, e.WithProviders(config.ProviderAWS, config.ProviderAWSX))
}

func ecsLinuxAgentSingleContainerDefinition(e config.Env, apiKeySSMParamName pulumi.StringInput, fakeintake *fakeintake.Fakeintake, params *ecsagentparams.Params) ecs.TaskDefinitionContainerDefinitionArgs {
	return ecs.TaskDefinitionContainerDefinitionArgs{
		Cpu:       pulumi.IntPtr(200),
		Memory:    pulumi.IntPtr(512),
		Name:      pulumi.String("datadog-agent"),
		Image:     pulumi.String(dockerAgentFullImagePath(e, "public.ecr.aws/datadog/agent", "", false, false, false, params.WindowsImage)),
		Essential: pulumi.BoolPtr(true),
		LinuxParameters: ecs.TaskDefinitionLinuxParametersArgs{
			Capabilities: ecs.TaskDefinitionKernelCapabilitiesArgs{
				Add: pulumi.ToStringArray([]string{"SYS_ADMIN", "SYS_RESOURCE", "SYS_PTRACE", "NET_ADMIN", "NET_BROADCAST", "NET_RAW", "IPC_LOCK", "CHOWN"}),
			},
		},
		Environment: append(append(append(ecs.TaskDefinitionKeyValuePairArray{
			ecs.TaskDefinitionKeyValuePairArgs{
				Name:  pulumi.StringPtr("DD_APM_ENABLED"),
				Value: pulumi.StringPtr("true"),
			},
			ecs.TaskDefinitionKeyValuePairArgs{
				Name:  pulumi.StringPtr("DD_APM_NON_LOCAL_TRAFFIC"),
				Value: pulumi.StringPtr("true"),
			},
			ecs.TaskDefinitionKeyValuePairArgs{
				Name:  pulumi.StringPtr("DD_CHECKS_TAG_CARDINALITY"),
				Value: pulumi.StringPtr("high"),
			},
			ecs.TaskDefinitionKeyValuePairArgs{
				Name:  pulumi.StringPtr("DD_DOGSTATSD_TAG_CARDINALITY"),
				Value: pulumi.StringPtr("high"),
			},
			ecs.TaskDefinitionKeyValuePairArgs{
				Name:  pulumi.StringPtr("DD_DOGSTATSD_ORIGIN_DETECTION"),
				Value: pulumi.StringPtr("true"),
			},
			ecs.TaskDefinitionKeyValuePairArgs{
				Name:  pulumi.StringPtr("DD_DOGSTATSD_ORIGIN_DETECTION_CLIENT"),
				Value: pulumi.StringPtr("true"),
			},

			ecs.TaskDefinitionKeyValuePairArgs{
				Name:  pulumi.StringPtr("DD_DOGSTATSD_SOCKET"),
				Value: pulumi.StringPtr("/var/run/datadog/dsd.socket"),
			},
			ecs.TaskDefinitionKeyValuePairArgs{
				Name:  pulumi.StringPtr("DD_LOGS_ENABLED"),
				Value: pulumi.StringPtr("true"),
			},
			ecs.TaskDefinitionKeyValuePairArgs{
				Name:  pulumi.StringPtr("DD_LOGS_CONFIG_CONTAINER_COLLECT_ALL"),
				Value: pulumi.StringPtr("true"),
			},
			ecs.TaskDefinitionKeyValuePairArgs{
				Name:  pulumi.StringPtr("DD_ECS_COLLECT_RESOURCE_TAGS_EC2"),
				Value: pulumi.StringPtr("true"),
			},
			ecs.TaskDefinitionKeyValuePairArgs{
				Name:  pulumi.StringPtr("DD_DOGSTATSD_NON_LOCAL_TRAFFIC"),
				Value: pulumi.StringPtr("true"),
			},
			ecs.TaskDefinitionKeyValuePairArgs{
				// DD_PROCESS_CONFIG_PROCESS_COLLECTION_ENABLED is compatible with Agent 7.35+
				Name:  pulumi.StringPtr("DD_PROCESS_CONFIG_PROCESS_COLLECTION_ENABLED"),
				Value: pulumi.StringPtr("true"),
			},
			ecs.TaskDefinitionKeyValuePairArgs{
				Name:  pulumi.StringPtr("DD_TELEMETRY_ENABLED"),
				Value: pulumi.StringPtr("true"),
			},
			ecs.TaskDefinitionKeyValuePairArgs{
				Name:  pulumi.StringPtr("DD_TELEMETRY_CHECKS"),
				Value: pulumi.StringPtr("*"),
			},
		}, ecsAgentAdditionalEndpointsEnv(params)...), ecsFakeintakeAdditionalEndpointsEnv(fakeintake)...), ecsAgentAdditionalEnvFromConfig(e)...),
		Secrets: ecs.TaskDefinitionSecretArray{
			ecs.TaskDefinitionSecretArgs{
				Name:      pulumi.String("DD_API_KEY"),
				ValueFrom: apiKeySSMParamName,
			},
		},
		MountPoints: ecs.TaskDefinitionMountPointArray{
			ecs.TaskDefinitionMountPointArgs{
				ContainerPath: pulumi.StringPtr("/var/run/docker.sock"),
				SourceVolume:  pulumi.StringPtr("docker_sock"),
				ReadOnly:      pulumi.BoolPtr(true),
			},
			ecs.TaskDefinitionMountPointArgs{
				ContainerPath: pulumi.StringPtr("/host/proc"),
				SourceVolume:  pulumi.StringPtr("proc"),
				ReadOnly:      pulumi.BoolPtr(true),
			},
			ecs.TaskDefinitionMountPointArgs{
				ContainerPath: pulumi.StringPtr("/host/sys/fs/cgroup"),
				SourceVolume:  pulumi.StringPtr("cgroup"),
				ReadOnly:      pulumi.BoolPtr(true),
			},
			ecs.TaskDefinitionMountPointArgs{
				ContainerPath: pulumi.StringPtr("/opt/datadog-agent/run"),
				SourceVolume:  pulumi.StringPtr("dd-logpointdir"),
				ReadOnly:      pulumi.BoolPtr(false),
			},
			ecs.TaskDefinitionMountPointArgs{
				ContainerPath: pulumi.StringPtr("/var/run/datadog"),
				SourceVolume:  pulumi.StringPtr("dd-sockets"),
				ReadOnly:      pulumi.BoolPtr(false),
			},
			ecs.TaskDefinitionMountPointArgs{
				ContainerPath: pulumi.StringPtr("/sys/kernel/debug"),
				SourceVolume:  pulumi.StringPtr("debug"),
				ReadOnly:      pulumi.BoolPtr(false),
			},
		},
		HealthCheck: &ecs.TaskDefinitionHealthCheckArgs{
			Retries:     pulumi.IntPtr(2),
			Command:     pulumi.ToStringArray([]string{"CMD-SHELL", "agent health"}),
			StartPeriod: pulumi.IntPtr(10),
			Interval:    pulumi.IntPtr(30),
			Timeout:     pulumi.IntPtr(5),
		},
		PortMappings: ecs.TaskDefinitionPortMappingArray{
			ecs.TaskDefinitionPortMappingArgs{
				ContainerPort: pulumi.Int(8125),
				HostPort:      pulumi.IntPtr(8125),
				Protocol:      pulumi.StringPtr("udp"),
			},
			ecs.TaskDefinitionPortMappingArgs{
				ContainerPort: pulumi.Int(8126),
				HostPort:      pulumi.IntPtr(8126),
				Protocol:      pulumi.StringPtr("tcp"),
			},
		},
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
}

func ecsAgentAdditionalEndpointsEnv(params *ecsagentparams.Params) []ecs.TaskDefinitionKeyValuePairInput {
	if params == nil {
		return []ecs.TaskDefinitionKeyValuePairInput{}
	}

	taskDefArray := []ecs.TaskDefinitionKeyValuePairInput{}
	for k, v := range params.AgentServiceEnvironment {
		taskDefArray = append(taskDefArray, ecs.TaskDefinitionKeyValuePairArgs{Name: pulumi.StringPtr(k), Value: pulumi.StringPtr(v)})
	}
	return taskDefArray
}

func ecsFakeintakeAdditionalEndpointsEnv(fakeintake *fakeintake.Fakeintake) []ecs.TaskDefinitionKeyValuePairInput {
	if fakeintake == nil {
		return []ecs.TaskDefinitionKeyValuePairInput{}
	}
	return []ecs.TaskDefinitionKeyValuePairInput{
		ecs.TaskDefinitionKeyValuePairArgs{
			Name:  pulumi.StringPtr("DD_SKIP_SSL_VALIDATION"),
			Value: pulumi.StringPtr("true"),
		},
		ecs.TaskDefinitionKeyValuePairArgs{
			Name:  pulumi.StringPtr("DD_REMOTE_CONFIGURATION_NO_TLS_VALIDATION"),
			Value: pulumi.StringPtr("true"),
		},
		ecs.TaskDefinitionKeyValuePairArgs{
			Name:  pulumi.StringPtr("DD_PROCESS_CONFIG_PROCESS_DD_URL"),
			Value: fakeintake.URL.ToStringOutput(),
		},
		ecs.TaskDefinitionKeyValuePairArgs{
			Name:  pulumi.StringPtr("DD_ORCHESTRATOR_EXPLORER_ORCHESTRATOR_DD_URL"),
			Value: fakeintake.URL.ToStringOutput(),
		},
		ecs.TaskDefinitionKeyValuePairArgs{
			Name:  pulumi.StringPtr("DD_ADDITIONAL_ENDPOINTS"),
			Value: pulumi.Sprintf(`{"%s": ["FAKEAPIKEY"]}`, fakeintake.URL.ToStringOutput()),
		},
		ecs.TaskDefinitionKeyValuePairArgs{
			Name:  pulumi.String("DD_LOGS_CONFIG_ADDITIONAL_ENDPOINTS"),
			Value: pulumi.Sprintf(`[{"host": "%s"}]`, fakeintake.Host),
		},
		ecs.TaskDefinitionKeyValuePairArgs{
			Name:  pulumi.StringPtr("DD_LOGS_CONFIG_USE_HTTP"),
			Value: pulumi.StringPtr("true"),
		},
	}
}

func ecsAgentAdditionalEnvFromConfig(e config.Env) []ecs.TaskDefinitionKeyValuePairInput {
	extraEnvVars := e.AgentExtraEnvVars()
	options := make([]ecs.TaskDefinitionKeyValuePairInput, 0, len(extraEnvVars))

	for name, value := range extraEnvVars {
		options = append(options, ecs.TaskDefinitionKeyValuePairArgs{
			Name:  pulumi.StringPtr(name),
			Value: pulumi.StringPtr(value),
		})
	}
	return options
}
