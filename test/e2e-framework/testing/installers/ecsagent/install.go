// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package ecsagent installs the Datadog Agent on an ECS cluster as a Daemon
// service via the AWS SDK, without relying on Pulumi.
//
// The installer:
//  1. Writes the API key to AWS SSM Parameter Store
//  2. Registers a task definition with the agent container (mirrors the
//     Pulumi component in components/datadog/agent/ecs.go)
//  3. Creates or updates an ECS service with DAEMON scheduling
//
// The ECS cluster and FakeIntake are still provisioned by Pulumi; only the
// agent service/task-definition step moves outside Pulumi.
package ecsagent

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"testing"

	"github.com/aws/aws-sdk-go-v2/aws"
	awsConfig "github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/ecs"
	ecstypes "github.com/aws/aws-sdk-go-v2/service/ecs/types"
	"github.com/aws/aws-sdk-go-v2/service/ssm"
	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/test/e2e-framework/components/datadog/ecsagentparams"
	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/environments"
	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/runner"
	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/runner/parameters"
)

const (
	agentImage     = "public.ecr.aws/datadog/agent:latest"
	ssmParamPrefix = "/e2e/datadog/agent-apikey"
	daemonService  = "ec2-linux-dd-agent"
	taskFamily     = "datadog-agent-ec2"
)

// Install installs the Datadog Agent on an ECS cluster as a Daemon service.
// It creates/updates the SSM API key parameter, the task definition, and the
// ECS service. The ECS cluster and FakeIntake must already be provisioned
// (env.ECSCluster and env.FakeIntake are populated by Pulumi).
//
// Usage in SetupSuite (or PostProvision):
//
//	ecsagent.Install(s.T(), s.Env(), ecsagentparams.WithNetworkMode("bridge"))
func Install(t *testing.T, env *environments.ECS, opts ...ecsagentparams.Option) {
	t.Helper()
	require.NotNil(t, env.ECSCluster, "ecsagent.Install: ECSCluster is nil, Pulumi must provision it first")

	params, err := ecsagentparams.NewParams(opts...)
	require.NoError(t, err, "ecsagent.Install: failed to parse options")

	apiKey, err := runner.GetProfile().SecretStore().Get(parameters.APIKey)
	require.NoError(t, err, "ecsagent.Install: failed to get API key")
	apiKey = strings.TrimSpace(apiKey)

	// Use the cluster ARN to derive the region.
	clusterArn := env.ECSCluster.ClusterArn
	region := regionFromArn(clusterArn)
	if region == "" {
		region = "us-east-1" // fallback; real clusters always have ARNs
	}

	cfg, err := awsConfig.LoadDefaultConfig(context.Background(),
		awsConfig.WithRegion(region),
	)
	require.NoError(t, err, "ecsagent.Install: failed to load AWS config")

	// 1. Write API key to SSM.
	ssmClient := ssm.NewFromConfig(cfg)
	paramName := fmt.Sprintf("%s/%s", ssmParamPrefix, env.ECSCluster.ClusterName)
	_, err = ssmClient.PutParameter(context.Background(), &ssm.PutParameterInput{
		Name:      aws.String(paramName),
		Value:     aws.String(apiKey),
		Type:      "SecureString",
		Overwrite: aws.Bool(true),
	})
	require.NoError(t, err, "ecsagent.Install: failed to write API key to SSM")

	// 2. Build container definition.
	envVars := buildEnvVars(params, env)
	containerDef := buildContainerDef(envVars, paramName)

	containerJSON, err := json.Marshal([]interface{}{containerDef})
	require.NoError(t, err, "ecsagent.Install: failed to marshal container definitions")

	// 3. Register task definition.
	ecsClient := ecs.NewFromConfig(cfg)
	family := fmt.Sprintf("%s-%s", taskFamily, env.ECSCluster.ClusterName)
	tdResp, err := ecsClient.RegisterTaskDefinition(context.Background(), &ecs.RegisterTaskDefinitionInput{
		Family:                  aws.String(family),
		NetworkMode:             ecstypes.NetworkMode(params.NetworkMode),
		PidMode:                 ecstypes.PidModeHost,
		RequiresCompatibilities: []ecstypes.Compatibility{ecstypes.CompatibilityEc2},
		ContainerDefinitions:    containerDefsFromJSON(containerJSON),
		ExecutionRoleArn:        executionRoleFromEnv(),
		TaskRoleArn:             taskRoleFromEnv(),
		Volumes:                 agentVolumes(),
	})
	require.NoError(t, err, "ecsagent.Install: failed to register task definition")
	tdArn := aws.ToString(tdResp.TaskDefinition.TaskDefinitionArn)

	// 4. Create or update the ECS daemon service.
	svcName := fmt.Sprintf("%s-%s", daemonService, env.ECSCluster.ClusterName)
	_, createErr := ecsClient.CreateService(context.Background(), &ecs.CreateServiceInput{
		ServiceName:          aws.String(svcName),
		Cluster:              aws.String(clusterArn),
		TaskDefinition:       aws.String(tdArn),
		SchedulingStrategy:   ecstypes.SchedulingStrategyDaemon,
		EnableExecuteCommand: true,
		PlacementConstraints: []ecstypes.PlacementConstraint{
			{
				Type:       ecstypes.PlacementConstraintTypeDistinctInstance,
				Expression: aws.String("attribute:ecs.os-type == linux"),
			},
		},
	})
	if createErr != nil && strings.Contains(createErr.Error(), "already exists") {
		// Service already exists — update the task definition.
		_, err = ecsClient.UpdateService(context.Background(), &ecs.UpdateServiceInput{
			Service:        aws.String(svcName),
			Cluster:        aws.String(clusterArn),
			TaskDefinition: aws.String(tdArn),
		})
		require.NoError(t, err, "ecsagent.Install: failed to update ECS service")
	} else {
		require.NoError(t, createErr, "ecsagent.Install: failed to create ECS service")
	}

	t.Logf("ecsagent.Install: service %s deployed on cluster %s", svcName, env.ECSCluster.ClusterName)
}

