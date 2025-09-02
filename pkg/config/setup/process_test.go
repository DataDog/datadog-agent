// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package setup

import (
	"fmt"
	"runtime"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"

	pkgconfigmodel "github.com/DataDog/datadog-agent/pkg/config/model"
)

// TestProcessDefaults tests to ensure that the config has set process settings correctly
func TestProcessDefaultConfig(t *testing.T) {
	cfg := newTestConf(t)

	for _, tc := range []struct {
		key          string
		defaultValue interface{}
	}{
		{
			key:          "process_config.dd_agent_bin",
			defaultValue: DefaultDDAgentBin,
		},
		{
			key:          "process_config.log_file",
			defaultValue: DefaultProcessAgentLogFile,
		},
		{
			key:          "process_config.grpc_connection_timeout_secs",
			defaultValue: DefaultGRPCConnectionTimeoutSecs,
		},
		{
			key:          "process_config.ignore_zombie_processes",
			defaultValue: false,
		},
		{
			key:          "process_config.process_discovery.enabled",
			defaultValue: true,
		},
		{
			key:          "process_config.process_discovery.interval",
			defaultValue: 4 * time.Hour,
		},
		{
			key:          "process_config.process_collection.enabled",
			defaultValue: false,
		},
		{
			key:          "process_config.container_collection.enabled",
			defaultValue: true,
		},
		{
			key:          "process_config.run_in_core_agent.enabled",
			defaultValue: runtime.GOOS == "linux",
		},
		{
			key:          "process_config.queue_size",
			defaultValue: DefaultProcessQueueSize,
		},
		{
			key:          "process_config.rt_queue_size",
			defaultValue: DefaultProcessRTQueueSize,
		},
		{
			key:          "process_config.process_queue_bytes",
			defaultValue: DefaultProcessQueueBytes,
		},
		{
			key:          "process_config.windows.use_perf_counters",
			defaultValue: false,
		},
		{
			key:          "process_config.additional_endpoints",
			defaultValue: make(map[string][]string),
		},
		{
			key:          "process_config.events_additional_endpoints",
			defaultValue: make(map[string][]string),
		},
		{
			key:          "process_config.internal_profiling.enabled",
			defaultValue: false,
		},
		{
			key:          "process_config.max_per_message",
			defaultValue: DefaultProcessMaxPerMessage,
		},
		{
			key:          "process_config.expvar_port",
			defaultValue: DefaultProcessExpVarPort,
		},
		{
			key:          "process_config.max_message_bytes",
			defaultValue: DefaultProcessMaxMessageBytes,
		},
		{
			key:          "process_config.cmd_port",
			defaultValue: DefaultProcessCmdPort,
		},
		{
			key:          "process_config.event_collection.store.max_items",
			defaultValue: DefaultProcessEventStoreMaxItems,
		},
		{
			key:          "process_config.event_collection.store.max_pending_pushes",
			defaultValue: DefaultProcessEventStoreMaxPendingPushes,
		},
		{
			key:          "process_config.event_collection.store.max_pending_pulls",
			defaultValue: DefaultProcessEventStoreMaxPendingPulls,
		},
		{
			key:          "process_config.event_collection.store.stats_interval",
			defaultValue: DefaultProcessEventStoreStatsInterval,
		},
		{
			key:          "process_config.event_collection.enabled",
			defaultValue: false,
		},
		{
			key:          "process_config.event_collection.interval",
			defaultValue: DefaultProcessEventsCheckInterval,
		},
		{
			key:          "process_config.language_detection.grpc_port",
			defaultValue: DefaultProcessEntityStreamPort,
		},
		{
			key:          "process_config.intervals.connections",
			defaultValue: nil,
		},
		// TODO: process_config.process_collection.use_wlm is a temporary configuration for refactoring purposes
		{
			key:          "process_config.process_collection.use_wlm",
			defaultValue: false,
		},
	} {
		t.Run(tc.key+" default", func(t *testing.T) {
			assert.Equal(t, tc.defaultValue, cfg.Get(tc.key))
		})
	}
}

