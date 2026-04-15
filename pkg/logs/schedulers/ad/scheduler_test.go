// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package ad

import (
	"fmt"
	"testing"

	configmock "github.com/DataDog/datadog-agent/pkg/config/mock"
	"github.com/DataDog/datadog-agent/pkg/config/model"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/comp/core/autodiscovery/integration"
	"github.com/DataDog/datadog-agent/comp/core/autodiscovery/providers/names"
	"github.com/DataDog/datadog-agent/comp/logs/agent/config"
	"github.com/DataDog/datadog-agent/pkg/logs/schedulers"
	sourcesPkg "github.com/DataDog/datadog-agent/pkg/logs/sources"
)

func setup() (scheduler *Scheduler, spy *schedulers.MockSourceManager) {
	scheduler = New(nil).(*Scheduler)
	spy = &schedulers.MockSourceManager{}
	scheduler.mgr = spy
	return scheduler, spy
}

func TestScheduleConfigCreatesNewSource(t *testing.T) {
	scheduler, spy := setup()
	configSource := integration.Config{
		LogsConfig:    []byte(`[{"service":"foo","source":"bar"}]`),
		ADIdentifiers: []string{"docker://a1887023ed72a2b0d083ef465e8edfe4932a25731d4bda2f39f288f70af3405b"},
		Provider:      names.Kubernetes,
		TaggerEntity:  "container_id://a1887023ed72a2b0d083ef465e8edfe4932a25731d4bda2f39f288f70af3405b",
		ServiceID:     "docker://a1887023ed72a2b0d083ef465e8edfe4932a25731d4bda2f39f288f70af3405b",
		ClusterCheck:  false,
	}

	scheduler.Schedule([]integration.Config{configSource})

	require.Equal(t, 1, len(spy.Events))
	require.True(t, spy.Events[0].Add)
	logSource := spy.Events[0].Source
	assert.Equal(t, config.DockerType, logSource.Name)
	// We use the docker socket, not sourceType here
	assert.Equal(t, sourcesPkg.SourceType(""), logSource.GetSourceType())
	assert.Equal(t, "foo", logSource.Config.Service)
	assert.Equal(t, "bar", logSource.Config.Source)
	assert.Equal(t, config.DockerType, logSource.Config.Type)
	assert.Equal(t, "a1887023ed72a2b0d083ef465e8edfe4932a25731d4bda2f39f288f70af3405b", logSource.Config.Identifier)
}

func TestScheduleTCPConfig(t *testing.T) {
	scheduler, spy := setup()
	configSource := integration.Config{
		LogsConfig:    []byte(`[{"service":"foo","source":"bar", "type":"tcp"}]`),
		ADIdentifiers: []string{"docker://a1887023ed72a2b0d083ef465e8edfe4932a25731d4bda2f39f288f70af3405b"},
		Provider:      names.Kubernetes,
		TaggerEntity:  "container_id://a1887023ed72a2b0d083ef465e8edfe4932a25731d4bda2f39f288f70af3405b",
		ServiceID:     "docker://a1887023ed72a2b0d083ef465e8edfe4932a25731d4bda2f39f288f70af3405b",
		ClusterCheck:  false,
	}

	scheduler.Schedule([]integration.Config{configSource})

	require.Equal(t, 1, len(spy.Events))
	require.True(t, spy.Events[0].Add)
	logSource := spy.Events[0].Source
	assert.Equal(t, config.DockerType, logSource.Name)
	assert.Equal(t, sourcesPkg.SourceType(""), logSource.GetSourceType())
	assert.Equal(t, "foo", logSource.Config.Service)
	assert.Equal(t, "bar", logSource.Config.Source)
	assert.Equal(t, "tcp", logSource.Config.Type)
	assert.Equal(t, "a1887023ed72a2b0d083ef465e8edfe4932a25731d4bda2f39f288f70af3405b", logSource.Config.Identifier)
}

