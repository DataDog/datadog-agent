// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package discovery

import (
	"encoding/json"
	"path"
	"strings"
	"testing"
	"time"

	"github.com/DataDog/agent-payload/v5/agentdiscovery"
	"github.com/DataDog/datadog-agent/test/e2e-framework/common/utils"
	"github.com/DataDog/datadog-agent/test/e2e-framework/components/datadog/apps"
	"github.com/DataDog/datadog-agent/test/e2e-framework/components/datadog/dockeragentparams"
	"github.com/DataDog/datadog-agent/test/e2e-framework/components/remote"
	"github.com/DataDog/datadog-agent/test/e2e-framework/resources/aws"
	scendocker "github.com/DataDog/datadog-agent/test/e2e-framework/scenarios/aws/ec2docker"
	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/e2e"
	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/environments"
	awsdocker "github.com/DataDog/datadog-agent/test/e2e-framework/testing/provisioners/aws/docker"
	"github.com/DataDog/datadog-agent/test/fakeintake/aggregator"
	"github.com/pulumi/pulumi/sdk/v3/go/pulumi"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const (
	agentDiscoveryEndpoint                    = "/api/v2/agentdiscovery"
	configFilesDiscoveryRedisConfigDir        = "/tmp/configfilesdiscovery-redis"
	configFilesDiscoveryRedisContainerName    = "redis-configfilesdiscovery"
	configFilesDiscoveryRedisContainerPath    = "/usr/local/etc/redis/redis.conf"
	configFilesDiscoveryRedisConfigFileName   = "redis.conf"
	configFilesDiscoveryRedisStartFileName    = "start"
	configFilesDiscoveryRedisStartScriptName  = "start-redis.sh"
	configFilesDiscoveryRedisConfigSentinel   = "configfilesdiscovery-e2e-sentinel"
	configFilesDiscoveryRedisIntegrationName  = "redisdb"
	configFilesDiscoveryRedisContainerRuntime = "docker"
)

const configFilesDiscoveryRedisConfig = `port 6379
appendonly no
maxmemory-policy allkeys-lru
# configfilesdiscovery-e2e-sentinel
`

const configFilesDiscoveryRedisStartScript = `#!/bin/sh
set -eu

while [ ! -f /configfilesdiscovery/start ]; do
  sleep 1
done

exec redis-server /usr/local/etc/redis/redis.conf
`

const configFilesDiscoveryRedisCompose = `version: "3.9"
services:
  redis-configfilesdiscovery:
    image: ghcr.io/datadog/redis:{APPS_VERSION}
    container_name: redis-configfilesdiscovery
    command:
      - /bin/sh
      - /configfilesdiscovery/start-redis.sh
    labels:
      com.datadoghq.ad.checks: |
        {
          "redisdb": {
            "instances": [
              {
                "host": "%%host%%",
                "port": 6379
              }
            ]
          }
        }
    volumes:
      - ${CONFIG_FILES_DISCOVERY_REDIS_CONFIG_DIR}:/configfilesdiscovery:ro
      - ${CONFIG_FILES_DISCOVERY_REDIS_CONFIG_DIR}/redis.conf:/usr/local/etc/redis/redis.conf:ro
`

type configFilesDiscoveryDockerSuite struct {
	e2e.BaseSuite[environments.DockerHost]
}

func TestConfigFilesDiscoveryDockerSuite(t *testing.T) {
	t.Parallel()

	redisCompose := strings.ReplaceAll(configFilesDiscoveryRedisCompose, "{APPS_VERSION}", apps.Version)
	agentOpts := []dockeragentparams.Option{
		dockeragentparams.WithAgentServiceEnvVariable("DD_CONFIG_FILES_DISCOVERY_ENABLED", pulumi.StringPtr("true")),
		dockeragentparams.WithAgentServiceEnvVariable("DD_CONFIG_FILES_DISCOVERY_FORWARDER_USE_COMPRESSION", pulumi.StringPtr("false")),
		dockeragentparams.WithAgentServiceEnvVariable("DD_CONFIG_FILES_DISCOVERY_FORWARDER_BATCH_WAIT", pulumi.StringPtr("0.1")),
		dockeragentparams.WithAgentServiceEnvVariable("DD_CONFIG_FILES_DISCOVERY_HEARTBEAT_INTERVAL", pulumi.StringPtr("10s")),
		dockeragentparams.WithAgentServiceEnvVariable("DD_CONFIG_FILES_DISCOVERY_HEARTBEAT_JITTER", pulumi.StringPtr("0s")),
		dockeragentparams.WithAgentServiceEnvVariable("DD_CONFIG_FILES_DISCOVERY_STARTUP_JITTER", pulumi.StringPtr("0s")),
		dockeragentparams.WithExtraComposeManifest("configfilesdiscovery-redis", pulumi.String(redisCompose)),
		dockeragentparams.WithEnvironmentVariables(pulumi.StringMap{
			"CONFIG_FILES_DISCOVERY_REDIS_CONFIG_DIR": pulumi.String(configFilesDiscoveryRedisConfigDir),
		}),
	}

	e2e.Run(t, &configFilesDiscoveryDockerSuite{}, e2e.WithProvisioner(awsdocker.Provisioner(
		awsdocker.WithRunOptions(
			scendocker.WithPreAgentInstallHook(createConfigFilesDiscoveryRedisConfig),
			scendocker.WithAgentOptions(agentOpts...),
		),
	)))
}

func createConfigFilesDiscoveryRedisConfig(_ *aws.Environment, host *remote.Host) (pulumi.Resource, error) {
	fileManager := host.OS.FileManager()
	createDir, err := fileManager.CreateDirectory(configFilesDiscoveryRedisConfigDir, false)
	if err != nil {
		return nil, err
	}

	configPath := path.Join(configFilesDiscoveryRedisConfigDir, configFilesDiscoveryRedisConfigFileName)
	configFile, err := fileManager.CopyInlineFile(
		pulumi.String(configFilesDiscoveryRedisConfig),
		configPath,
		utils.PulumiDependsOn(createDir),
	)
	if err != nil {
		return nil, err
	}

	startScriptPath := path.Join(configFilesDiscoveryRedisConfigDir, configFilesDiscoveryRedisStartScriptName)
	startScript, err := fileManager.CopyInlineFile(
		pulumi.String(configFilesDiscoveryRedisStartScript),
		startScriptPath,
		utils.PulumiDependsOn(configFile),
	)
	if err != nil {
		return nil, err
	}
	return startScript, nil
}

func (s *configFilesDiscoveryDockerSuite) TestRedisConfigFileDiscoveredAfterProcessStartsAndHeartbeatSentToEventPlatform() {
	t := s.T()
	host := s.Env().RemoteHost
	startFilePath := path.Join(configFilesDiscoveryRedisConfigDir, configFilesDiscoveryRedisStartFileName)

	t.Cleanup(func() {
		if _, cleanupErr := host.Execute("sudo rm -f " + startFilePath); cleanupErr != nil {
			t.Logf("failed to remove redis start file: %v", cleanupErr)
		}
		if _, cleanupErr := host.Execute("sudo docker restart " + configFilesDiscoveryRedisContainerName); cleanupErr != nil {
			t.Logf("failed to return redis container to its waiting state: %v", cleanupErr)
		}
	})
	_, err := host.Execute("sudo docker stop " + configFilesDiscoveryRedisContainerName)
	require.NoError(t, err)
	_, err = host.Execute("sudo rm -f " + startFilePath)
	require.NoError(t, err)

	require.Eventually(t, func() bool {
		return !strings.Contains(s.Env().Agent.Client.ConfigCheck(), configFilesDiscoveryRedisIntegrationName)
	}, time.Minute, time.Second, "redis AD config remained scheduled after the container stopped")
	require.NoError(t, s.Env().FakeIntake.Client().FlushServerAndResetAggregators())

	_, err = host.Execute("sudo docker start " + configFilesDiscoveryRedisContainerName)
	require.NoError(t, err)
	require.EventuallyWithT(t, func(c *assert.CollectT) {
		// Docker needs the PID column to map the ps output back to container processes,
		// even though this assertion only inspects the command arguments.
		processes, processErr := host.Execute("sudo docker top " + configFilesDiscoveryRedisContainerName + " -eo pid,args")
		if !assert.NoError(c, processErr) {
			return
		}
		assert.Contains(c, processes, configFilesDiscoveryRedisStartScriptName)
		assert.NotContains(c, processes, "redis-server")
		assert.Contains(c, s.Env().Agent.Client.ConfigCheck(), configFilesDiscoveryRedisIntegrationName)
	}, 2*time.Minute, 2*time.Second, "redis AD config was not scheduled while the wrapper was waiting")

	_, err = host.Execute("sudo touch " + startFilePath)
	require.NoError(t, err)

	assert.EventuallyWithT(t, func(c *assert.CollectT) {
		payloads, err := s.Env().FakeIntake.Client().GetAgentDiscoveryPayloads()
		if !assert.NoError(c, err) {
			return
		}
		if !assert.NotEmpty(c, payloads, "no Agent Discovery payloads on %s", agentDiscoveryEndpoint) {
			return
		}

		redisPayloads := findRedisConfigPayloads(payloads)
		assert.GreaterOrEqual(c, len(redisPayloads), 2, "fewer than two redis config payloads found in %+v", payloads)
		if len(redisPayloads) < 2 {
			return
		}

		for _, redisPayload := range redisPayloads {
			assert.Equal(c, configFilesDiscoveryRedisIntegrationName, redisPayload.payload.Integration)
			assert.Equal(c, configFilesDiscoveryRedisContainerRuntime, redisPayload.payload.Runtime)
			assert.NotEmpty(c, redisPayload.payload.HostID)
			assert.NotEmpty(c, redisPayload.payload.RuntimeID)
			assert.False(c, redisPayload.payload.IngestionTimestamp.IsZero())

			assert.Equal(c, configFilesDiscoveryRedisContainerPath, redisPayload.config.Path)
			assert.Equal(c, agentdiscovery.AgentDiscoveryConfigFilePayloadFormat_PAYLOAD_FORMAT_REDIS_CONF, redisPayload.config.PayloadFormat)
			assert.False(c, redisPayload.config.Truncated)
			assert.Equal(c, configFilesDiscoveryRedisConfig, string(redisPayload.config.Content))
			assert.Contains(c, string(redisPayload.config.Content), configFilesDiscoveryRedisConfigSentinel)
		}
	}, 3*time.Minute, 10*time.Second, "timed out waiting for config files discovery payload")

	if t.Failed() {
		s.logConfigFilesDiscoveryDebug(t)
	}
}

type redisConfigPayload struct {
	payload *aggregator.AgentDiscoveryPayload
	config  aggregator.AgentDiscoveryConfigFile
}

func findRedisConfigPayloads(payloads []*aggregator.AgentDiscoveryPayload) []redisConfigPayload {
	var redisPayloads []redisConfigPayload
	for _, payload := range payloads {
		if payload.Integration != configFilesDiscoveryRedisIntegrationName {
			continue
		}
		for _, config := range payload.ConfigFiles {
			if config.Path == configFilesDiscoveryRedisContainerPath {
				redisPayloads = append(redisPayloads, redisConfigPayload{
					payload: payload,
					config:  config,
				})
			}
		}
	}
	return redisPayloads
}

func (s *configFilesDiscoveryDockerSuite) logConfigFilesDiscoveryDebug(t *testing.T) {
	client := s.Env().FakeIntake.Client()

	if routeStats, err := client.RouteStats(); err != nil {
		t.Logf("failed to get fakeintake route stats: %v", err)
	} else {
		t.Logf("fakeintake route stats: %+v", routeStats)
	}

	if payloads, err := client.GetAgentDiscoveryPayloads(); err != nil {
		t.Logf("failed to get Agent Discovery payloads: %v", err)
	} else {
		for i, payload := range payloads {
			payloadJSON, err := json.MarshalIndent(payload, "", "  ")
			if err != nil {
				t.Logf("failed to format Agent Discovery payload %d: %v", i, err)
				continue
			}
			t.Logf("Agent Discovery payload %d: %s", i, payloadJSON)
		}
	}

	if s.Env().Agent != nil && s.Env().Agent.Client != nil {
		if status := s.Env().Agent.Client.Status(); status != nil {
			t.Logf("agent status:\n%s", status.Content)
		}
		t.Logf("agent configcheck:\n%s", s.Env().Agent.Client.ConfigCheck())
	}
}
