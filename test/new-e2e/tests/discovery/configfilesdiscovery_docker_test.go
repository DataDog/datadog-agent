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
)

const (
	agentDiscoveryEndpoint                    = "/api/v2/agentdiscovery"
	configFilesDiscoveryRedisConfigDir        = "/tmp/configfilesdiscovery-redis"
	configFilesDiscoveryRedisContainerPath    = "/usr/local/etc/redis/redis.conf"
	configFilesDiscoveryRedisConfigFileName   = "redis.conf"
	configFilesDiscoveryRedisConfigSentinel   = "configfilesdiscovery-e2e-sentinel"
	configFilesDiscoveryRedisIntegrationName  = "redisdb"
	configFilesDiscoveryRedisContainerRuntime = "docker"
)

const configFilesDiscoveryRedisConfig = `port 6379
appendonly no
maxmemory-policy allkeys-lru
# configfilesdiscovery-e2e-sentinel
`

const configFilesDiscoveryRedisCompose = `version: "3.9"
services:
  redis-configfilesdiscovery:
    image: ghcr.io/datadog/redis:{APPS_VERSION}
    container_name: redis-configfilesdiscovery
    command:
      - redis-server
      - /usr/local/etc/redis/redis.conf
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
	return configFile, nil
}

func (s *configFilesDiscoveryDockerSuite) TestRedisConfigFilePayloadSentToEventPlatform() {
	t := s.T()

	var payloads []*aggregator.AgentDiscoveryPayload
	assert.EventuallyWithT(t, func(c *assert.CollectT) {
		var err error
		payloads, err = s.Env().FakeIntake.Client().GetAgentDiscoveryPayloads()
		assert.NoError(c, err)
		assert.NotEmpty(c, payloads, "no Agent Discovery payloads on %s", agentDiscoveryEndpoint)
		if err != nil || len(payloads) == 0 {
			return
		}

		payload, config, ok := findRedisConfigPayload(payloads)
		assert.True(c, ok, "no redis config payload found in %+v", payloads)
		if !ok {
			return
		}

		assert.Equal(c, configFilesDiscoveryRedisIntegrationName, payload.Integration)
		assert.Equal(c, configFilesDiscoveryRedisContainerRuntime, payload.Runtime)
		assert.NotEmpty(c, payload.HostID)
		assert.NotEmpty(c, payload.RuntimeID)
		assert.False(c, payload.IngestionTimestamp.IsZero())

		assert.Equal(c, configFilesDiscoveryRedisContainerPath, config.Path)
		assert.Equal(c, agentdiscovery.AgentDiscoveryConfigFilePayloadFormat_PAYLOAD_FORMAT_REDIS_CONF, config.PayloadFormat)
		assert.False(c, config.Truncated)
		assert.Equal(c, configFilesDiscoveryRedisConfig, string(config.Content))
		assert.Contains(c, string(config.Content), configFilesDiscoveryRedisConfigSentinel)
	}, 3*time.Minute, 10*time.Second, "timed out waiting for config files discovery payload")

	if t.Failed() {
		s.logConfigFilesDiscoveryDebug(t)
	}
}

func findRedisConfigPayload(payloads []*aggregator.AgentDiscoveryPayload) (*aggregator.AgentDiscoveryPayload, aggregator.AgentDiscoveryConfigFile, bool) {
	for _, payload := range payloads {
		if payload.Integration != configFilesDiscoveryRedisIntegrationName {
			continue
		}
		for _, config := range payload.ConfigFiles {
			if config.Path == configFilesDiscoveryRedisContainerPath {
				return payload, config, true
			}
		}
	}
	return nil, aggregator.AgentDiscoveryConfigFile{}, false
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