// TestPrefixes tests that for every corresponding `DD_PROCESS_CONFIG` prefix, there is a `DD_PROCESS_AGENT` prefix as well.
func TestProcessAgentPrefixes(t *testing.T) {
	envVarSlice := newTestConf(t).GetEnvVars()
	envVars := make(map[string]struct{}, len(envVarSlice))
	for _, envVar := range envVarSlice {
		envVars[envVar] = struct{}{}
	}

	for envVar := range envVars {
		if !strings.HasPrefix(envVar, "DD_PROCESS_CONFIG") {
			continue
		}

		processAgentEnvVar := strings.Replace(envVar, "PROCESS_CONFIG", "PROCESS_AGENT", 1)
		t.Run(fmt.Sprintf("%s and %s", envVar, processAgentEnvVar), func(t *testing.T) {
			_, ok := envVars[processAgentEnvVar]
			assert.Truef(t, ok, "%s is defined but not %s", envVar, processAgentEnvVar)
		})
	}
}

// TestPrefixes tests that for every corresponding `DD_PROCESS_AGENT` prefix, there is a `DD_PROCESS_CONFIG` prefix as well.
func TestProcessConfigPrefixes(t *testing.T) {
	envVarSlice := newTestConf(t).GetEnvVars()
	envVars := make(map[string]struct{}, len(envVarSlice))
	for _, envVar := range envVarSlice {
		envVars[envVar] = struct{}{}
	}

	for envVar := range envVars {
		if !strings.HasPrefix(envVar, "DD_PROCESS_AGENT") {
			continue
		}

		processAgentEnvVar := strings.Replace(envVar, "PROCESS_AGENT", "PROCESS_CONFIG", 1)
		t.Run(fmt.Sprintf("%s and %s", envVar, processAgentEnvVar), func(t *testing.T) {
			// Check to see if envVars contains processAgentEnvVar. We can't use assert.Contains,
			// because when it fails the library prints all of envVars which is too noisy
			_, ok := envVars[processAgentEnvVar]
			assert.Truef(t, ok, "%s is defined but not %s", envVar, processAgentEnvVar)
		})
	}
}

