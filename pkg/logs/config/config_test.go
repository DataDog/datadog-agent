// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2018 Datadog, Inc.

package config

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestDefaultDatadogConfig(t *testing.T) {
	assert.Equal(t, false, LogsAgent.GetBool("log_enabled"))
	assert.Equal(t, false, LogsAgent.GetBool("logs_enabled"))
	assert.Equal(t, "", LogsAgent.GetString("logset"))
	assert.Equal(t, "agent-intake.logs.datadoghq.com", LogsAgent.GetString("logs_config.dd_url"))
	assert.Equal(t, 10516, LogsAgent.GetInt("logs_config.dd_port"))
	assert.Equal(t, false, LogsAgent.GetBool("logs_config.dev_mode_no_ssl"))
	assert.Equal(t, "agent-443-intake.logs.datadoghq.com", LogsAgent.GetString("logs_config.dd_url_443"))
	assert.Equal(t, false, LogsAgent.GetBool("logs_config.use_port_443"))
	assert.Equal(t, true, LogsAgent.GetBool("logs_config.dev_mode_use_proto"))
	assert.Equal(t, 100, LogsAgent.GetInt("logs_config.open_files_limit"))
	assert.Equal(t, 9000, LogsAgent.GetInt("logs_config.frame_size"))
	assert.Equal(t, -1, LogsAgent.GetInt("logs_config.tcp_forward_port"))
	assert.Equal(t, "", LogsAgent.GetString("logs_config.socks5_proxy_address"))
	assert.Equal(t, "", LogsAgent.GetString("logs_config.logs_dd_url"))
	assert.Equal(t, false, LogsAgent.GetBool("logs_config.logs_no_ssl"))
}

func TestDefaultSources(t *testing.T) {
	var sources []*LogSource
	var source *LogSource

	LogsAgent.Set("logs_config.tcp_forward_port", 1234)
	LogsAgent.Set("logs_config.container_collect_all", true)

	sources = DefaultSources()
	assert.Equal(t, 2, len(sources))

	source = sources[0]
	assert.Equal(t, "tcp_forward", source.Name)
	assert.Equal(t, TCPType, source.Config.Type)
	assert.Equal(t, 1234, source.Config.Port)

	source = sources[1]
	assert.Equal(t, "container_collect_all", source.Name)
	assert.Equal(t, DockerType, source.Config.Type)
	assert.Equal(t, "docker", source.Config.Source)
	assert.Equal(t, "docker", source.Config.Service)
}

func TestBuildServerConfigShouldSucceedWithDefaultAndValidOverride(t *testing.T) {
	var serverConfig *ServerConfig
	var err error

	serverConfig, err = BuildServerConfig()
	assert.Nil(t, err)
	assert.Equal(t, "agent-intake.logs.datadoghq.com", serverConfig.Name)
	assert.Equal(t, 10516, serverConfig.Port)
	assert.True(t, serverConfig.UseSSL)
	assert.Equal(t, "agent-intake.logs.datadoghq.com:10516", serverConfig.Address())

	LogsAgent.Set("logs_config.use_port_443", true)
	serverConfig, err = BuildServerConfig()
	assert.Nil(t, err)
	assert.Equal(t, "agent-443-intake.logs.datadoghq.com", serverConfig.Name)
	assert.Equal(t, 443, serverConfig.Port)
	assert.True(t, serverConfig.UseSSL)
	assert.Equal(t, "agent-443-intake.logs.datadoghq.com:443", serverConfig.Address())

	LogsAgent.Set("logs_config.logs_dd_url", "host:1234")
	LogsAgent.Set("logs_config.logs_no_ssl", true)
	serverConfig, err = BuildServerConfig()
	assert.Nil(t, err)
	assert.Equal(t, "host", serverConfig.Name)
	assert.Equal(t, 1234, serverConfig.Port)
	assert.False(t, serverConfig.UseSSL)
	assert.Equal(t, "host:1234", serverConfig.Address())

	LogsAgent.Set("logs_config.logs_dd_url", ":1234")
	LogsAgent.Set("logs_config.logs_no_ssl", false)
	serverConfig, err = BuildServerConfig()
	assert.Nil(t, err)
	assert.Equal(t, "", serverConfig.Name)
	assert.Equal(t, 1234, serverConfig.Port)
	assert.True(t, serverConfig.UseSSL)
	assert.Equal(t, ":1234", serverConfig.Address())
}

func TestBuildServerConfigShouldFailWithInvalidOverride(t *testing.T) {
	invalidURLs := []string{
		"host:foo",
		"host",
	}

	for _, url := range invalidURLs {
		LogsAgent.Set("logs_config.logs_dd_url", url)
		_, err := BuildServerConfig()
		assert.NotNil(t, err)
	}
}
