// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package discovery

import (
	_ "embed"
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
	agentDiscoveryEndpoint = "/api/v2/agentdiscovery"
	startMarkerFileName    = "start"
	dockerRuntime          = "docker"
)

const (
	redisConfigDir               = "/tmp/configfilesdiscovery-redis"
	redisExplicitContainerName   = "redis-configfilesdiscovery-explicit"
	redisDefaultContainerName    = "redis-configfilesdiscovery-default"
	redisEnvContainerName        = "redis-env-configfilesdiscovery"
	redisExplicitContainerPath   = "/configfilesdiscovery/redis-explicit.conf"
	redisDefaultContainerPath    = "/etc/redis/redis.conf"
	redisExplicitConfigFileName  = "redis-explicit.conf"
	redisDefaultConfigFileName   = "redis-default.conf"
	redisExplicitStartScriptName = "start-redis.sh"
	redisDefaultStartScriptName  = "start-default-redis.sh"
	redisExplicitConfigSentinel  = "configfilesdiscovery-explicit-e2e-sentinel"
	redisDefaultConfigSentinel   = "configfilesdiscovery-default-e2e-sentinel"
	redisIntegrationName         = "redisdb"
	redisTLSCertFile             = "/etc/redis/tls.crt"
)

const (
	kafkaConfigDir        = "/tmp/configfilesdiscovery-kafka"
	kafkaContainerName    = "kafka-configfilesdiscovery-default"
	kafkaEnvContainerName = "kafka-env-configfilesdiscovery"
	kafkaContainerPath    = "/etc/kafka/kafka.properties"
	kafkaStartScriptName  = "start-kafka.sh"
	kafkaConfigSentinel   = "configfilesdiscovery-e2e-sentinel"
	kafkaIntegrationName  = "kafka"
	kafkaClusterID        = "MkU3OEVBNTcwNTJENDM2Qk"
	kafkaTokenEndpoint    = "https://identity.example/oauth2/token"
)

const (
	postgresConfigDir       = "/tmp/configfilesdiscovery-postgres"
	postgresContainerName   = "postgres-configfilesdiscovery"
	postgresIntegrationName = "postgres"
	postgresDBName          = "configfilesdiscovery"
	postgresUser            = "configfilesdiscovery"
)

const (
	sparkMasterContainerName = "spark-master-env-configfilesdiscovery"
	sparkIntegrationName     = "spark"
	sparkMasterHost          = "spark-master-env-configfilesdiscovery"
	sparkMasterClass         = "org.apache.spark.deploy.master.Master"
)

//go:embed testdata/compose/docker-compose.configfilesdiscovery-redis.yaml
var redisComposeTemplate string

//go:embed testdata/compose/docker-compose.configfilesdiscovery-kafka.yaml
var kafkaCompose string

//go:embed testdata/compose/docker-compose.configfilesdiscovery-postgres.yaml
var postgresCompose string

//go:embed testdata/compose/docker-compose.configfilesdiscovery-spark-master.yaml
var sparkMasterCompose string

const redisExplicitConfig = `port 6379
appendonly no
maxmemory-policy allkeys-lru
# configfilesdiscovery-explicit-e2e-sentinel
`

const redisDefaultConfig = `port 6379
appendonly no
maxmemory-policy allkeys-lru
# configfilesdiscovery-default-e2e-sentinel
`

const redisExplicitStartScript = `#!/bin/sh
set -eu

while [ ! -f /configfilesdiscovery/start ]; do
  sleep 1
done

# Redis rewrites its process title by default, which can hide the startup
# arguments before workloadmeta's periodic process scan observes them.
exec redis-server /configfilesdiscovery/redis-explicit.conf --set-proc-title no
`

const redisDefaultStartScript = `#!/bin/sh
set -eu

while [ ! -f /configfilesdiscovery/start ]; do
  sleep 1
done

exec redis-server /etc/redis/redis.conf
`

const kafkaStartScript = `#!/usr/bin/env bash
set -euo pipefail

/etc/confluent/docker/configure
/etc/confluent/docker/ensure

while [ ! -f /configfilesdiscovery/start ]; do
  sleep 1
done

exec /etc/confluent/docker/launch
`

type configFilesDiscoveryDockerSuite struct {
	e2e.BaseSuite[environments.DockerHost]
}

type configFilesDiscoveryFixtureFile struct {
	name    string
	content string
}

