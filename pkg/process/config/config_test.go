// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux || windows
// +build linux windows

package config

import (
	"os"
	"runtime"
	"strings"
	"testing"
	"time"

	sysconfig "github.com/DataDog/datadog-agent/cmd/system-probe/config"
	"github.com/DataDog/datadog-agent/pkg/config"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

var originalConfig = config.Datadog

const (
	ns = "process_config"
)

func key(pieces ...string) string {
	return strings.Join(pieces, ".")
}

func restoreGlobalConfig() {
	config.Datadog = originalConfig
}

func newConfig() {
	config.Datadog = config.NewConfig("datadog", "DD", strings.NewReplacer(".", "_"))
	config.InitConfig(config.Datadog)
	// force timeout to 0s, otherwise each test waits 60s
	config.Datadog.Set(key(ns, "grpc_connection_timeout_secs"), 0)
}

func loadAgentConfigForTest(t *testing.T, path, networksYamlPath string) *AgentConfig {
	config.InitSystemProbeConfig(config.Datadog)

	require.NoError(t, LoadConfigIfExists(path))

	syscfg, err := sysconfig.Merge(networksYamlPath)
	require.NoError(t, err)

	cfg, err := NewAgentConfig("test", path, syscfg)
	require.NoError(t, err)
	return cfg
}

// TestEnvGrpcConnectionTimeoutSecs tests DD_PROCESS_CONFIG_GRPC_CONNECTION_TIMEOUT_SECS.
// This environment variable cannot be tested with the other environment variables because it is overridden.
func TestEnvGrpcConnectionTimeoutSecs(t *testing.T) {
	syscfg, err := sysconfig.Merge("")
	require.NoError(t, err)

	t.Run("DD_PROCESS_CONFIG_GRPC_CONNECTION_TIMEOUT_SECS", func(t *testing.T) {
		t.Setenv("DD_PROCESS_CONFIG_GRPC_CONNECTION_TIMEOUT_SECS", "1")
		_, _ = NewAgentConfig("test", "", syscfg)
		assert.Equal(t, 1, config.Datadog.GetInt("process_config.grpc_connection_timeout_secs"))
	})

	t.Run("DD_PROCESS_AGENT_GRPC_CONNECTION_TIMEOUT_SECS", func(t *testing.T) {
		t.Setenv("DD_PROCESS_AGENT_GRPC_CONNECTION_TIMEOUT_SECS", "2")
		_, _ = NewAgentConfig("test", "", syscfg)
		assert.Equal(t, 2, config.Datadog.GetInt("process_config.grpc_connection_timeout_secs"))
	})
}

func TestYamlConfig(t *testing.T) {
	// Reset the config
	config.Datadog = config.NewConfig("datadog", "DD", strings.NewReplacer(".", "_"))

	f, err := os.CreateTemp("", "yamlConfigTest*.yaml")
	defer os.Remove(f.Name())
	assert.NoError(t, err)

	_, err = f.WriteString(`
log_level: debug
log_to_console: false
process_config:
    log_file: /tmp/test
    dd_agent_bin: /tmp/test
    grpc_connection_timeout_secs: 1
    remote_tagger: true
    process_discovery:
        enabled: true
        interval: 1h
`)
	require.NoError(t, err)

	require.NoError(t, LoadConfigIfExists(f.Name()))

	assert.Equal(t, "debug", config.Datadog.GetString("log_level"))
	assert.False(t, config.Datadog.GetBool("log_to_console"))
	assert.Equal(t, "/tmp/test", config.Datadog.GetString("process_config.log_file"))
	assert.Equal(t, "/tmp/test", config.Datadog.GetString("process_config.dd_agent_bin"))
	assert.Equal(t, 1, config.Datadog.GetInt("process_config.grpc_connection_timeout_secs"))
	assert.True(t, config.Datadog.GetBool("process_config.remote_tagger"))
	assert.True(t, config.Datadog.GetBool("process_config.process_discovery.enabled"))
	assert.Equal(t, time.Hour, config.Datadog.GetDuration("process_config.process_discovery.interval"))
}

func TestOnlyEnvConfigLogLevelOverride(t *testing.T) {
	newConfig()
	defer restoreGlobalConfig()

	t.Setenv("DD_LOG_LEVEL", "error")
	t.Setenv("LOG_LEVEL", "debug")

	syscfg, err := sysconfig.Merge("")
	require.NoError(t, err)
	_, _ = NewAgentConfig("test", "", syscfg)
	assert.Equal(t, "error", config.Datadog.GetString("log_level"))
}

func TestDefaultConfig(t *testing.T) {
	assert := assert.New(t)

	// assert that some sane defaults are set
	assert.Equal("info", config.Datadog.GetString("log_level"))

	t.Setenv("DOCKER_DD_AGENT", "yes")
	_ = NewDefaultAgentConfig()
	assert.Equal(os.Getenv("HOST_PROC"), "")
	assert.Equal(os.Getenv("HOST_SYS"), "")
	t.Setenv("DOCKER_DD_AGENT", "no")
	assert.Equal(config.DefaultProcessExpVarPort, config.Datadog.GetInt("process_config.expvar_port"))

	assert.Equal("info", config.Datadog.GetString("log_level"))
	assert.True(config.Datadog.GetBool("log_to_console"))
	assert.Equal(config.DefaultProcessAgentLogFile, config.Datadog.GetString("process_config.log_file"))
	assert.Equal(config.DefaultDDAgentBin, config.Datadog.GetString("process_config.dd_agent_bin"))
	assert.Equal(config.DefaultGRPCConnectionTimeoutSecs, config.Datadog.GetInt("process_config.grpc_connection_timeout_secs"))
	assert.False(config.Datadog.GetBool("process_config.remote_tagger"))
	assert.False(config.Datadog.GetBool("process_config.remote_workloadmeta"))
	assert.True(config.Datadog.GetBool("process_config.process_discovery.enabled"))
	assert.Equal(4*time.Hour, config.Datadog.GetDuration("process_config.process_discovery.interval"))
}

func TestAgentConfigYamlAndSystemProbeConfig(t *testing.T) {
	newConfig()
	defer restoreGlobalConfig()

	assert := assert.New(t)

	_ = loadAgentConfigForTest(t, "./testdata/TestDDAgentConfigYamlAndSystemProbeConfig.yaml", "")

	assert.Equal("apikey_20", config.Datadog.GetString("api_key"))
	assert.Equal("http://my-process-app.datadoghq.com", config.Datadog.GetString("process_config.process_dd_url"))
	assert.Equal(10, config.Datadog.GetInt("process_config.queue_size"))
	assert.Equal(5065, config.Datadog.GetInt("process_config.expvar_port"))

	newConfig()
	agentConfig := loadAgentConfigForTest(t, "./testdata/TestDDAgentConfigYamlAndSystemProbeConfig.yaml", "./testdata/TestDDAgentConfigYamlAndSystemProbeConfig-Net.yaml")

	assert.Equal("apikey_20", config.Datadog.GetString("api_key"))
	assert.Equal("http://my-process-app.datadoghq.com", config.Datadog.GetString("process_config.process_dd_url"))
	assert.Equal(10, config.Datadog.GetInt("process_config.queue_size"))
	if runtime.GOOS != "windows" {
		assert.Equal("/var/my-location/system-probe.log", agentConfig.SystemProbeAddress)
	}

	newConfig()
	_ = loadAgentConfigForTest(t, "./testdata/TestDDAgentConfigYamlAndSystemProbeConfig.yaml", "./testdata/TestDDAgentConfigYamlAndSystemProbeConfig-Net-2.yaml")

	assert.Equal("apikey_20", config.Datadog.GetString("api_key"))
	assert.Equal("http://my-process-app.datadoghq.com", config.Datadog.GetString("process_config.process_dd_url"))
	assert.Equal(10, config.Datadog.GetInt("process_config.queue_size"))

	newConfig()
	agentConfig = loadAgentConfigForTest(t, "./testdata/TestDDAgentConfigYamlAndSystemProbeConfig.yaml", "./testdata/TestDDAgentConfigYamlAndSystemProbeConfig-Net-Windows.yaml")

	if runtime.GOOS == "windows" {
		assert.Equal("localhost:4444", agentConfig.SystemProbeAddress)
	}
}
