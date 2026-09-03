// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package collectors

import (
	"context"
	"errors"
	"slices"
	"testing"

	configfilesdiscoveryimpl "github.com/DataDog/datadog-agent/comp/core/configfilesdiscovery/impl"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestKafkaGetConfigPath(t *testing.T) {
	tests := []struct {
		name        string
		commandline configfilesdiscoveryimpl.TargetCommandline
		wantPath    string
		wantOK      bool
	}{
		{
			name: "server start script with absolute path",
			commandline: configfilesdiscoveryimpl.TargetCommandline{
				Args: []string{"kafka-server-start.sh", "/opt/kafka/config/server.properties"},
			},
			wantPath: "/opt/kafka/config/server.properties",
			wantOK:   true,
		},
		{
			name: "server start script with daemon flag",
			commandline: configfilesdiscoveryimpl.TargetCommandline{
				Args: []string{"kafka-server-start.sh", "-daemon", "/opt/kafka/config/kraft/server.properties"},
			},
			wantPath: "/opt/kafka/config/kraft/server.properties",
			wantOK:   true,
		},
		{
			name: "server start script with overrides",
			commandline: configfilesdiscoveryimpl.TargetCommandline{
				Args: []string{
					"/opt/kafka/bin/kafka-server-start.sh",
					"/opt/kafka/config/server.properties",
					"--override",
					"listeners=PLAINTEXT://:9092",
					"--override=log.dirs=/var/lib/kafka/data",
				},
			},
			wantPath: "/opt/kafka/config/server.properties",
			wantOK:   true,
		},
		{
			name: "actual JVM class",
			commandline: configfilesdiscoveryimpl.TargetCommandline{
				Args: []string{
					"java",
					"-Xmx1G",
					"-cp",
					"/opt/kafka/libs/*",
					"kafka.Kafka",
					"/etc/kafka/server.properties",
				},
			},
			wantPath: "/etc/kafka/server.properties",
			wantOK:   true,
		},
		{
			name: "relative path",
			commandline: configfilesdiscoveryimpl.TargetCommandline{
				Args:       []string{"kafka-server-start.sh", "config/server.properties"},
				WorkingDir: "/opt/kafka",
			},
			wantPath: "/opt/kafka/config/server.properties",
			wantOK:   true,
		},
		{
			name: "shell form",
			commandline: configfilesdiscoveryimpl.TargetCommandline{
				Args: []string{"/bin/sh", "-c", "kafka-server-start.sh /etc/kafka/server.properties --override broker.id=1"},
			},
			wantPath: "/etc/kafka/server.properties",
			wantOK:   true,
		},
		{
			name: "run class wrapper",
			commandline: configfilesdiscoveryimpl.TargetCommandline{
				Args: []string{
					"kafka-run-class.sh",
					"kafka.Kafka",
					"/etc/kafka/kraft/server.properties",
				},
			},
			wantPath: "/etc/kafka/kraft/server.properties",
			wantOK:   true,
		},
		{
			name: "direct config path without kafka launcher",
			commandline: configfilesdiscoveryimpl.TargetCommandline{
				Args: []string{"/etc/kafka/server.properties"},
			},
		},
		{
			name: "default docker entrypoint without explicit properties path",
			commandline: configfilesdiscoveryimpl.TargetCommandline{
				Args: []string{"/etc/kafka/docker/run"},
			},
		},
		{
			name: "overrides only",
			commandline: configfilesdiscoveryimpl.TargetCommandline{
				Args: []string{"kafka-server-start.sh", "--override", "listeners=PLAINTEXT://:9092"},
			},
		},
		{
			name: "unknown flag before config",
			commandline: configfilesdiscoveryimpl.TargetCommandline{
				Args: []string{"kafka-server-start.sh", "--unknown", "/etc/kafka/server.properties"},
			},
		},
		{
			name: "non kafka command",
			commandline: configfilesdiscoveryimpl.TargetCommandline{
				Args: []string{"redis-server", "/etc/redis/redis.conf"},
			},
		},
		{
			name: "relative path without absolute working dir",
			commandline: configfilesdiscoveryimpl.TargetCommandline{
				Args: []string{"kafka-server-start.sh", "config/server.properties"},
			},
		},
		{
			name: "path with NUL byte",
			commandline: configfilesdiscoveryimpl.TargetCommandline{
				Args: []string{"kafka-server-start.sh", "/etc/kafka/server.properties\x00.extra"},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			configArg, gotOK := kafkaGetConfigArgFromCommandline(tt.commandline.Args)
			var gotPath string
			if gotOK {
				gotPath, gotOK = resolveConfigPath(configArg, tt.commandline.WorkingDir)
			}

			assert.Equal(t, tt.wantOK, gotOK)
			assert.Equal(t, tt.wantPath, gotPath)
		})
	}
}

func TestKafkaCollectorResolvesAndReadsRelativeProcessConfig(t *testing.T) {
	eventArgs := []string{"kafka-server-start.sh", "config/server.properties"}
	eventCommandline := configfilesdiscoveryimpl.TargetCommandline{
		Args:       eventArgs,
		WorkingDir: "/opt/kafka",
	}
	reader := &kafkaCollectorTestReader{
		runtimeCommandline: configfilesdiscoveryimpl.TargetCommandline{
			Args: []string{"/bin/bash", "/mnt/kafka-wrapper/start-kafka.sh"},
		},
		liveProcessCommandlines: []configfilesdiscoveryimpl.TargetCommandline{eventCommandline},
		file:                    configfilesdiscoveryimpl.ConfigFile{Path: "/opt/kafka/config/server.properties"},
	}

	collector := kafkaConfigCollector{}
	assert.False(t, collector.CanCollectFromProcess(configfilesdiscoveryimpl.TargetCommandline{Args: eventArgs}))
	assert.True(t, collector.CanCollectFromProcess(eventCommandline))
	assert.True(t, collector.CanCollectFromProcess(configfilesdiscoveryimpl.TargetCommandline{
		Args: []string{"kafka-server-start.sh", "/etc/kafka/server.properties"},
	}))

	collected, err := collector.Collect(context.Background(), reader)

	require.NoError(t, err)
	assert.Equal(t, []string{"/opt/kafka/config/server.properties"}, reader.readFileCalls)
	require.Len(t, collected.ConfigFiles, 1)
}

func TestIncludeKafkaEnvVar(t *testing.T) {
	tests := []struct {
		envName string
		want    bool
	}{
		{envName: "KAFKA_CONTROLLER_LISTENER_NAMES", want: true},
		{envName: "KAFKA_CFG_PROCESS_ROLES", want: true},
		{envName: "CONFLUENT_BALANCER_ENABLE", want: true},
		{envName: "CLUSTER_ID", want: true},
		{envName: "UNREQUESTED"},
		{envName: "KAFKA_node_id"},
		{envName: "KAFKA_OPTS"},
		{envName: "KAFKA_JMX_OPTS"},
		{envName: "KAFKA_JVM_PERFORMANCE_OPTS"},
		{envName: "KAFKA_CFG_JVM_OPTS"},
		{envName: "CONFLUENT_JVM_OPTS"},
		{envName: "KAFKA_EXTRA_ARGS"},
		{envName: "KAFKA_EXTRA_FLAGS"},
		{envName: "KAFKA_CFG_EXTRA_ARGS"},
		{envName: "KAFKA_SCHEMA_REGISTRY_BASIC_AUTH_USER_INFO"},
		{envName: "KAFKA_CONFLUENT_METRICS_REPORTER_BASIC_AUTH_USER_INFO"},
		{envName: "KAFKA_SUPER_USERS"},
		{envName: "KAFKA_CFG_SUPER_USERS"},
		{envName: "KAFKA_SSL_KEYSTORE_LOCATION", want: true},
	}

	for _, tt := range tests {
		t.Run(tt.envName, func(t *testing.T) {
			assert.Equal(t, tt.want, includeKafkaEnvVar(tt.envName))
		})
	}
}

func TestKafkaCollectorReadsDetectedConfig(t *testing.T) {
	reader := &kafkaCollectorTestReader{
		runtimeCommandline: configfilesdiscoveryimpl.TargetCommandline{
			Args: []string{"kafka-server-start.sh", "/etc/kafka/server.properties"},
		},
		file: configfilesdiscoveryimpl.ConfigFile{
			Path:      "/etc/kafka/server.properties",
			Content:   []byte("broker.id=1\n"),
			Truncated: true,
		},
		env: map[string]string{
			"KAFKA_NODE_ID":       "1",
			"KAFKA_PROCESS_ROLES": "broker",
			"UNREQUESTED":         "value",
		},
	}
	collector := NewKafka()

	collected, err := collector.Collect(context.Background(), reader)

	require.NoError(t, err)
	requireKafkaEnvVarPredicate(t, reader)
	assert.Equal(t, []string{"/etc/kafka/server.properties"}, reader.readFileCalls)
	require.Len(t, collected.ConfigFiles, 1)
	assert.Equal(t, configfilesdiscoveryimpl.ConfigFile{
		Path:          "/etc/kafka/server.properties",
		Content:       []byte("broker.id=1\n"),
		Truncated:     true,
		PayloadFormat: kafkaConfigPayloadFormat,
	}, collected.ConfigFiles[0])
	assert.Equal(t, []configfilesdiscoveryimpl.ConfigEnvVar{
		{Name: "KAFKA_NODE_ID", Value: "1"},
		{Name: "KAFKA_PROCESS_ROLES", Value: "broker"},
	}, collected.EnvVars)
}

func TestKafkaCollectorCollectsEnvOnly(t *testing.T) {
	tests := []struct {
		name        string
		commandline []string
		env         map[string]string
		want        []configfilesdiscoveryimpl.ConfigEnvVar
	}{
		{
			name:        "Confluent",
			commandline: []string{"/etc/kafka/docker/run"},
			env: map[string]string{
				"CLUSTER_ID":          "cluster-id",
				"KAFKA_NODE_ID":       "1",
				"KAFKA_PROCESS_ROLES": "broker,controller",
			},
			want: []configfilesdiscoveryimpl.ConfigEnvVar{
				{Name: "CLUSTER_ID", Value: "cluster-id"},
				{Name: "KAFKA_NODE_ID", Value: "1"},
				{Name: "KAFKA_PROCESS_ROLES", Value: "broker,controller"},
			},
		},
		{
			name:        "Bitnami",
			commandline: []string{"/opt/bitnami/scripts/kafka/run.sh"},
			env: map[string]string{
				"KAFKA_CFG_PROCESS_ROLES":                       "broker",
				"KAFKA_CFG_CONTROLLER_QUORUM_BOOTSTRAP_SERVERS": "controller:9093",
				"KAFKA_INITIAL_CONTROLLERS":                     "1@controller:9093:dir",
				"KAFKA_TLS_TYPE":                                "PEM",
			},
			want: []configfilesdiscoveryimpl.ConfigEnvVar{
				{Name: "KAFKA_CFG_CONTROLLER_QUORUM_BOOTSTRAP_SERVERS", Value: "controller:9093"},
				{Name: "KAFKA_CFG_PROCESS_ROLES", Value: "broker"},
				{Name: "KAFKA_INITIAL_CONTROLLERS", Value: "1@controller:9093:dir"},
				{Name: "KAFKA_TLS_TYPE", Value: "PEM"},
			},
		},
		{
			name:        "Confluent Enterprise",
			commandline: []string{"/etc/kafka/docker/run"},
			env: map[string]string{
				"CONFLUENT_BALANCER_ENABLE": "true",
				"CONFLUENT_TIER_S3_BUCKET":  "tiered-storage",
			},
			want: []configfilesdiscoveryimpl.ConfigEnvVar{
				{Name: "CONFLUENT_BALANCER_ENABLE", Value: "true"},
				{Name: "CONFLUENT_TIER_S3_BUCKET", Value: "tiered-storage"},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			reader := &kafkaCollectorTestReader{
				runtimeCommandline: configfilesdiscoveryimpl.TargetCommandline{Args: tt.commandline},
				env:                tt.env,
			}

			collected, err := NewKafka().Collect(context.Background(), reader)

			require.NoError(t, err)
			assert.Equal(t, slices.Concat(kafkaDefaultConfigPathGroups...), reader.readFileCalls)
			assert.Empty(t, collected.ConfigFiles)
			assert.Equal(t, tt.want, collected.EnvVars)
		})
	}
}

func TestKafkaCollectorFiltersSecretEnvVars(t *testing.T) {
	reader := &kafkaCollectorTestReader{
		runtimeCommandline: configfilesdiscoveryimpl.TargetCommandline{
			Args: []string{"/etc/kafka/docker/run"},
		},
		env: map[string]string{
			"CONFLUENT_ADMIN_PWD":                              "secret",
			"KAFKA_CFG_OAUTH_ACCESS_TOKEN":                     "secret",
			"KAFKA_CONFLUENT_LICENSE_TOPIC_REPLICATION_FACTOR": "3",
			"KAFKA_NODE_ID":                                    "1",
			"KAFKA_OPTS":                                       "-Dsome.token=secret",
			"KAFKA_SASL_OAUTHBEARER_TOKEN_ENDPOINT_URL":        "https://identity.example/oauth2/token",
			"KAFKA_SASL_PWD":                                   "secret",
		},
	}

	collected, err := NewKafka().Collect(context.Background(), reader)

	require.NoError(t, err)
	assert.Equal(t, []configfilesdiscoveryimpl.ConfigEnvVar{
		{Name: "KAFKA_CONFLUENT_LICENSE_TOPIC_REPLICATION_FACTOR", Value: "3"},
		{Name: "KAFKA_NODE_ID", Value: "1"},
		{Name: "KAFKA_SASL_OAUTHBEARER_TOKEN_ENDPOINT_URL", Value: "https://identity.example/oauth2/token"},
	}, collected.EnvVars)
}

func TestKafkaCollectorSkipsDefaultsWhenCommandlineHasNoConfigFile(t *testing.T) {
	reader := &kafkaCollectorTestReader{
		runtimeCommandline: configfilesdiscoveryimpl.TargetCommandline{
			Args: []string{"kafka-server-start.sh", "--override", "broker.id=1"},
		},
		file: configfilesdiscoveryimpl.ConfigFile{
			Path:    "/etc/kafka/server.properties",
			Content: []byte("broker.id=1\n"),
		},
	}
	collector := NewKafka()

	collected, err := collector.Collect(context.Background(), reader)

	require.NoError(t, err)
	requireKafkaEnvVarPredicate(t, reader)
	assert.Empty(t, reader.readFileCalls)
	assert.Empty(t, collected.ConfigFiles)
	assert.Empty(t, collected.EnvVars)
}

func TestKafkaCollectorReadsDefaultConfig(t *testing.T) {
	reader := &kafkaCollectorTestReader{
		runtimeCommandline: configfilesdiscoveryimpl.TargetCommandline{
			Args: []string{"/etc/kafka/docker/run"},
		},
		file: configfilesdiscoveryimpl.ConfigFile{
			Path:    "/opt/bitnami/kafka/config/server.properties",
			Content: []byte("broker.id=1\n"),
		},
	}

	collected, err := NewKafka().Collect(context.Background(), reader)

	require.NoError(t, err)
	assert.Equal(t, slices.Concat(kafkaDefaultConfigPathGroups...), reader.readFileCalls)
	require.Len(t, collected.ConfigFiles, 1)
	assert.Equal(t, configfilesdiscoveryimpl.ConfigFile{
		Path:          "/opt/bitnami/kafka/config/server.properties",
		Content:       []byte("broker.id=1\n"),
		PayloadFormat: kafkaConfigPayloadFormat,
	}, collected.ConfigFiles[0])
}

func TestKafkaCollectorSelectsActiveDistributionConfig(t *testing.T) {
	tests := []struct {
		name               string
		runtimeCommandline configfilesdiscoveryimpl.TargetCommandline
		activeConfig       configfilesdiscoveryimpl.ConfigFile
		exampleConfigs     map[string]configfilesdiscoveryimpl.ConfigFile
		wantReadFileCalls  []string
	}{
		{
			name: "apache",
			runtimeCommandline: configfilesdiscoveryimpl.TargetCommandline{
				Args: []string{"/etc/kafka/docker/run"},
			},
			activeConfig: configfilesdiscoveryimpl.ConfigFile{
				Path:    "/opt/kafka/config/server.properties",
				Content: []byte("process.roles=broker,controller\n"),
			},
			exampleConfigs: map[string]configfilesdiscoveryimpl.ConfigFile{
				"/opt/kafka/config/kraft/server.properties": {
					Path:    "/opt/kafka/config/kraft/server.properties",
					Content: []byte("process.roles=broker,controller\n"),
				},
			},
		},
		{
			name: "confluent",
			runtimeCommandline: configfilesdiscoveryimpl.TargetCommandline{
				Args: []string{"/bin/bash", "/etc/confluent/docker/run"},
			},
			activeConfig: configfilesdiscoveryimpl.ConfigFile{
				Path:    "/etc/kafka/kafka.properties",
				Content: []byte("node.id=1\n"),
			},
			exampleConfigs: map[string]configfilesdiscoveryimpl.ConfigFile{
				"/etc/kafka/server.properties": {
					Path:    "/etc/kafka/server.properties",
					Content: []byte("broker.id=0\n"),
				},
				"/etc/kafka/kraft/server.properties": {
					Path:    "/etc/kafka/kraft/server.properties",
					Content: []byte("process.roles=broker,controller\n"),
				},
			},
		},
		{
			name: "strimzi",
			runtimeCommandline: configfilesdiscoveryimpl.TargetCommandline{
				Args: []string{"/opt/kafka/kafka_run.sh"},
			},
			activeConfig: configfilesdiscoveryimpl.ConfigFile{
				Path:    "/tmp/strimzi.properties",
				Content: []byte("node.id=1\n"),
			},
			exampleConfigs: map[string]configfilesdiscoveryimpl.ConfigFile{
				"/opt/kafka/config/server.properties": {
					Path:    "/opt/kafka/config/server.properties",
					Content: []byte("broker.id=0\n"),
				},
			},
			wantReadFileCalls: []string{"/tmp/strimzi.properties"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			files := tt.exampleConfigs
			files[tt.activeConfig.Path] = tt.activeConfig
			reader := &kafkaCollectorTestReader{
				runtimeCommandline: tt.runtimeCommandline,
				files:              files,
			}

			collected, err := NewKafka().Collect(context.Background(), reader)

			require.NoError(t, err)
			wantReadFileCalls := tt.wantReadFileCalls
			if wantReadFileCalls == nil {
				wantReadFileCalls = slices.Concat(kafkaDefaultConfigPathGroups...)
			}
			assert.Equal(t, wantReadFileCalls, reader.readFileCalls)
			require.Len(t, collected.ConfigFiles, 1)
			expectedConfig := tt.activeConfig
			expectedConfig.PayloadFormat = kafkaConfigPayloadFormat
			assert.Equal(t, expectedConfig, collected.ConfigFiles[0])
		})
	}
}

func TestKafkaCollectorReadsUniqueConfigAcrossProcesses(t *testing.T) {
	reader := &kafkaCollectorTestReader{
		runtimeCommandline: configfilesdiscoveryimpl.TargetCommandline{
			Args: []string{"/bin/bash", "/mnt/kafka-wrapper/start-kafka.sh"},
		},
		liveProcessCommandlines: []configfilesdiscoveryimpl.TargetCommandline{
			{Args: []string{"kafka-server-start.sh", "/etc/kafka/server.properties"}},
			{Args: []string{"java", "kafka.Kafka", "/etc/kafka/server.properties"}},
		},
		file: configfilesdiscoveryimpl.ConfigFile{Path: "/etc/kafka/server.properties"},
	}

	collected, err := NewKafka().Collect(context.Background(), reader)

	require.NoError(t, err)
	assert.Equal(t, []string{"/etc/kafka/server.properties"}, reader.readFileCalls)
	assert.Equal(t, 1, reader.processCommandlineCalls)
	require.Len(t, collected.ConfigFiles, 1)
}

func TestKafkaCollectorSkipsConflictingProcessConfigPaths(t *testing.T) {
	reader := &kafkaCollectorTestReader{
		runtimeCommandline: configfilesdiscoveryimpl.TargetCommandline{
			Args: []string{"/bin/bash", "/mnt/kafka-wrapper/start-kafka.sh"},
		},
		liveProcessCommandlines: []configfilesdiscoveryimpl.TargetCommandline{
			{Args: []string{"java", "kafka.Kafka", "/etc/kafka/server.properties"}},
			{Args: []string{"java", "kafka.Kafka", "/etc/kafka/other.properties"}},
		},
	}

	collected, err := NewKafka().Collect(context.Background(), reader)

	require.NoError(t, err)
	assert.Empty(t, reader.readFileCalls)
	assert.Empty(t, collected.ConfigFiles)
}

func TestKafkaCollectorSkipsUnresolvedMatchingProcessConfigPath(t *testing.T) {
	reader := &kafkaCollectorTestReader{
		runtimeCommandline: configfilesdiscoveryimpl.TargetCommandline{
			Args: []string{"/bin/bash", "/mnt/kafka-wrapper/start-kafka.sh"},
		},
		liveProcessCommandlines: []configfilesdiscoveryimpl.TargetCommandline{
			{Args: []string{"java", "kafka.Kafka", "/etc/kafka/server.properties"}},
			{Args: []string{"java", "kafka.Kafka", "config/server.properties"}},
		},
	}

	collected, err := NewKafka().Collect(context.Background(), reader)

	require.NoError(t, err)
	assert.Empty(t, reader.readFileCalls)
	assert.Empty(t, collected.ConfigFiles)
}

func TestKafkaCollectorUsesRuntimeConfigBeforeProcessConfig(t *testing.T) {
	reader := &kafkaCollectorTestReader{
		runtimeCommandline: configfilesdiscoveryimpl.TargetCommandline{
			Args: []string{"kafka-server-start.sh", "/etc/kafka/runtime.properties"},
		},
		liveProcessCommandlines: []configfilesdiscoveryimpl.TargetCommandline{
			{Args: []string{"java", "kafka.Kafka", "/etc/kafka/process.properties"}},
		},
		file: configfilesdiscoveryimpl.ConfigFile{Path: "/etc/kafka/runtime.properties"},
	}

	collected, err := NewKafka().Collect(context.Background(), reader)

	require.NoError(t, err)
	assert.Equal(t, []string{"/etc/kafka/runtime.properties"}, reader.readFileCalls)
	assert.Zero(t, reader.processCommandlineCalls)
	require.Len(t, collected.ConfigFiles, 1)
}

func TestKafkaCollectorFallsBackToProcessCommandlineOnRuntimeCommandlineError(t *testing.T) {
	expectedErr := errors.New("command line unavailable")
	reader := &kafkaCollectorTestReader{
		commandlineErr: expectedErr,
		liveProcessCommandlines: []configfilesdiscoveryimpl.TargetCommandline{{
			Args: []string{"java", "kafka.Kafka", "/etc/kafka/server.properties"},
		}},
		file: configfilesdiscoveryimpl.ConfigFile{
			Path: "/etc/kafka/server.properties",
		},
	}

	collected, err := NewKafka().Collect(context.Background(), reader)

	require.NoError(t, err)
	assert.Equal(t, 1, reader.processCommandlineCalls)
	assert.Equal(t, []string{"/etc/kafka/server.properties"}, reader.readFileCalls)
	require.Len(t, collected.ConfigFiles, 1)
}

func TestKafkaCollectorReturnsCommandlineErrors(t *testing.T) {
	expectedErr := errors.New("command line unavailable")
	reader := &kafkaCollectorTestReader{commandlineErr: expectedErr}
	collector := NewKafka()

	collected, err := collector.Collect(context.Background(), reader)

	require.ErrorIs(t, err, expectedErr)
	assert.Equal(t, 1, reader.processCommandlineCalls)
	assert.Empty(t, reader.readEnvVarPredicates)
	assert.Equal(t, configfilesdiscoveryimpl.CollectedConfig{}, collected)
}

func TestKafkaCollectorReadsDetectedConfigWhenEnvVarsFail(t *testing.T) {
	expectedErr := errors.New("env unavailable")
	reader := &kafkaCollectorTestReader{
		runtimeCommandline: configfilesdiscoveryimpl.TargetCommandline{
			Args: []string{"kafka-server-start.sh", "/etc/kafka/server.properties"},
		},
		file: configfilesdiscoveryimpl.ConfigFile{
			Path:    "/etc/kafka/server.properties",
			Content: []byte("broker.id=1\n"),
		},
		readEnvVarsErr: expectedErr,
	}
	collector := NewKafka()

	collected, err := collector.Collect(context.Background(), reader)

	require.NoError(t, err)
	assert.Equal(t, []string{"/etc/kafka/server.properties"}, reader.readFileCalls)
	assert.Equal(t, []configfilesdiscoveryimpl.ConfigFile{
		{
			Path:          "/etc/kafka/server.properties",
			Content:       []byte("broker.id=1\n"),
			PayloadFormat: kafkaConfigPayloadFormat,
		},
	}, collected.ConfigFiles)
	assert.Empty(t, collected.EnvVars)
}

func TestKafkaCollectorReturnsEnvVarErrorsWithoutConfigPath(t *testing.T) {
	expectedErr := errors.New("env unavailable")
	reader := &kafkaCollectorTestReader{
		runtimeCommandline: configfilesdiscoveryimpl.TargetCommandline{
			Args: []string{"/etc/kafka/docker/run"},
		},
		readEnvVarsErr: expectedErr,
	}
	collector := NewKafka()

	collected, err := collector.Collect(context.Background(), reader)

	require.ErrorIs(t, err, expectedErr)
	assert.Equal(t, slices.Concat(kafkaDefaultConfigPathGroups...), reader.readFileCalls)
	assert.Equal(t, configfilesdiscoveryimpl.CollectedConfig{}, collected)
}

func TestKafkaCollectorReturnsReadFileErrors(t *testing.T) {
	expectedErr := errors.New("read failed")
	reader := &kafkaCollectorTestReader{
		runtimeCommandline: configfilesdiscoveryimpl.TargetCommandline{
			Args: []string{"kafka-server-start.sh", "/etc/kafka/server.properties"},
		},
		readFileErr: expectedErr,
	}
	collector := NewKafka()

	collected, err := collector.Collect(context.Background(), reader)

	require.ErrorIs(t, err, expectedErr)
	assert.Equal(t, []string{"/etc/kafka/server.properties"}, reader.readFileCalls)
	assert.Equal(t, configfilesdiscoveryimpl.CollectedConfig{}, collected)
}

type kafkaCollectorTestReader struct {
	runtimeCommandline      configfilesdiscoveryimpl.TargetCommandline
	liveProcessCommandlines []configfilesdiscoveryimpl.TargetCommandline
	commandlineErr          error
	processCommandlineCalls int
	readFileCalls           []string
	file                    configfilesdiscoveryimpl.ConfigFile
	files                   map[string]configfilesdiscoveryimpl.ConfigFile
	readFileErr             error
	env                     map[string]string
	readEnvVarPredicates    []configfilesdiscoveryimpl.ConfigEnvVarPredicate
	readEnvVarsErr          error
}

func (r *kafkaCollectorTestReader) Runtime() configfilesdiscoveryimpl.RuntimeType {
	return configfilesdiscoveryimpl.RuntimeDocker
}

func (r *kafkaCollectorTestReader) Close() {}

func (r *kafkaCollectorTestReader) ReadFile(_ context.Context, path string) (configfilesdiscoveryimpl.ConfigFile, error) {
	r.readFileCalls = append(r.readFileCalls, path)
	if r.readFileErr != nil {
		return configfilesdiscoveryimpl.ConfigFile{}, r.readFileErr
	}
	if r.file.Path == path {
		return r.file, nil
	}
	if file, ok := r.files[path]; ok {
		return file, nil
	}
	return configfilesdiscoveryimpl.ConfigFile{}, errors.New("file not found")
}

func (r *kafkaCollectorTestReader) ReadEnvVars(_ context.Context, predicate configfilesdiscoveryimpl.ConfigEnvVarPredicate) (map[string]string, error) {
	r.readEnvVarPredicates = append(r.readEnvVarPredicates, predicate)
	if r.readEnvVarsErr != nil {
		return nil, r.readEnvVarsErr
	}

	env := make(map[string]string)
	for name, value := range r.env {
		if configfilesdiscoveryimpl.IsSecretEnvVarName(name) {
			continue
		}
		if predicate == nil || !predicate(name) {
			continue
		}
		env[name] = value
	}
	return env, nil
}

func (r *kafkaCollectorTestReader) ReadRuntimeCommandline(context.Context) (configfilesdiscoveryimpl.TargetCommandline, error) {
	if r.commandlineErr != nil {
		return configfilesdiscoveryimpl.TargetCommandline{}, r.commandlineErr
	}
	return r.runtimeCommandline, nil
}

func (r *kafkaCollectorTestReader) ReadLiveProcessCommandlines(context.Context) []configfilesdiscoveryimpl.TargetCommandline {
	r.processCommandlineCalls++
	return r.liveProcessCommandlines
}

func requireKafkaEnvVarPredicate(t *testing.T, reader *kafkaCollectorTestReader) configfilesdiscoveryimpl.ConfigEnvVarPredicate {
	t.Helper()
	require.Len(t, reader.readEnvVarPredicates, 1)
	require.NotNil(t, reader.readEnvVarPredicates[0])
	return reader.readEnvVarPredicates[0]
}