type configFilesDiscoveryContainerFixture struct {
	integrationName     string
	configDir           string
	containerNames      []string
	startContainerNames []string
}

type configFilePayloadExpectation struct {
	integrationName string
	configPath      string
	payloadFormat   agentdiscovery.AgentDiscoveryConfigFilePayloadFormat
}

func TestConfigFilesDiscoveryDockerSuite(t *testing.T) {
	t.Parallel()

	redisCompose := strings.ReplaceAll(redisComposeTemplate, "{APPS_VERSION}", apps.Version)
	agentOpts := []dockeragentparams.Option{
		dockeragentparams.WithAgentServiceEnvVariable("DD_CONFIG_FILES_DISCOVERY_ENABLED", pulumi.StringPtr("true")),
		dockeragentparams.WithAgentServiceEnvVariable("DD_CONFIG_FILES_DISCOVERY_FORWARDER_USE_COMPRESSION", pulumi.StringPtr("false")),
		dockeragentparams.WithAgentServiceEnvVariable("DD_CONFIG_FILES_DISCOVERY_FORWARDER_BATCH_WAIT", pulumi.StringPtr("0.1")),
		dockeragentparams.WithAgentServiceEnvVariable("DD_CONFIG_FILES_DISCOVERY_HEARTBEAT_INTERVAL", pulumi.StringPtr("10s")),
		dockeragentparams.WithAgentServiceEnvVariable("DD_CONFIG_FILES_DISCOVERY_HEARTBEAT_JITTER", pulumi.StringPtr("0s")),
		dockeragentparams.WithAgentServiceEnvVariable("DD_CONFIG_FILES_DISCOVERY_STARTUP_JITTER", pulumi.StringPtr("0s")),
		dockeragentparams.WithExtraComposeManifest("configfilesdiscovery-redis", pulumi.String(redisCompose)),
		dockeragentparams.WithExtraComposeManifest("configfilesdiscovery-kafka", pulumi.String(kafkaCompose)),
		dockeragentparams.WithExtraComposeManifest("configfilesdiscovery-postgres", pulumi.String(postgresCompose)),
		dockeragentparams.WithExtraComposeManifest("configfilesdiscovery-spark-master", pulumi.String(sparkMasterCompose)),
		dockeragentparams.WithEnvironmentVariables(pulumi.StringMap{
			"CONFIG_FILES_DISCOVERY_REDIS_CONFIG_DIR": pulumi.String(redisConfigDir),
			"CONFIG_FILES_DISCOVERY_KAFKA_CONFIG_DIR": pulumi.String(kafkaConfigDir),
		}),
	}

	e2e.Run(t, &configFilesDiscoveryDockerSuite{}, e2e.WithProvisioner(awsdocker.Provisioner(
		awsdocker.WithRunOptions(
			scendocker.WithPreAgentInstallHook(createConfigFilesDiscoveryRedisConfig),
			scendocker.WithPreAgentInstallHook(createConfigFilesDiscoveryKafkaConfig),
			scendocker.WithAgentOptions(agentOpts...),
		),
	)))
}

func createConfigFilesDiscoveryRedisConfig(_ *aws.Environment, host *remote.Host) (pulumi.Resource, error) {
	return createConfigFilesDiscoveryFixtureFiles(
		host,
		redisConfigDir,
		[]configFilesDiscoveryFixtureFile{
			{name: redisExplicitConfigFileName, content: redisExplicitConfig},
			{name: redisDefaultConfigFileName, content: redisDefaultConfig},
			{name: redisExplicitStartScriptName, content: redisExplicitStartScript},
			{name: redisDefaultStartScriptName, content: redisDefaultStartScript},
		},
	)
}

func createConfigFilesDiscoveryKafkaConfig(_ *aws.Environment, host *remote.Host) (pulumi.Resource, error) {
	return createConfigFilesDiscoveryFixtureFiles(
		host,
		kafkaConfigDir,
		[]configFilesDiscoveryFixtureFile{
			{name: kafkaStartScriptName, content: kafkaStartScript},
		},
	)
}

func createConfigFilesDiscoveryFixtureFiles(host *remote.Host, configDir string, files []configFilesDiscoveryFixtureFile) (pulumi.Resource, error) {
	fileManager := host.OS.FileManager()
	createDir, err := fileManager.CreateDirectory(configDir, false)
	if err != nil {
		return nil, err
	}

	var dependency pulumi.Resource = createDir
	for _, file := range files {
		dependency, err = fileManager.CopyInlineFile(
			pulumi.String(file.content),
			path.Join(configDir, file.name),
			utils.PulumiDependsOn(dependency),
		)
		if err != nil {
			return nil, err
		}
	}

	return dependency, nil
}