func TestScheduleUDPConfig(t *testing.T) {
	scheduler, spy := setup()
	configSource := integration.Config{
		LogsConfig:    []byte(`[{"service":"foo","source":"bar", "type":"udp"}]`),
		ADIdentifiers: []string{"docker://a1887023ed72a2b0d083ef465e8edfe4932a25731d4bda2f39f288f70af3405b"},
		Provider:      names.Kubernetes,
		TaggerEntity:  "container_id://a1887023ed72a2b0d083ef465e8edfe4932a25731d4bda2f39f288f70af3405b",
		ServiceID:     "docker://a1887023ed72a2b0d083ef465e8edfe4932a25731d4bda2f39f288f70af3405b",
		ClusterCheck:  false,
	}

	scheduler.Schedule([]integration.Config{configSource})

	require.Equal(t, 1, len(spy.Events))
	require.True(t, spy.Events[0].Add)
	logSource := spy.Events[0].Source
	assert.Equal(t, config.DockerType, logSource.Name)
	assert.Equal(t, sourcesPkg.SourceType(""), logSource.GetSourceType())
	assert.Equal(t, "foo", logSource.Config.Service)
	assert.Equal(t, "bar", logSource.Config.Source)
	assert.Equal(t, "udp", logSource.Config.Type)
	assert.Equal(t, "a1887023ed72a2b0d083ef465e8edfe4932a25731d4bda2f39f288f70af3405b", logSource.Config.Identifier)
}

func TestScheduleIntegrationConfig(t *testing.T) {
	scheduler, spy := setup()
	configSource := integration.Config{
		LogsConfig:    []byte(`[{"service":"foo","source":"bar", "type":"integration"}]`),
		ADIdentifiers: []string{"docker://a1887023ed72a2b0d083ef465e8edfe4932a25731d4bda2f39f288f70af3405b"},
		Provider:      names.Kubernetes,
		TaggerEntity:  "container_id://a1887023ed72a2b0d083ef465e8edfe4932a25731d4bda2f39f288f70af3405b",
		ServiceID:     "docker://a1887023ed72a2b0d083ef465e8edfe4932a25731d4bda2f39f288f70af3405b",
		ClusterCheck:  false,
	}

	scheduler.Schedule([]integration.Config{configSource})

	require.Equal(t, 1, len(spy.Events))
	require.True(t, spy.Events[0].Add)
	logSource := spy.Events[0].Source
	assert.Equal(t, config.DockerType, logSource.Name)
	assert.Equal(t, sourcesPkg.SourceType(""), logSource.GetSourceType())
	assert.Equal(t, "foo", logSource.Config.Service)
	assert.Equal(t, "bar", logSource.Config.Source)
	assert.Equal(t, "integration", logSource.Config.Type)
	assert.Equal(t, "a1887023ed72a2b0d083ef465e8edfe4932a25731d4bda2f39f288f70af3405b", logSource.Config.Identifier)
}

func TestScheduleConfigCreatesNewSourceServiceFallback(t *testing.T) {
	scheduler, spy := setup()
	configSource := integration.Config{
		InitConfig:    []byte(`{"service":"foo"}`),
		LogsConfig:    []byte(`[{"source":"bar"}]`),
		ADIdentifiers: []string{"docker://a1887023ed72a2b0d083ef465e8edfe4932a25731d4bda2f39f288f70af3405b"},
		Provider:      names.Kubernetes,
		TaggerEntity:  "container_id://a1887023ed72a2b0d083ef465e8edfe4932a25731d4bda2f39f288f70af3405b",
		ServiceID:     "docker://a1887023ed72a2b0d083ef465e8edfe4932a25731d4bda2f39f288f70af3405b",
		ClusterCheck:  false,
	}

	scheduler.Schedule([]integration.Config{configSource})

	require.Equal(t, 1, len(spy.Events))
	require.True(t, spy.Events[0].Add)
	logSource := spy.Events[0].Source
	assert.Equal(t, config.DockerType, logSource.Name)
	// We use the docker socket, not sourceType here
	assert.Equal(t, sourcesPkg.SourceType(""), logSource.GetSourceType())
	assert.Equal(t, "foo", logSource.Config.Service)
	assert.Equal(t, "bar", logSource.Config.Source)
	assert.Equal(t, config.DockerType, logSource.Config.Type)
	assert.Equal(t, "a1887023ed72a2b0d083ef465e8edfe4932a25731d4bda2f39f288f70af3405b", logSource.Config.Identifier)
}

