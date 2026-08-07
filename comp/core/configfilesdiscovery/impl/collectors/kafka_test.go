// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package collectors

import (
	"context"
	"errors"
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
			gotPath, gotOK := kafkaGetConfigPath(tt.commandline)

			assert.Equal(t, tt.wantOK, gotOK)
			assert.Equal(t, tt.wantPath, gotPath)
		})
	}
}

func TestIncludeKafkaEnvVar(t *testing.T) {
	tests := []struct {
		name    string
		envName string
		want    bool
	}{
		{
			name:    "apache kafka config",
			envName: "KAFKA_CONTROLLER_LISTENER_NAMES",
			want:    true,
		},
		{
			name:    "bitnami kafka config",
			envName: "KAFKA_CFG_PROCESS_ROLES",
			want:    true,
		},
		{
			name:    "confluent enterprise config",
			envName: "CONFLUENT_BALANCER_ENABLE",
			want:    true,
		},
		{
			name:    "image helper",
			envName: "CLUSTER_ID",
			want:    true,
		},
		{
			name:    "unrelated",
			envName: "UNREQUESTED",
		},
		{
			name:    "lowercase is not a generated kafka env name",
			envName: "KAFKA_node_id",
		},
		{
			name:    "arbitrary kafka opts",
			envName: "KAFKA_OPTS",
		},
		{
			name:    "jmx opts",
			envName: "KAFKA_JMX_OPTS",
		},
		{
			name:    "jvm performance opts",
			envName: "KAFKA_JVM_PERFORMANCE_OPTS",
		},
		{
			name:    "bitnami jvm opts",
			envName: "KAFKA_CFG_JVM_OPTS",
		},
		{
			name:    "confluent opts",
			envName: "CONFLUENT_JVM_OPTS",
		},
		{
			name:    "extra args",
			envName: "KAFKA_EXTRA_ARGS",
		},
		{
			name:    "extra flags",
			envName: "KAFKA_EXTRA_FLAGS",
		},
		{
			name:    "bitnami extra args",
			envName: "KAFKA_CFG_EXTRA_ARGS",
		},
		{
			name:    "basic auth user info",
			envName: "KAFKA_SCHEMA_REGISTRY_BASIC_AUTH_USER_INFO",
		},
		{
			name:    "confluent metrics reporter basic auth user info",
			envName: "KAFKA_CONFLUENT_METRICS_REPORTER_BASIC_AUTH_USER_INFO",
		},
		{
			name:    "kafka super users",
			envName: "KAFKA_SUPER_USERS",
		},
		{
			name:    "bitnami super users",
			envName: "KAFKA_CFG_SUPER_USERS",
		},
		{
			name:    "safe location reference",
			envName: "KAFKA_SSL_KEYSTORE_LOCATION",
			want:    true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, includeKafkaEnvVar(tt.envName))
		})
	}
}