func TestEnvVarOverride(t *testing.T) {
	processRunInAgent := true
	if runtime.GOOS != "linux" {
		processRunInAgent = false
	}

	for _, tc := range []struct {
		key, env, value string
		expType         string
		expected        interface{}
	}{
		{
			key:      "log_level",
			env:      "DD_LOG_LEVEL",
			value:    "warn",
			expected: "warn",
		},
		{
			key:      "log_to_console",
			env:      "DD_LOG_TO_CONSOLE",
			value:    "false",
			expected: false,
		},
		{
			key:      "process_config.log_file",
			env:      "DD_PROCESS_CONFIG_LOG_FILE",
			value:    "test",
			expected: "test",
		},
		{
			key:      "process_config.dd_agent_bin",
			env:      "DD_PROCESS_AGENT_DD_AGENT_BIN",
			value:    "test",
			expected: "test",
		},
		{
			key:      "process_config.grpc_connection_timeout_secs",
			env:      "DD_PROCESS_CONFIG_GRPC_CONNECTION_TIMEOUT_SECS",
			value:    "1",
			expected: 1,
		},
		{
			key:      "process_config.process_discovery.enabled",
			env:      "DD_PROCESS_CONFIG_PROCESS_DISCOVERY_ENABLED",
			value:    "true",
			expected: true,
		},
		{
			key:      "process_config.process_discovery.interval",
			env:      "DD_PROCESS_CONFIG_PROCESS_DISCOVERY_INTERVAL",
			value:    "1h",
			expected: time.Hour,
		},
		{
			key:      "process_config.disable_realtime_checks",
			env:      "DD_PROCESS_CONFIG_DISABLE_REALTIME_CHECKS",
			value:    "true",
			expected: true,
		},
		{
			key:      "process_config.enabled",
			env:      "DD_PROCESS_CONFIG_ENABLED",
			value:    "true",
			expected: "true",
		},
		{
			key:      "process_config.process_collection.enabled",
			env:      "DD_PROCESS_CONFIG_PROCESS_COLLECTION_ENABLED",
			value:    "true",
			expected: true,
		},
		{
			key:      "process_config.container_collection.enabled",
			env:      "DD_PROCESS_CONFIG_CONTAINER_COLLECTION_ENABLED",
			value:    "true",
			expected: true,
		},
		{
			key:      "process_config.run_in_core_agent.enabled",
			env:      "DD_PROCESS_CONFIG_RUN_IN_CORE_AGENT_ENABLED",
			value:    "true",
			expected: processRunInAgent,
		},
		{
			key:      "process_config.enabled",
			env:      "DD_PROCESS_CONFIG_ENABLED",
			value:    "false",
			expected: "disabled",
		},
		{
			key:      "process_config.queue_size",
			env:      "DD_PROCESS_CONFIG_QUEUE_SIZE",
			value:    "42",
			expected: 42,
		},
		{
			key:      "process_config.rt_queue_size",
			env:      "DD_PROCESS_CONFIG_RT_QUEUE_SIZE",
			value:    "10",
			expected: 10,
		},
		{
			key:      "process_config.process_queue_bytes",
			env:      "DD_PROCESS_CONFIG_PROCESS_QUEUE_BYTES",
			value:    "20000",
			expected: 20000,
		},
		{
			key:      "process_config.windows.use_perf_counters",
			env:      "DD_PROCESS_CONFIG_WINDOWS_USE_PERF_COUNTERS",
			value:    "true",
			expected: true,
		},
		{
			key:      "process_config.process_dd_url",
			env:      "DD_PROCESS_AGENT_URL",
			value:    "datacat.com",
			expected: "datacat.com",
		},
		{
			key:      "process_config.events_dd_url",
			env:      "DD_PROCESS_CONFIG_EVENTS_DD_URL",
			value:    "datacat.com",
			expected: "datacat.com",
		},
		{
			key:      "process_config.ignore_zombie_processes",
			env:      "DD_PROCESS_CONFIG_IGNORE_ZOMBIE_PROCESSES",
			value:    "true",
			expected: true,
		},
		{
			key:      "process_config.internal_profiling.enabled",
			env:      "DD_PROCESS_CONFIG_INTERNAL_PROFILING_ENABLED",
			value:    "true",
			expected: true,
		},
		{
			key:      "process_config.max_per_message",
			env:      "DD_PROCESS_CONFIG_MAX_PER_MESSAGE",
			value:    "10",
			expected: 10,
		},
		{
			key:      "process_config.max_message_bytes",
			env:      "DD_PROCESS_CONFIG_MAX_MESSAGE_BYTES",
			value:    "100000",
			expected: 100000,
		},
		{
			key:      "process_config.expvar_port",
			env:      "DD_PROCESS_CONFIG_EXPVAR_PORT",
			value:    "1234",
			expected: 1234,
		},
		{
			key:      "process_config.cmd_port",
			env:      "DD_PROCESS_CONFIG_CMD_PORT",
			value:    "1235",
			expected: 1235,
		},
		{
			key:      "process_config.scrub_args",
			env:      "DD_SCRUB_ARGS",
			value:    "false",
			expType:  "boolean", // process_config.scrub_args has no default value so Get returns a string
			expected: false,
		},
		{
			key:      "process_config.scrub_args",
			env:      "DD_SCRUB_ARGS",
			value:    "true",
			expType:  "boolean",
			expected: true,
		},
		{
			key:      "process_config.scrub_args",
			env:      "DD_PROCESS_CONFIG_SCRUB_ARGS",
			value:    "false",
			expType:  "boolean",
			expected: false,
		},
		{
			key:      "process_config.scrub_args",
			env:      "DD_PROCESS_CONFIG_SCRUB_ARGS",
			value:    "true",
			expType:  "boolean",
			expected: true,
		},
		{
			key:      "process_config.strip_proc_arguments",
			env:      "DD_STRIP_PROCESS_ARGS",
			value:    "false",
			expType:  "boolean", // process_config.strip_proc_arguments has no default value so Get returns a string
			expected: false,
		},
		{
			key:      "process_config.strip_proc_arguments",
			env:      "DD_STRIP_PROCESS_ARGS",
			value:    "true",
			expType:  "boolean",
			expected: true,
		},
		{
			key:      "process_config.strip_proc_arguments",
			env:      "DD_PROCESS_CONFIG_STRIP_PROC_ARGUMENTS",
			value:    "false",
			expType:  "boolean",
			expected: false,
		},
		{
			key:      "process_config.strip_proc_arguments",
			env:      "DD_PROCESS_CONFIG_STRIP_PROC_ARGUMENTS",
			value:    "true",
			expType:  "boolean",
			expected: true,
		},
		{
			key:      "process_config.event_collection.store.max_items",
			env:      "DD_PROCESS_CONFIG_EVENT_COLLECTION_STORE_MAX_ITEMS",
			value:    "400",
			expected: 400,
		},
		{
			key:      "process_config.event_collection.store.max_pending_pushes",
			env:      "DD_PROCESS_CONFIG_EVENT_COLLECTION_STORE_MAX_PENDING_PUSHES",
			value:    "100",
			expected: 100,
		},
		{
			key:      "process_config.event_collection.store.max_pending_pulls",
			env:      "DD_PROCESS_CONFIG_EVENT_COLLECTION_STORE_MAX_PENDING_PULLS",
			value:    "50",
			expected: 50,
		},
		{
			key:      "process_config.event_collection.store.stats_interval",
			env:      "DD_PROCESS_CONFIG_EVENT_COLLECTION_STORE_STATS_INTERVAL",
			value:    "60",
			expected: 60,
		},
		{
			key:      "process_config.event_collection.enabled",
			env:      "DD_PROCESS_CONFIG_EVENT_COLLECTION_ENABLED",
			value:    "true",
			expected: true,
		},
		{
			key:      "process_config.event_collection.interval",
			env:      "DD_PROCESS_CONFIG_EVENT_COLLECTION_INTERVAL",
			value:    "20s",
			expected: 20 * time.Second,
		},
		{
			key:      "process_config.language_detection.grpc_port",
			env:      "DD_PROCESS_CONFIG_LANGUAGE_DETECTION_GRPC_PORT",
			value:    "5431",
			expected: 5431,
		},
		{
			key:      "process_config.intervals.connections",
			env:      "DD_PROCESS_CONFIG_INTERVALS_CONNECTIONS",
			value:    "10",
			expected: "10",
		},
		// TODO: process_config.process_collection.use_wlm is a temporary configuration for refactoring purposes
		{
			key:      "process_config.process_collection.use_wlm",
			env:      "DD_PROCESS_CONFIG_PROCESS_COLLECTION_USE_WLM",
			value:    "false",
			expected: false,
		},
	} {
		t.Run(tc.env, func(t *testing.T) {
			// internal configuration rely on a syncOnce so we have to reset if after each call
			t.Cleanup(func() { processesAddOverrideOnce = *new(sync.Once) })

			t.Setenv(tc.env, tc.value)

			cfg := newTestConf(t)
			assert.Equal(t, tc.expected, readCfgWithType(cfg, tc.key, tc.expType))
		})

		// Also test the DD_PROCESS_AGENT prefix if it has one
		if strings.HasPrefix(tc.env, "DD_PROCESS_CONFIG") {
			env := strings.Replace(tc.env, "PROCESS_CONFIG", "PROCESS_AGENT", 1)
			t.Run(env, func(t *testing.T) {
				t.Setenv(env, tc.value)

				cfg := newTestConf(t)
				assert.Equal(t, tc.expected, readCfgWithType(cfg, tc.key, tc.expType))
			})
		}
	}

	cfg := newTestConf(t)
	// StringMapStringSlice can't be converted by `Config.Get` so we need to test this separately
	t.Run("DD_PROCESS_CONFIG_ADDITIONAL_ENDPOINTS", func(t *testing.T) {
		t.Setenv("DD_PROCESS_CONFIG_ADDITIONAL_ENDPOINTS", `{"https://process.datadoghq.com": ["fakeAPIKey"]}`)
		assert.Equal(t, map[string][]string{
			"https://process.datadoghq.com": {
				"fakeAPIKey",
			},
		}, cfg.GetStringMapStringSlice("process_config.additional_endpoints"))
	})

	t.Run("DD_PROCESS_CONFIG_EVENTS_ADDITIONAL_ENDPOINTS", func(t *testing.T) {
		t.Setenv("DD_PROCESS_CONFIG_EVENTS_ADDITIONAL_ENDPOINTS", `{"https://process-events.datadoghq.io": ["fakeAPIKey"]}`)
		assert.Equal(t, map[string][]string{
			"https://process-events.datadoghq.io": {
				"fakeAPIKey",
			},
		}, cfg.GetStringMapStringSlice("process_config.events_additional_endpoints"))
	})
}

