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

func TestKafkaCollectorMatchesAndReadsRelativeProcessConfig(t *testing.T) {
	eventArgs := []string{"kafka-server-start.sh", "config/server.properties"}
	reader := &kafkaCollectorTestReader{
		commandline: configfilesdiscoveryimpl.TargetCommandline{
			Args: []string{"/bin/bash", "/mnt/kafka-wrapper/start-kafka.sh"},
		},
		commandlines: []configfilesdiscoveryimpl.TargetCommandline{{
			Args:       eventArgs,
			WorkingDir: "/opt/kafka",
		}},
		file: configfilesdiscoveryimpl.ConfigFile{Path: "/opt/kafka/config/server.properties"},
	}

	configArg, matched := kafkaGetConfigArgFromCommandline(eventArgs)
	require.True(t, matched)
	_, resolved := resolveConfigPath(configArg, "")
	assert.False(t, resolved)
	require.True(t, kafkaConfigCollector{}.MatchesCommandline(eventArgs))

	collected, err := kafkaConfigCollector{}.Collect(context.Background(), reader)

	require.NoError(t, err)
	assert.Equal(t, []string{"/opt/kafka/config/server.properties"}, reader.readFileCalls)
	require.Len(t, collected.ConfigFiles, 1)
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
	}
	collector := NewKafka()

	collected, err := collector.Collect(context.Background(), reader)

	require.NoError(t, err)
	assert.Equal(t, []string{"/etc/kafka/server.properties"}, reader.readFileCalls)
	require.Len(t, collected.ConfigFiles, 1)
	assert.Equal(t, configfilesdiscoveryimpl.ConfigFile{
		Path:          "/etc/kafka/server.properties",
		Content:       []byte("broker.id=1\n"),
		Truncated:     true,
		PayloadFormat: kafkaConfigPayloadFormat,
	}, collected.ConfigFiles[0])
	assert.Empty(t, collected.EnvVars)
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
	assert.Empty(t, reader.readFileCalls)
	assert.Empty(t, collected.ConfigFiles)
	assert.Empty(t, collected.EnvVars)
}

func TestKafkaCollectorReadsUniqueConfigAcrossProcesses(t *testing.T) {
	reader := &kafkaCollectorTestReader{
		commandline: configfilesdiscoveryimpl.TargetCommandline{
			Args: []string{"/bin/bash", "/mnt/kafka-wrapper/start-kafka.sh"},
		},
		commandlines: []configfilesdiscoveryimpl.TargetCommandline{
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
		commandline: configfilesdiscoveryimpl.TargetCommandline{
			Args: []string{"/bin/bash", "/mnt/kafka-wrapper/start-kafka.sh"},
		},
		commandlines: []configfilesdiscoveryimpl.TargetCommandline{
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
		commandline: configfilesdiscoveryimpl.TargetCommandline{
			Args: []string{"/bin/bash", "/mnt/kafka-wrapper/start-kafka.sh"},
		},
		commandlines: []configfilesdiscoveryimpl.TargetCommandline{
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
		commandline: configfilesdiscoveryimpl.TargetCommandline{
			Args: []string{"kafka-server-start.sh", "/etc/kafka/runtime.properties"},
		},
		commandlines: []configfilesdiscoveryimpl.TargetCommandline{
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

func TestKafkaCollectorReturnsCommandlineErrors(t *testing.T) {
	expectedErr := errors.New("command line unavailable")
	reader := &kafkaCollectorTestReader{commandlineErr: expectedErr}
	collector := NewKafka()

	collected, err := collector.Collect(context.Background(), reader)

	require.ErrorIs(t, err, expectedErr)
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
	commandline             configfilesdiscoveryimpl.TargetCommandline
	commandlines            []configfilesdiscoveryimpl.TargetCommandline
	commandlineErr          error
	processCommandlineCalls int
	readFileCalls           []string
	file                    configfilesdiscoveryimpl.ConfigFile
	readFileErr             error
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

func (r *kafkaCollectorTestReader) ReadEnvVars(context.Context, []string) (map[string]string, error) {
	return nil, errors.New("not implemented")
}

func (r *kafkaCollectorTestReader) ReadRuntimeCommandline(context.Context) (configfilesdiscoveryimpl.TargetCommandline, error) {
	if r.commandlineErr != nil {
		return configfilesdiscoveryimpl.TargetCommandline{}, r.commandlineErr
	}
	return r.commandline, nil
}

func (r *kafkaCollectorTestReader) ReadLiveProcessCommandlines(context.Context) []configfilesdiscoveryimpl.TargetCommandline {
	r.processCommandlineCalls++
	return r.commandlines
}