func (s *configFilesDiscoveryDockerSuite) prepareConfigFilesDiscoveryContainers(t *testing.T, fixture configFilesDiscoveryContainerFixture) string {
	t.Helper()

	host := s.Env().RemoteHost
	startFilePath := path.Join(fixture.configDir, startMarkerFileName)
	containerNames := strings.Join(fixture.containerNames, " ")
	startContainerNames := containerNames
	if len(fixture.startContainerNames) > 0 {
		startContainerNames = strings.Join(fixture.startContainerNames, " ")
	}

	t.Cleanup(func() {
		if t.Failed() {
			s.logConfigFilesDiscoveryDebug(t)
			for _, containerName := range fixture.containerNames {
				if logs, logsErr := host.Execute("sudo docker logs --tail 200 " + containerName); logsErr != nil {
					t.Logf("failed to get %s container logs: %v", containerName, logsErr)
				} else {
					t.Logf("%s container logs:\n%s", containerName, logs)
				}
			}
		}
		if _, cleanupErr := host.Execute("sudo rm -f " + startFilePath); cleanupErr != nil {
			t.Logf("failed to remove %s start file: %v", fixture.integrationName, cleanupErr)
		}
		if _, cleanupErr := host.Execute("sudo docker restart " + containerNames); cleanupErr != nil {
			t.Logf("failed to restart %s containers: %v", fixture.integrationName, cleanupErr)
		}
	})

	_, err := host.Execute("sudo docker stop " + containerNames)
	require.NoError(t, err)
	_, err = host.Execute("sudo rm -f " + startFilePath)
	require.NoError(t, err)
	require.Eventually(t, func() bool {
		return !isIntegrationScheduled(s.Env().Agent.Client.ConfigCheck(), fixture.integrationName)
	}, time.Minute, time.Second, "%s AD config remained scheduled after its containers stopped", fixture.integrationName)
	require.NoError(t, s.Env().FakeIntake.Client().FlushServerAndResetAggregators())
	_, err = host.Execute("sudo docker start " + startContainerNames)
	require.NoError(t, err)

	return startFilePath
}

func isIntegrationScheduled(configCheckOutput string, integrationName string) bool {
	return strings.Contains(configCheckOutput, "=== "+integrationName+" check ===")
}

func (s *configFilesDiscoveryDockerSuite) TestKafkaEnvVarsDiscoveredWithoutConfigFile() {
	t := s.T()
	s.prepareConfigFilesDiscoveryContainers(t, configFilesDiscoveryContainerFixture{
		integrationName:     kafkaIntegrationName,
		configDir:           kafkaConfigDir,
		containerNames:      []string{kafkaContainerName, kafkaEnvContainerName},
		startContainerNames: []string{kafkaEnvContainerName},
	})

	require.EventuallyWithT(t, func(c *assert.CollectT) {
		assert.True(c, isIntegrationScheduled(s.Env().Agent.Client.ConfigCheck(), kafkaIntegrationName))

		payloads, err := s.Env().FakeIntake.Client().GetAgentDiscoveryPayloads()
		if !assert.NoError(c, err) {
			return
		}
		kafkaPayloads := findEnvPayloads(payloads, kafkaIntegrationName)
		if !assert.NotEmpty(c, kafkaPayloads, "no kafka env payloads found in %+v", payloads) {
			return
		}

		for _, payload := range kafkaPayloads {
			assertAgentDiscoveryPayload(c, payload, kafkaIntegrationName)
			assert.Empty(c, payload.ConfigFiles)

			envVars := make(map[string]string, len(payload.EnvVars))
			for _, envVar := range payload.EnvVars {
				envVars[envVar.Name] = envVar.Value
			}
			assert.Equal(c, kafkaClusterID, envVars["CLUSTER_ID"])
			assert.Equal(c, "1", envVars["KAFKA_NODE_ID"])
			assert.Equal(c, "broker,controller", envVars["KAFKA_PROCESS_ROLES"])
			assert.Equal(c, kafkaTokenEndpoint, envVars["KAFKA_SASL_OAUTHBEARER_TOKEN_ENDPOINT_URL"])
			assert.NotContains(c, envVars, "KAFKA_CFG_OAUTH_ACCESS_TOKEN")
		}
	}, 3*time.Minute, 10*time.Second, "timed out waiting for kafka env var discovery payload")
}

