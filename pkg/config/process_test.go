// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package config

import (
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

// allProcessSettings is a slice that contains details for testing regarding process agent config settings
// When adding to this list please try to conform to the same ordering that is in `process.go`

var allProcessSettings = []struct {
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
		key:          "process_config.remote_tagger",
		defaultValue: true,
	},
	{
		key:          "process_config.process_discovery.enabled",
		defaultValue: false,
	},
	{
		key:          "process_config.process_discovery.interval",
		defaultValue: 4 * time.Hour,
	},
}

// TestProcessDefaults tests to ensure that the config has set process settings correctly
func TestProcessConfig(t *testing.T) {
	cfg := setupConf()

	for _, tc := range allProcessSettings {
		t.Run(tc.key+" default", func(t *testing.T) {
			assert.Equal(t, tc.defaultValue, cfg.Get(tc.key))
		})
	}
}

// TestPrefixes tests that for every corresponding `DD_PROCESS_CONFIG` prefix, there is a `DD_PROCESS_AGENT` prefix as well.
func TestProcessAgentPrefixes(t *testing.T) {
	envVarSlice := setupConf().GetEnvVars()
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
	envVarSlice := setupConf().GetEnvVars()
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
	cfg := setupConf()

	for _, tc := range []struct {
		key, env, value string
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
	} {
		t.Run(tc.env, func(t *testing.T) {
			reset := setEnvForTest(tc.env, tc.value)
			assert.Equal(t, tc.expected, cfg.Get(tc.key))
			reset()
		})

		// Also test the DD_PROCESS_AGENT prefix if it has one
		if strings.HasPrefix(tc.env, "DD_PROCESS_CONFIG") {
			env := strings.Replace(tc.env, "PROCESS_CONFIG", "PROCESS_AGENT", 1)
			t.Run(env, func(t *testing.T) {
				reset := setEnvForTest(env, tc.value)
				assert.Equal(t, tc.expected, cfg.Get(tc.key))
				reset()
			})
		}
	}
}

func TestProcBindEnvAndSetDefault(t *testing.T) {
	cfg := setupConf()
	procBindEnvAndSetDefault(cfg, "process_config.foo.bar", "asdf")

	envs := map[string]struct{}{}
	for _, env := range cfg.GetEnvVars() {
		envs[env] = struct{}{}
	}

	_, ok := envs["DD_PROCESS_CONFIG_FOO_BAR"]
	assert.True(t, ok)

	_, ok = envs["DD_PROCESS_AGENT_FOO_BAR"]
	assert.True(t, ok)
}