func TestScheduleConfigCreatesNewSourceServiceOverride(t *testing.T) {
	scheduler, spy := setup()
	configSource := integration.Config{
		InitConfig:    []byte(`{"service":"foo"}`),
		LogsConfig:    []byte(`[{"source":"bar","service":"baz"}]`),
		ADIdentifiers: []string{"docker://a1887023ed72a2b0d083ef465e8edfe4932a25731d4bda2f39f288f70af3405b"},
		Provider:      names.Kubernetes,
		TaggerEntity:  "container_id://a1887023ed72a2b0d083ef465e8edfe4932a25731d4bda2f39f288f70af3405b",
		ServiceID:     "docker://a1887023ed72a2b0d083ef465e8edfe4932a25731d4bda2f39f288f70af3405b",
		ClusterCheck:  false,
	}

	scheduler.Schedule([]integration.Config{configSource})

	require.Equal(t, 1, len(spy.Events))
	require.True(t, spy.Events[0].Add)
	logSource := spy.Events[0].Source
	assert.Equal(t, config.DockerType, logSource.Name)
	// We use the docker socket, not sourceType here
	assert.Equal(t, sourcesPkg.SourceType(""), logSource.GetSourceType())
	assert.Equal(t, "baz", logSource.Config.Service)
	assert.Equal(t, "bar", logSource.Config.Source)
	assert.Equal(t, config.DockerType, logSource.Config.Type)
	assert.Equal(t, "a1887023ed72a2b0d083ef465e8edfe4932a25731d4bda2f39f288f70af3405b", logSource.Config.Identifier)
}

func TestUnscheduleConfigRemovesSource(t *testing.T) {
	scheduler, spy := setup()
	configSource := integration.Config{
		LogsConfig:    []byte(`[{"service":"foo","source":"bar"}]`),
		ADIdentifiers: []string{"docker://a1887023ed72a2b0d083ef465e8edfe4932a25731d4bda2f39f288f70af3405b"},
		Provider:      names.Kubernetes,
		TaggerEntity:  "container_id://a1887023ed72a2b0d083ef465e8edfe4932a25731d4bda2f39f288f70af3405b",
		ServiceID:     "docker://a1887023ed72a2b0d083ef465e8edfe4932a25731d4bda2f39f288f70af3405b",
		ClusterCheck:  false,
	}

	// We need to have a source to remove
	sources, _ := CreateSources(configSource)
	spy.Sources = sources

	scheduler.Unschedule([]integration.Config{configSource})

	require.Equal(t, 1, len(spy.Events))
	require.False(t, spy.Events[0].Add)
	logSource := spy.Events[0].Source
	assert.Equal(t, config.DockerType, logSource.Name)
	// We use the docker socket, not sourceType here
	assert.Equal(t, sourcesPkg.SourceType(""), logSource.GetSourceType())
	assert.Equal(t, "foo", logSource.Config.Service)
	assert.Equal(t, "bar", logSource.Config.Source)
	assert.Equal(t, config.DockerType, logSource.Config.Type)
	assert.Equal(t, "a1887023ed72a2b0d083ef465e8edfe4932a25731d4bda2f39f288f70af3405b", logSource.Config.Identifier)
}

func TestIgnoreConfigIfLogsExcluded(t *testing.T) {
	scheduler, spy := setup()
	configService := integration.Config{
		LogsConfig:   []byte(""),
		TaggerEntity: "container_id://a1887023ed72a2b0d083ef465e8edfe4932a25731d4bda2f39f288f70af3405b",
		ServiceID:    "docker://a1887023ed72a2b0d083ef465e8edfe4932a25731d4bda2f39f288f70af3405b",
		ClusterCheck: false,
		LogsExcluded: true,
	}

	scheduler.Schedule([]integration.Config{configService})
	scheduler.Unschedule([]integration.Config{configService})
	require.Equal(t, 0, len(spy.Events)) // no events
}