func (s *configFilesDiscoveryDockerSuite) TestRedisEnvVarsDiscoveredWithoutConfigFile() {
	t := s.T()
	s.prepareConfigFilesDiscoveryContainers(t, configFilesDiscoveryContainerFixture{
		integrationName: redisIntegrationName,
		configDir:       redisConfigDir,
		containerNames: []string{
			redisExplicitContainerName,
			redisDefaultContainerName,
			redisEnvContainerName,
		},
		startContainerNames: []string{redisEnvContainerName},
	})

	require.EventuallyWithT(t, func(c *assert.CollectT) {
		assert.True(c, isIntegrationScheduled(s.Env().Agent.Client.ConfigCheck(), redisIntegrationName))

		payloads, err := s.Env().FakeIntake.Client().GetAgentDiscoveryPayloads()
		if !assert.NoError(c, err) {
			return
		}
		redisPayloads := findEnvPayloads(payloads, redisIntegrationName)
		if !assert.NotEmpty(c, redisPayloads, "no redis env payloads found in %+v", payloads) {
			return
		}

		for _, payload := range redisPayloads {
			assertAgentDiscoveryPayload(c, payload, redisIntegrationName)
			assert.Empty(c, payload.ConfigFiles)

			envVars := make(map[string]string, len(payload.EnvVars))
			for _, envVar := range payload.EnvVars {
				envVars[envVar.Name] = envVar.Value
			}
			assert.Equal(c, "yes", envVars["REDIS_AOF_ENABLED"])
			assert.Equal(c, "6380", envVars["REDIS_PORT_NUMBER"])
			assert.Equal(c, redisTLSCertFile, envVars["REDIS_TLS_CERT_FILE"])
			assert.NotContains(c, envVars, "REDIS_PASSWORD")
			assert.NotContains(c, envVars, "REDIS_REQUIREPASS")
		}
	}, 3*time.Minute, 10*time.Second, "timed out waiting for redis env var discovery payload")
}

func (s *configFilesDiscoveryDockerSuite) TestPostgresEnvVarsDiscovered() {
	t := s.T()
	s.prepareConfigFilesDiscoveryContainers(t, configFilesDiscoveryContainerFixture{
		integrationName: postgresIntegrationName,
		configDir:       postgresConfigDir,
		containerNames:  []string{postgresContainerName},
	})

	require.EventuallyWithT(t, func(c *assert.CollectT) {
		assert.True(c, isIntegrationScheduled(s.Env().Agent.Client.ConfigCheck(), postgresIntegrationName))

		payloads, err := s.Env().FakeIntake.Client().GetAgentDiscoveryPayloads()
		if !assert.NoError(c, err) {
			return
		}
		postgresPayloads := findEnvPayloads(payloads, postgresIntegrationName)
		if !assert.NotEmpty(c, postgresPayloads, "no postgres env payloads found in %+v", payloads) {
			return
		}

		for _, payload := range postgresPayloads {
			assertAgentDiscoveryPayload(c, payload, postgresIntegrationName)
			assert.Empty(c, payload.ConfigFiles)

			envVars := make(map[string]string, len(payload.EnvVars))
			for _, envVar := range payload.EnvVars {
				envVars[envVar.Name] = envVar.Value
			}
			assert.Equal(c, "/var/lib/postgresql/data/configfilesdiscovery", envVars["PGDATA"])
			assert.Equal(c, "5432", envVars["PGPORT"])
			assert.Equal(c, postgresDBName, envVars["POSTGRES_DB"])
			assert.Equal(c, postgresUser, envVars["POSTGRES_USER"])
			assert.Equal(c, "/bitnami/postgresql/data/configfilesdiscovery", envVars["POSTGRESQL_DATA_DIR"])
			assert.Equal(c, "configfilesdiscovery-bitnami", envVars["POSTGRESQL_DATABASE"])
			assert.Equal(c, "configfilesdiscovery-bitnami", envVars["POSTGRESQL_USERNAME"])
			assert.Equal(c, "/bitnami/postgresql/wal", envVars["POSTGRESQL_INITDB_WAL_DIR"])
			assert.Equal(c, "200", envVars["POSTGRESQL_MAX_CONNECTIONS"])
			assert.Equal(c, "scram-sha-256", envVars["POSTGRESQL_PASSWORD_ENCRYPTION"])
			assert.Equal(c, "no", envVars["POSTGRESQL_ENABLE_TLS"])
			assert.Equal(c, "master", envVars["POSTGRESQL_REPLICATION_MODE"])
			assert.NotContains(c, envVars, "POSTGRES_PASSWORD")
			assert.NotContains(c, envVars, "POSTGRESQL_PASSWORD")
			assert.NotContains(c, envVars, "POSTGRESQL_PASSWORD_FILE")
			assert.NotContains(c, envVars, "POSTGRESQL_LDAP_URL")
		}
	}, 3*time.Minute, 10*time.Second, "timed out waiting for postgres env var discovery payload")
}