func TestKafkaCollectorReadsDetectedConfig(t *testing.T) {
	reader := &kafkaCollectorTestReader{
		commandline: configfilesdiscoveryimpl.TargetCommandline{
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

func TestKafkaCollectorCollectsConfluentEnvOnly(t *testing.T) {
	reader := &kafkaCollectorTestReader{
		commandline: configfilesdiscoveryimpl.TargetCommandline{
			Args: []string{"/etc/kafka/docker/run"},
		},
		env: map[string]string{
			"CLUSTER_ID":          "cluster-id",
			"KAFKA_NODE_ID":       "1",
			"KAFKA_PROCESS_ROLES": "broker,controller",
		},
	}
	collector := NewKafka()

	collected, err := collector.Collect(context.Background(), reader)

	require.NoError(t, err)
	assert.Empty(t, reader.readFileCalls)
	assert.Empty(t, collected.ConfigFiles)
	assert.Equal(t, []configfilesdiscoveryimpl.ConfigEnvVar{
		{Name: "CLUSTER_ID", Value: "cluster-id"},
		{Name: "KAFKA_NODE_ID", Value: "1"},
		{Name: "KAFKA_PROCESS_ROLES", Value: "broker,controller"},
	}, collected.EnvVars)
}

func TestKafkaCollectorCollectsBitnamiEnvVars(t *testing.T) {
	reader := &kafkaCollectorTestReader{
		commandline: configfilesdiscoveryimpl.TargetCommandline{
			Args: []string{"/opt/bitnami/scripts/kafka/run.sh"},
		},
		env: map[string]string{
			"KAFKA_CFG_PROCESS_ROLES":                       "broker",
			"KAFKA_CFG_CONTROLLER_QUORUM_BOOTSTRAP_SERVERS": "controller:9093",
			"KAFKA_INITIAL_CONTROLLERS":                     "1@controller:9093:dir",
			"KAFKA_TLS_TYPE":                                "PEM",
		},
	}
	collector := NewKafka()

	collected, err := collector.Collect(context.Background(), reader)

	require.NoError(t, err)
	assert.Empty(t, reader.readFileCalls)
	assert.Empty(t, collected.ConfigFiles)
	assert.Equal(t, []configfilesdiscoveryimpl.ConfigEnvVar{
		{Name: "KAFKA_CFG_CONTROLLER_QUORUM_BOOTSTRAP_SERVERS", Value: "controller:9093"},
		{Name: "KAFKA_CFG_PROCESS_ROLES", Value: "broker"},
		{Name: "KAFKA_INITIAL_CONTROLLERS", Value: "1@controller:9093:dir"},
		{Name: "KAFKA_TLS_TYPE", Value: "PEM"},
	}, collected.EnvVars)
}

func TestKafkaCollectorCollectsConfluentEnterpriseEnvVars(t *testing.T) {
	reader := &kafkaCollectorTestReader{
		commandline: configfilesdiscoveryimpl.TargetCommandline{
			Args: []string{"/etc/kafka/docker/run"},
		},
		env: map[string]string{
			"CONFLUENT_BALANCER_ENABLE": "true",
			"CONFLUENT_TIER_S3_BUCKET":  "tiered-storage",
		},
	}
	collector := NewKafka()

	collected, err := collector.Collect(context.Background(), reader)

	require.NoError(t, err)
	assert.Empty(t, reader.readFileCalls)
	assert.Empty(t, collected.ConfigFiles)
	assert.Equal(t, []configfilesdiscoveryimpl.ConfigEnvVar{
		{Name: "CONFLUENT_BALANCER_ENABLE", Value: "true"},
		{Name: "CONFLUENT_TIER_S3_BUCKET", Value: "tiered-storage"},
	}, collected.EnvVars)
}

func TestKafkaCollectorDoesNotCollectKnownSecretEnvVars(t *testing.T) {
	reader := &kafkaCollectorTestReader{
		commandline: configfilesdiscoveryimpl.TargetCommandline{
			Args: []string{"/etc/kafka/docker/run"},
		},
		env: map[string]string{
			"KAFKA_NODE_ID":                                           "1",
			"KAFKA_SSL_KEYSTORE_PASSWORD":                             "secret",
			"KAFKA_SSL_KEYSTORE_KEY":                                  "secret",
			"KAFKA_SSL_TRUSTSTORE_CERTIFICATES":                       "secret",
			"KAFKA_SCHEMA_REGISTRY_BASIC_AUTH_USER_INFO":              "user:secret",
			"KAFKA_SASL_JAAS_CONFIG":                                  "secret",
			"KAFKA_SASL_OAUTHBEARER_CLIENT_CREDENTIALS_CLIENT_SECRET": "secret",
			"KAFKA_CFG_SSL_KEY_PASSWORD":                              "secret",
			"KAFKA_CLIENT_PASSWORDS":                                  "secret",
			"CONFLUENT_LICENSE":                                       "secret",
			"KAFKA_CONFLUENT_METRICS_REPORTER_BASIC_AUTH_USER_INFO":   "user:secret",
			"CONFLUENT_TIER_S3_SSL_KEY_PASSWORD":                      "secret",
			"KAFKA_SUPER_USERS":                                       "User:admin",
			"KAFKA_CFG_SUPER_USERS":                                   "User:admin",
			"KAFKA_OPTS":                                              "-Dsome.token=secret",
			"KAFKA_HEAP_OPTS":                                         "-Xmx1g",
			"KAFKA_JMX_OPTS":                                          "-Djavax.net.ssl.trustStorePassword=secret",
			"KAFKA_JVM_PERFORMANCE_OPTS":                              "-XX:+UseG1GC",
			"KAFKA_CFG_JVM_OPTS":                                      "-XX:+UseG1GC",
			"CONFLUENT_JVM_OPTS":                                      "-Dsome.token=secret",
			"KAFKA_EXTRA_ARGS":                                        "--override sasl.jaas.config=secret",
			"KAFKA_EXTRA_FLAGS":                                       "--override listener.name.internal.ssl.key.password=secret",
			"KAFKA_CFG_EXTRA_ARGS":                                    "--override ssl.keystore.password=secret",
			"KAFKA_NODE_ID_COMMAND":                                   "echo secret",
			"KAFKA_BROKER_ID_COMMAND":                                 "read-token --password secret",
			"JAVA_TOOL_OPTIONS":                                       "secret",
			"KAFKA_LISTENER_NAME_INTERNAL_PLAIN_SASL_JAAS_CONFIG":     "secret",
		},
	}
	collector := NewKafka()

	collected, err := collector.Collect(context.Background(), reader)

	require.NoError(t, err)
	predicate := requireKafkaEnvVarPredicate(t, reader)
	assert.False(t, predicate("KAFKA_SUPER_USERS"))
	assert.False(t, predicate("KAFKA_CFG_SUPER_USERS"))
	assert.False(t, predicate("KAFKA_OPTS"))
	assert.False(t, predicate("KAFKA_HEAP_OPTS"))
	assert.False(t, predicate("KAFKA_JMX_OPTS"))
	assert.False(t, predicate("KAFKA_JVM_PERFORMANCE_OPTS"))
	assert.False(t, predicate("KAFKA_CFG_JVM_OPTS"))
	assert.False(t, predicate("CONFLUENT_JVM_OPTS"))
	assert.False(t, predicate("KAFKA_EXTRA_ARGS"))
	assert.False(t, predicate("KAFKA_EXTRA_FLAGS"))
	assert.False(t, predicate("KAFKA_CFG_EXTRA_ARGS"))
	assert.False(t, predicate("KAFKA_NODE_ID_COMMAND"))
	assert.False(t, predicate("KAFKA_BROKER_ID_COMMAND"))
	assert.False(t, predicate("JAVA_TOOL_OPTIONS"))
	assert.False(t, predicate("KAFKA_SCHEMA_REGISTRY_BASIC_AUTH_USER_INFO"))
	assert.False(t, predicate("KAFKA_CONFLUENT_METRICS_REPORTER_BASIC_AUTH_USER_INFO"))
	assert.Equal(t, []configfilesdiscoveryimpl.ConfigEnvVar{
		{Name: "KAFKA_NODE_ID", Value: "1"},
	}, collected.EnvVars)
}

func TestKafkaCollectorSkipsWhenNoConfigPathIsDetected(t *testing.T) {
	reader := &kafkaCollectorTestReader{
		commandline: configfilesdiscoveryimpl.TargetCommandline{
			Args: []string{"kafka-server-start.sh", "--override", "broker.id=1"},
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

func TestKafkaCollectorReturnsCommandlineErrors(t *testing.T) {
	expectedErr := errors.New("command line unavailable")
	reader := &kafkaCollectorTestReader{commandlineErr: expectedErr}
	collector := NewKafka()

	collected, err := collector.Collect(context.Background(), reader)

	require.ErrorIs(t, err, expectedErr)
	assert.Empty(t, reader.readEnvVarPredicates)
	assert.Equal(t, configfilesdiscoveryimpl.CollectedConfig{}, collected)
}

func TestKafkaCollectorReadsDetectedConfigWhenEnvVarsFail(t *testing.T) {
	expectedErr := errors.New("env unavailable")
	reader := &kafkaCollectorTestReader{
		commandline: configfilesdiscoveryimpl.TargetCommandline{
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
		commandline: configfilesdiscoveryimpl.TargetCommandline{
			Args: []string{"/etc/kafka/docker/run"},
		},
		readEnvVarsErr: expectedErr,
	}
	collector := NewKafka()

	collected, err := collector.Collect(context.Background(), reader)

	require.ErrorIs(t, err, expectedErr)
	assert.Empty(t, reader.readFileCalls)
	assert.Equal(t, configfilesdiscoveryimpl.CollectedConfig{}, collected)
}

func TestKafkaCollectorReturnsReadFileErrors(t *testing.T) {
	expectedErr := errors.New("read failed")
	reader := &kafkaCollectorTestReader{
		commandline: configfilesdiscoveryimpl.TargetCommandline{
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
	commandline          configfilesdiscoveryimpl.TargetCommandline
	commandlineErr       error
	readFileCalls        []string
	file                 configfilesdiscoveryimpl.ConfigFile
	readFileErr          error
	env                  map[string]string
	readEnvVarPredicates []configfilesdiscoveryimpl.ConfigEnvVarPredicate
	readEnvVarsErr       error
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
	return r.file, nil
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

func (r *kafkaCollectorTestReader) ReadCommandline(context.Context) (configfilesdiscoveryimpl.TargetCommandline, error) {
	if r.commandlineErr != nil {
		return configfilesdiscoveryimpl.TargetCommandline{}, r.commandlineErr
	}
	return r.commandline, nil
}

func requireKafkaEnvVarPredicate(t *testing.T, reader *kafkaCollectorTestReader) configfilesdiscoveryimpl.ConfigEnvVarPredicate {
	t.Helper()
	require.Len(t, reader.readEnvVarPredicates, 1)
	require.NotNil(t, reader.readEnvVarPredicates[0])
	return reader.readEnvVarPredicates[0]
}