// regionFromArn extracts the AWS region from an ARN (arn:aws:ecs:<region>:...).
func regionFromArn(arn string) string {
	parts := strings.Split(arn, ":")
	if len(parts) >= 4 {
		return parts[3]
	}
	return ""
}

// buildEnvVars assembles the container environment variables, mirroring
// components/datadog/agent/ecs.go:ecsLinuxAgentSingleContainerDefinition.
func buildEnvVars(params *ecsagentparams.Params, env *environments.ECS) []ecstypes.KeyValuePair {
	base := []ecstypes.KeyValuePair{
		kv("DD_APM_ENABLED", "true"),
		kv("DD_APM_NON_LOCAL_TRAFFIC", "true"),
		kv("DD_CHECKS_TAG_CARDINALITY", "high"),
		kv("DD_DOGSTATSD_TAG_CARDINALITY", "high"),
		kv("DD_DOGSTATSD_ORIGIN_DETECTION", "true"),
		kv("DD_DOGSTATSD_ORIGIN_DETECTION_CLIENT", "true"),
		kv("DD_DOGSTATSD_SOCKET", "/var/run/datadog/dsd.socket"),
		kv("DD_LOGS_ENABLED", "true"),
		kv("DD_LOGS_CONFIG_CONTAINER_COLLECT_ALL", "true"),
		kv("DD_ECS_COLLECT_RESOURCE_TAGS_EC2", "true"),
		kv("DD_DOGSTATSD_NON_LOCAL_TRAFFIC", "true"),
		kv("DD_PROCESS_CONFIG_PROCESS_COLLECTION_ENABLED", "true"),
		kv("DD_TELEMETRY_ENABLED", "true"),
		kv("DD_TELEMETRY_CHECKS", "*"),
	}

	// Extra env vars from options.
	for k, v := range params.AgentServiceEnvironment {
		k, v := k, v
		base = append(base, kv(k, v))
	}

	// FakeIntake endpoints (mirrors ecsFakeintakeAdditionalEndpointsEnv).
	if env.FakeIntake != nil && env.FakeIntake.URL != "" {
		url := env.FakeIntake.URL
		skipSSL := env.FakeIntake.Scheme != "http" // SSL validation skip when http (no cert)
		logsNoSSL := "false"
		if env.FakeIntake.Scheme == "http" {
			logsNoSSL = "true"
		}
		skipSSLStr := "false"
		if skipSSL {
			skipSSLStr = "true"
		}
		base = append(base,
			kv("DD_SKIP_SSL_VALIDATION", skipSSLStr),
			kv("DD_REMOTE_CONFIGURATION_NO_TLS_VALIDATION", "true"),
			kv("DD_REMOTE_CONFIGURATION_ENABLED", "true"),
			kv("DD_REMOTE_CONFIGURATION_RC_DD_URL", url),
			kv("DD_REMOTE_CONFIGURATION_NO_TLS", "true"),
			kv("DD_REMOTE_CONFIGURATION_REFRESH_INTERVAL", "5s"),
			kv("DD_PROCESS_CONFIG_PROCESS_DD_URL", url),
			kv("DD_ORCHESTRATOR_EXPLORER_ORCHESTRATOR_DD_URL", url),
			kv("DD_ADDITIONAL_ENDPOINTS", fmt.Sprintf(`{%q: ["FAKEAPIKEY"]}`, url)),
			kv("DD_LOGS_CONFIG_ADDITIONAL_ENDPOINTS", fmt.Sprintf(
				`[{"host": %q, "port": %d, "use_ssl": false}]`,
				env.FakeIntake.Host, env.FakeIntake.Port,
			)),
			kv("DD_LOGS_CONFIG_USE_HTTP", "true"),
			kv("DD_LOGS_CONFIG_LOGS_NO_SSL", logsNoSSL),
		)
	}

	return base
}