func (s *configFilesDiscoveryDockerSuite) TestSparkMasterEnvVarsDiscovered() {
	t := s.T()
	s.prepareConfigFilesDiscoveryContainers(t, configFilesDiscoveryContainerFixture{
		integrationName: sparkIntegrationName,
		containerNames:  []string{sparkMasterContainerName},
	})

	require.EventuallyWithT(t, func(c *assert.CollectT) {
		processes, err := s.Env().RemoteHost.Execute("sudo docker top " + sparkMasterContainerName + " -eo pid,args")
		if !assert.NoError(c, err) {
			return
		}
		assert.Contains(c, processes, sparkMasterClass)
		assert.True(c, isIntegrationScheduled(s.Env().Agent.Client.ConfigCheck(), sparkIntegrationName))

		payloads, err := s.Env().FakeIntake.Client().GetAgentDiscoveryPayloads()
		if !assert.NoError(c, err) {
			return
		}
		sparkPayloads := findEnvPayloads(payloads, sparkIntegrationName)
		if !assert.NotEmpty(c, sparkPayloads, "no Spark Master env payloads found in %+v", payloads) {
			return
		}

		for _, payload := range sparkPayloads {
			assertAgentDiscoveryPayload(c, payload, sparkIntegrationName)
			assert.Empty(c, payload.ConfigFiles)

			envVars := make(map[string]string, len(payload.EnvVars))
			for _, envVar := range payload.EnvVars {
				envVars[envVar.Name] = envVar.Value
			}
			assert.Equal(c, sparkMasterHost, envVars["SPARK_MASTER_HOST"])
			assert.Equal(c, "7077", envVars["SPARK_MASTER_PORT"])
			assert.Equal(c, "8080", envVars["SPARK_MASTER_WEBUI_PORT"])
			assert.Equal(c, "1g", envVars["SPARK_DAEMON_MEMORY"])
			assert.Equal(c, "/tmp/spark-local", envVars["SPARK_LOCAL_DIRS"])
			assert.Equal(c, "no", envVars["SPARK_RPC_ENCRYPTION_ENABLED"])
			assert.Equal(c, "no", envVars["SPARK_SSL_ENABLED"])
			assert.NotContains(c, envVars, "SPARK_RPC_AUTHENTICATION_SECRET")
			assert.NotContains(c, envVars, "SPARK_MASTER_OPTS")
			assert.NotContains(c, envVars, "SPARK_DAEMON_JAVA_OPTS")
			assert.NotContains(c, envVars, "JAVA_TOOL_OPTIONS")
		}
	}, 3*time.Minute, 10*time.Second, "timed out waiting for Spark Master env var discovery payload")
}