func TestProcessLogPriorityOverManualConfig(t *testing.T) {
	scheduler, spy := setup()

	// First, add a manually configured log file using YAML format
	manualConfig := integration.Config{
		LogsConfig: []byte(`logs:
  - type: file
    path: /var/log/app.log
    service: manual-service`),
		Provider: names.File,
		Name:     "manual-config",
	}

	// Add the manual config first
	scheduler.Schedule([]integration.Config{manualConfig})
	require.Equal(t, 1, len(spy.Events))
	require.True(t, spy.Events[0].Add)

	// Add the source to the mock's Sources slice so GetSources() returns it
	spy.Sources = append(spy.Sources, spy.Events[0].Source)

	// Now try to add a process_log config for the same file
	processLogConfig := integration.Config{
		LogsConfig: []byte(`[{"type":"file","path":"/var/log/app.log","service":"process-service"}]`),
		Provider:   names.ProcessLog,
		Name:       "process-config",
		ServiceID:  names.ProcessLog + ":///var/log/app.log",
	}

	// Clear events for the next test
	spy.Events = nil

	// Schedule the process_log config
	scheduler.Schedule([]integration.Config{processLogConfig})

	// The process_log config should be filtered out due to existing manual config
	require.Equal(t, 0, len(spy.Events), "process_log config should be filtered out when manual config exists")
}

func TestProcessLogConfigAllowedWhenNoConflict(t *testing.T) {
	scheduler, spy := setup()

	// Add a process_log config for a file that doesn't have a manual config
	processLogConfig := integration.Config{
		LogsConfig: []byte(`[{"type":"file","path":"/var/log/process.log","service":"process-service"}]`),
		Provider:   names.ProcessLog,
		Name:       "process-config",
		ServiceID:  names.ProcessLog + ":///var/log/process.log",
	}

	// Schedule the process_log config
	scheduler.Schedule([]integration.Config{processLogConfig})

	// The process_log config should be added since there's no conflict
	require.Equal(t, 1, len(spy.Events))
	require.True(t, spy.Events[0].Add)
	logSource := spy.Events[0].Source
	assert.Equal(t, "process-config", logSource.Name)
	assert.Equal(t, "process-service", logSource.Config.Service)
	assert.Equal(t, "/var/log/process.log", logSource.Config.Path)
}

func TestNonFileTypeProcessLogConfigAllowed(t *testing.T) {
	scheduler, spy := setup()

	// Add a process_log config for a non-file type (should not be filtered)
	processLogConfig := integration.Config{
		LogsConfig: []byte(`[{"type":"tcp","service":"process-service"}]`),
		Provider:   names.ProcessLog,
		Name:       "process-config",
		ServiceID:  names.ProcessLog + "://test-service",
	}

	// Schedule the process_log config
	scheduler.Schedule([]integration.Config{processLogConfig})

	// The process_log config should be added since it's not a file type
	require.Equal(t, 1, len(spy.Events))
	require.True(t, spy.Events[0].Add)
	logSource := spy.Events[0].Source
	assert.Equal(t, "process-config", logSource.Name)
	assert.Equal(t, "process-service", logSource.Config.Service)
	assert.Equal(t, "tcp", logSource.Config.Type)
}

func TestIgnoreRemoteConfigIfDisabled(t *testing.T) {
	mockConfig := configmock.New(t)
	for _, rcLogCfgSchedEnabled := range []bool{true, false} {
		testName := fmt.Sprintf("allow_log_config_scheduling=%t", rcLogCfgSchedEnabled)
		t.Run(testName, func(t *testing.T) {
			scheduler, spy := setup()
			configSource := integration.Config{
				LogsConfig:    []byte(`[{"service":"foo","source":"bar"}]`),
				ADIdentifiers: []string{"docker://a1887023ed72a2b0d083ef465e8edfe4932a25731d4bda2f39f288f70af3405b"},
				Provider:      names.RemoteConfig,
				TaggerEntity:  "container_id://a1887023ed72a2b0d083ef465e8edfe4932a25731d4bda2f39f288f70af3405b",
				ServiceID:     "docker://a1887023ed72a2b0d083ef465e8edfe4932a25731d4bda2f39f288f70af3405b",
				ClusterCheck:  false,
			}
			configmock.New(t)
			mockConfig.Set("remote_configuration.agent_integrations.allow_log_config_scheduling", rcLogCfgSchedEnabled, model.SourceFile)
			scheduler.Schedule([]integration.Config{configSource})
			if rcLogCfgSchedEnabled {
				require.Equal(t, 1, len(spy.Events))
				require.True(t, spy.Events[0].Add)
				logSource := spy.Events[0].Source
				assert.Equal(t, config.DockerType, logSource.Name)
				// We use the docker socket, not sourceType here
				assert.Equal(t, sourcesPkg.SourceType(""), logSource.GetSourceType())
				assert.Equal(t, "foo", logSource.Config.Service)
				assert.Equal(t, "bar", logSource.Config.Source)
				assert.Equal(t, config.DockerType, logSource.Config.Type)
				assert.Equal(t, "a1887023ed72a2b0d083ef465e8edfe4932a25731d4bda2f39f288f70af3405b", logSource.Config.Identifier)
			} else {
				require.Equal(t, 0, len(spy.Events)) // no events
			}
		})
	}
}