func readCfgWithType(cfg pkgconfigmodel.Config, key, expType string) interface{} {
	switch expType {
	case "stringSlice":
		return cfg.GetStringSlice(key)
	case "boolean":
		return cfg.GetBool(key)
	default:
		return cfg.Get(key)
	}
}

func TestEnvVarCustomSensitiveWords(t *testing.T) {
	cfg := newTestConf(t)
	expectedPrefixes := []string{"DD_", "DD_PROCESS_CONFIG_", "DD_PROCESS_AGENT_"}

	for i, tc := range []struct {
		words    string
		expected []string
	}{
		{
			words:    "pass*,word,secret",
			expected: []string{"pass*", "word", "secret"},
		},
		{
			words:    "[\"pass*\", \"word\",\"secret\"]",
			expected: []string{"pass*", "word", "secret"},
		},
		{
			words:    "[pass],word,user",
			expected: []string{"[pass]", "word", "user"},
		},
	} {
		for _, envPrefix := range expectedPrefixes {
			e := envPrefix + "CUSTOM_SENSITIVE_WORDS"
			t.Run(fmt.Sprintf("scrub sensitive words/%d/%s", i, e), func(t *testing.T) {
				t.Setenv(e, tc.words)
				args := cfg.GetStringSlice("process_config.custom_sensitive_words")
				assert.Equal(t, tc.expected, args)
			})
		}
	}
}