func (s *configFilesDiscoveryDockerSuite) TestKafkaDefaultConfigFileDiscovered() {
	t := s.T()
	host := s.Env().RemoteHost
	startFilePath := s.prepareConfigFilesDiscoveryContainers(t, configFilesDiscoveryContainerFixture{
		integrationName: kafkaIntegrationName,
		configDir:       kafkaConfigDir,
		containerNames:  []string{kafkaContainerName, kafkaEnvContainerName},
	})

	require.EventuallyWithT(t, func(c *assert.CollectT) {
		processes, processErr := host.Execute("sudo docker top " + kafkaContainerName + " -eo pid,args")
		if !assert.NoError(c, processErr) {
			return
		}
		assert.Contains(c, processes, kafkaStartScriptName)
		assert.NotContains(c, processes, "kafka-server-start")
		assert.NotContains(c, processes, "kafka.Kafka")
		assert.NotContains(c, processes, kafkaContainerPath)

		config, configErr := host.Execute("sudo docker exec " + kafkaContainerName + " cat " + kafkaContainerPath)
		if !assert.NoError(c, configErr) {
			return
		}
		assert.Contains(c, config, "broker.rack="+kafkaConfigSentinel)
		for _, examplePath := range []string{"/etc/kafka/server.properties", "/etc/kafka/kraft/server.properties"} {
			_, exampleErr := host.Execute("sudo docker exec " + kafkaContainerName + " test -f " + examplePath)
			assert.NoError(c, exampleErr, "expected Confluent example config %q to be present", examplePath)
		}
		assert.True(c, isIntegrationScheduled(s.Env().Agent.Client.ConfigCheck(), kafkaIntegrationName))
	}, 2*time.Minute, 2*time.Second, "kafka AD config was not scheduled after the image generated its properties file")

	expectedKafkaPayload := configFilePayloadExpectation{
		integrationName: kafkaIntegrationName,
		configPath:      kafkaContainerPath,
		payloadFormat:   agentdiscovery.AgentDiscoveryConfigFilePayloadFormat_PAYLOAD_FORMAT_PROPERTIES,
	}
	require.EventuallyWithT(t, func(c *assert.CollectT) {
		payloads, payloadErr := s.Env().FakeIntake.Client().GetAgentDiscoveryPayloads()
		if !assert.NoError(c, payloadErr) {
			return
		}
		kafkaPayloads := findConfigFilePayloads(payloads, kafkaIntegrationName, kafkaContainerPath)
		if !assert.NotEmpty(c, kafkaPayloads, "kafka default-path config was not collected while the runtime command was opaque") {
			return
		}

		for _, kafkaPayload := range kafkaPayloads {
			assertConfigFilePayload(c, kafkaPayload, expectedKafkaPayload)
			assert.Contains(c, string(kafkaPayload.config.Content), "broker.rack="+kafkaConfigSentinel)
		}
	}, 2*time.Minute, 2*time.Second, "kafka default-path fallback did not run while the wrapper was waiting")

	_, err := host.Execute("sudo touch " + startFilePath)
	require.NoError(t, err)
	require.EventuallyWithT(t, func(c *assert.CollectT) {
		processes, processErr := host.Execute("sudo docker top " + kafkaContainerName + " -eo pid,args")
		if !assert.NoError(c, processErr) {
			return
		}
		assert.Contains(c, processes, "kafka.Kafka")
		assert.Contains(c, processes, kafkaContainerPath)
	}, 2*time.Minute, 2*time.Second, "kafka broker did not start with the generated properties file")
}