func kv(name, value string) ecstypes.KeyValuePair {
	return ecstypes.KeyValuePair{Name: aws.String(name), Value: aws.String(value)}
}

// buildContainerDef returns a map compatible with ECS JSON task definition format.
func buildContainerDef(envVars []ecstypes.KeyValuePair, apiKeyParamName string) map[string]interface{} {
	envJSON := make([]map[string]string, 0, len(envVars))
	for _, e := range envVars {
		envJSON = append(envJSON, map[string]string{
			"name":  aws.ToString(e.Name),
			"value": aws.ToString(e.Value),
		})
	}
	return map[string]interface{}{
		"name":      "datadog-agent",
		"image":     agentImage,
		"cpu":       200,
		"memory":    512,
		"essential": true,
		"linuxParameters": map[string]interface{}{
			"capabilities": map[string]interface{}{
				"add": []string{"SYS_ADMIN", "SYS_RESOURCE", "SYS_PTRACE", "NET_ADMIN", "NET_BROADCAST", "NET_RAW", "IPC_LOCK", "CHOWN"},
			},
		},
		"environment": envJSON,
		"secrets": []map[string]string{
			{"name": "DD_API_KEY", "valueFrom": apiKeyParamName},
		},
		"mountPoints": []map[string]interface{}{
			{"containerPath": "/var/run/docker.sock", "sourceVolume": "docker_sock", "readOnly": true},
			{"containerPath": "/host/proc", "sourceVolume": "proc", "readOnly": true},
			{"containerPath": "/host/sys/fs/cgroup", "sourceVolume": "cgroup", "readOnly": true},
			{"containerPath": "/opt/datadog-agent/run", "sourceVolume": "dd-logpointdir", "readOnly": false},
			{"containerPath": "/var/run/datadog", "sourceVolume": "dd-sockets", "readOnly": false},
			{"containerPath": "/sys/kernel/debug", "sourceVolume": "debug", "readOnly": false},
		},
		"healthCheck": map[string]interface{}{
			"command":     []string{"CMD-SHELL", "agent health"},
			"retries":     2,
			"startPeriod": 10,
			"interval":    30,
			"timeout":     5,
		},
		"portMappings": []map[string]interface{}{
			{"containerPort": 8125, "hostPort": 8125, "protocol": "udp"},
			{"containerPort": 8126, "hostPort": 8126, "protocol": "tcp"},
		},
	}
}

// containerDefsFromJSON deserializes container definitions from JSON bytes back
// into the AWS SDK type slice expected by RegisterTaskDefinition.
func containerDefsFromJSON(data []byte) []ecstypes.ContainerDefinition {
	var raw []json.RawMessage
	if err := json.Unmarshal(data, &raw); err != nil {
		return nil
	}
	defs := make([]ecstypes.ContainerDefinition, 0, len(raw))
	for _, r := range raw {
		var def ecstypes.ContainerDefinition
		if err := json.Unmarshal(r, &def); err == nil {
			defs = append(defs, def)
		}
	}
	return defs
}

// agentVolumes returns the host volume mounts the agent container needs.
func agentVolumes() []ecstypes.Volume {
	vols := []struct{ name, path string }{
		{"docker_sock", "/var/run/docker.sock"},
		{"proc", "/proc"},
		{"cgroup", "/sys/fs/cgroup"},
		{"dd-logpointdir", "/opt/datadog-agent/run"},
		{"dd-sockets", "/var/run/datadog"},
		{"debug", "/sys/kernel/debug"},
	}
	result := make([]ecstypes.Volume, len(vols))
	for i, v := range vols {
		result[i] = ecstypes.Volume{
			Name: aws.String(v.name),
			Host: &ecstypes.HostVolumeProperties{SourcePath: aws.String(v.path)},
		}
	}
	return result
}

// executionRoleFromEnv reads the ECS task execution role ARN from the runner
// profile (same source as config.Env.ECSTaskExecutionRole() in Pulumi path).
func executionRoleFromEnv() *string {
	role, err := runner.GetProfile().ParamStore().GetWithDefault("ecs_task_execution_role", "")
	if err != nil || role == "" {
		return nil
	}
	return aws.String(role)
}

// taskRoleFromEnv reads the ECS task role ARN from the runner profile.
func taskRoleFromEnv() *string {
	role, err := runner.GetProfile().ParamStore().GetWithDefault("ecs_task_role", "")
	if err != nil || role == "" {
		return nil
	}
	return aws.String(role)
}