func TestProcBindEnvAndSetDefault(t *testing.T) {
	cfg := newTestConf(t)
	procBindEnvAndSetDefault(cfg, "process_config.foo.bar", "asdf")

	envs := map[string]struct{}{}
	for _, env := range cfg.GetEnvVars() {
		envs[env] = struct{}{}
	}

	_, ok := envs["DD_PROCESS_CONFIG_FOO_BAR"]
	assert.True(t, ok)

	_, ok = envs["DD_PROCESS_AGENT_FOO_BAR"]
	assert.True(t, ok)

	// Make sure the default is set properly
	assert.Equal(t, "asdf", cfg.GetString("process_config.foo.bar"))
}

func TestProcBindEnv(t *testing.T) {
	cfg := newTestConf(t)
	procBindEnv(cfg, "process_config.foo.bar")

	envs := map[string]struct{}{}
	for _, env := range cfg.GetEnvVars() {
		envs[env] = struct{}{}
	}

	_, ok := envs["DD_PROCESS_CONFIG_FOO_BAR"]
	assert.True(t, ok)

	_, ok = envs["DD_PROCESS_AGENT_FOO_BAR"]
	assert.True(t, ok)

	// Make sure that DD_PROCESS_CONFIG_FOO_BAR shows up as unset by default
	assert.False(t, cfg.IsSet("process_config.foo.bar"))

	// Try and set DD_PROCESS_CONFIG_FOO_BAR and make sure it shows up in the config
	t.Setenv("DD_PROCESS_CONFIG_FOO_BAR", "baz")
	assert.True(t, cfg.IsSet("process_config.foo.bar"))
	assert.Equal(t, "baz", cfg.GetString("process_config.foo.bar"))
}

func TestProcConfigEnabledTransform(t *testing.T) {
	for _, tc := range []struct {
		procConfigEnabled                                      string
		expectedContainerCollection, expectedProcessCollection bool
	}{
		{
			procConfigEnabled:           "true",
			expectedContainerCollection: false,
			expectedProcessCollection:   true,
		},
		{
			procConfigEnabled:           "false",
			expectedContainerCollection: true,
			expectedProcessCollection:   false,
		},
		{
			procConfigEnabled:           "disabled",
			expectedContainerCollection: false,
			expectedProcessCollection:   false,
		},
	} {
		t.Run("process_config.enabled="+tc.procConfigEnabled, func(t *testing.T) {
			cfg := newTestConf(t)
			cfg.SetWithoutSource("process_config.enabled", tc.procConfigEnabled)
			loadProcessTransforms(cfg)

			assert.Equal(t, tc.expectedContainerCollection, cfg.GetBool("process_config.container_collection.enabled"))
			assert.Equal(t, tc.expectedProcessCollection, cfg.GetBool("process_config.process_collection.enabled"))
		})
	}
}