func (s *configFilesDiscoveryDockerSuite) TestRedisConfigFilesDiscoveredAndHeartbeatsSentToEventPlatform() {
	t := s.T()
	host := s.Env().RemoteHost
	startFilePath := s.prepareConfigFilesDiscoveryContainers(t, configFilesDiscoveryContainerFixture{
		integrationName: redisIntegrationName,
		configDir:       redisConfigDir,
		containerNames: []string{
			redisExplicitContainerName,
			redisDefaultContainerName,
			redisEnvContainerName,
		},
		startContainerNames: []string{redisExplicitContainerName, redisDefaultContainerName},
	})

	require.EventuallyWithT(t, func(c *assert.CollectT) {
		// Docker needs the PID column to map the ps output back to container processes,
		// even though this assertion only inspects the command arguments.
		wrappers := []struct {
			containerName   string
			startScriptName string
		}{
			{containerName: redisExplicitContainerName, startScriptName: redisExplicitStartScriptName},
			{containerName: redisDefaultContainerName, startScriptName: redisDefaultStartScriptName},
		}
		for _, wrapper := range wrappers {
			processes, processErr := host.Execute("sudo docker top " + wrapper.containerName + " -eo pid,args")
			if !assert.NoError(c, processErr) {
				return
			}
			assert.Contains(c, processes, wrapper.startScriptName)
			assert.NotContains(c, processes, "redis-server")
		}
		assert.True(c, isIntegrationScheduled(s.Env().Agent.Client.ConfigCheck(), redisIntegrationName))
	}, 2*time.Minute, 2*time.Second, "redis AD config was not scheduled while the wrapper was waiting")
	require.EventuallyWithT(t, func(c *assert.CollectT) {
		payloads, payloadErr := s.Env().FakeIntake.Client().GetAgentDiscoveryPayloads()
		if !assert.NoError(c, payloadErr) {
			return
		}
		defaultPayloads := findConfigFilePayloads(payloads, redisIntegrationName, redisDefaultContainerPath)
		if !assert.NotEmpty(c, defaultPayloads, "default-path config was not collected while the runtime command was opaque") {
			return
		}
		assert.Equal(c, redisDefaultConfig, string(defaultPayloads[0].config.Content))
	}, 2*time.Minute, 2*time.Second, "redis default-path fallback did not run while the wrapper was waiting")

	expectedRedisConfigs := []struct {
		path     string
		content  string
		sentinel string
	}{
		{
			path:     redisExplicitContainerPath,
			content:  redisExplicitConfig,
			sentinel: redisExplicitConfigSentinel,
		},
		{
			path:     redisDefaultContainerPath,
			content:  redisDefaultConfig,
			sentinel: redisDefaultConfigSentinel,
		},
	}
	_, err := host.Execute("sudo touch " + startFilePath)
	require.NoError(t, err)

	require.EventuallyWithT(t, func(c *assert.CollectT) {
		payloads, err := s.Env().FakeIntake.Client().GetAgentDiscoveryPayloads()
		if !assert.NoError(c, err) {
			return
		}
		if !assert.NotEmpty(c, payloads, "no Agent Discovery payloads on %s", agentDiscoveryEndpoint) {
			return
		}

		for _, tt := range expectedRedisConfigs {
			redisPayloads := findConfigFilePayloads(payloads, redisIntegrationName, tt.path)
			assert.GreaterOrEqual(c, len(redisPayloads), 2, "fewer than two redis config payloads for %q found in %+v", tt.path, payloads)
			if len(redisPayloads) < 2 {
				continue
			}

			for _, redisPayload := range redisPayloads {
				assertConfigFilePayload(c, redisPayload, configFilePayloadExpectation{
					integrationName: redisIntegrationName,
					configPath:      tt.path,
					payloadFormat:   agentdiscovery.AgentDiscoveryConfigFilePayloadFormat_PAYLOAD_FORMAT_REDIS_CONF,
				})
				assert.Equal(c, tt.content, string(redisPayload.config.Content))
				assert.Contains(c, string(redisPayload.config.Content), tt.sentinel)
			}
		}
	}, 3*time.Minute, 10*time.Second, "timed out waiting for config files discovery payload")
}

type configFilePayload struct {
	payload *aggregator.AgentDiscoveryPayload
	config  aggregator.AgentDiscoveryConfigFile
}

func assertAgentDiscoveryPayload(c *assert.CollectT, payload *aggregator.AgentDiscoveryPayload, integrationName string) {
	c.Helper()

	assert.Equal(c, integrationName, payload.Integration)
	assert.Equal(c, dockerRuntime, payload.Runtime)
	assert.NotEmpty(c, payload.HostID)
	assert.NotEmpty(c, payload.RuntimeID)
	assert.False(c, payload.IngestionTimestamp.IsZero())
}

func assertConfigFilePayload(c *assert.CollectT, actual configFilePayload, expected configFilePayloadExpectation) {
	c.Helper()

	assertAgentDiscoveryPayload(c, actual.payload, expected.integrationName)

	assert.Equal(c, expected.configPath, actual.config.Path)
	assert.Equal(c, expected.payloadFormat, actual.config.PayloadFormat)
	assert.False(c, actual.config.Truncated)
}

func findConfigFilePayloads(payloads []*aggregator.AgentDiscoveryPayload, integrationName string, configPath string) []configFilePayload {
	var configFilePayloads []configFilePayload
	for _, payload := range payloads {
		if payload.Integration != integrationName {
			continue
		}
		for _, config := range payload.ConfigFiles {
			if config.Path == configPath {
				configFilePayloads = append(configFilePayloads, configFilePayload{
					payload: payload,
					config:  config,
				})
			}
		}
	}
	return configFilePayloads
}

func findEnvPayloads(payloads []*aggregator.AgentDiscoveryPayload, integrationName string) []*aggregator.AgentDiscoveryPayload {
	var envPayloads []*aggregator.AgentDiscoveryPayload
	for _, payload := range payloads {
		if payload.Integration == integrationName && payload.Runtime == dockerRuntime && len(payload.EnvVars) > 0 {
			envPayloads = append(envPayloads, payload)
		}
	}
	return envPayloads
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