func TestScheduleWithNilLogConfigurations(t *testing.T) {
	testCases := []struct {
		name             string
		logsConfig       []byte
		expectedEvents   int
		expectedServices []string
	}{
		{
			name:             "mixed nil and valid configurations",
			logsConfig:       []byte(`[null, {"service":"foo","source":"bar"}, null, {"service":"blah","source":"gah"}]`),
			expectedEvents:   2,
			expectedServices: []string{"foo", "blah"},
		},
		{
			name:             "all nil configurations",
			logsConfig:       []byte(`[null, null, null]`),
			expectedEvents:   0,
			expectedServices: []string{},
		},
		{
			name:             "single nil configuration",
			logsConfig:       []byte(`[null]`),
			expectedEvents:   0,
			expectedServices: []string{},
		},
	}

	for _, c := range testCases {
		t.Run(c.name, func(t *testing.T) {
			scheduler, spy := setup()
			configSource := integration.Config{
				LogsConfig:    c.logsConfig,
				ADIdentifiers: []string{"docker://a1887023ed72a2b0d083ef465e8edfe4932a25731d4bda2f39f288f70af3405b"},
				Provider:      names.Kubernetes,
				TaggerEntity:  "container_id://a1887023ed72a2b0d083ef465e8edfe4932a25731d4bda2f39f288f70af3405b",
				ServiceID:     "docker://a1887023ed72a2b0d083ef465e8edfe4932a25731d4bda2f39f288f70af3405b",
				ClusterCheck:  false,
			}

			scheduler.Schedule([]integration.Config{configSource})

			require.Equal(t, c.expectedEvents, len(spy.Events))
			for i, expectedService := range c.expectedServices {
				require.True(t, spy.Events[i].Add)
				assert.Equal(t, expectedService, spy.Events[i].Source.Config.Service)
			}
		})
	}
}

func TestCreateSourcesWithNilConfigurations(t *testing.T) {
	testCases := []struct {
		name            string
		config          integration.Config
		expectedSources int
		expectedService string
	}{
		{
			name: "JSON mixed configurations",
			config: integration.Config{
				LogsConfig: []byte(`[null, {"service":"foo","source":"bar","type":"file","path":"/test/path"}, null]`),
				Provider:   names.Kubernetes,
				ServiceID:  "docker://a1887023ed72a2b0d083ef465e8edfe4932a25731d4bda2f39f288f70af3405b",
			},
			expectedSources: 1,
			expectedService: "foo",
		},
		{
			name: "JSON all nil configurations",
			config: integration.Config{
				LogsConfig: []byte(`[null, null]`),
				Provider:   names.Kubernetes,
				ServiceID:  "docker://a1887023ed72a2b0d083ef465e8edfe4932a25731d4bda2f39f288f70af3405b",
			},
			expectedSources: 0,
		},
		{
			name: "YAML mixed configurations",
			config: integration.Config{
				LogsConfig: []byte(`logs:
  - null
  - service: "yaml-service"
    source: "yaml-source"
    type: "file"
    path: "/yaml/path"
  - null`),
				Provider:  names.File,
				ServiceID: "docker://a1887023ed72a2b0d083ef465e8edfe4932a25731d4bda2f39f288f70af3405b",
			},
			expectedSources: 1,
			expectedService: "yaml-service",
		},
	}

	for _, c := range testCases {
		t.Run(c.name, func(t *testing.T) {
			sources, err := CreateSources(c.config)

			require.NoError(t, err)
			require.Equal(t, c.expectedSources, len(sources))

			if c.expectedSources > 0 {
				assert.Equal(t, c.expectedService, sources[0].Config.Service)
			}
		})
	}
}
