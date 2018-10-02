// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2018 Datadog, Inc.

package config

import (
	"github.com/stretchr/testify/assert"
	"testing"
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
	assert.Equal(t, 30, LogsAgent.GetInt("logs_config.stop_grace_period"))
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

func TestBuildEndpointsShouldSucceedWithDefaultAndValidOverride(t *testing.T) {
	var endpoints *Endpoints
	var err error
	var endpoint Endpoint

	LogsAgent.Set("api_key", "azerty")
	LogsAgent.Set("logset", "baz")
	LogsAgent.Set("logs_config.socks5_proxy_address", "boz:1234")

	endpoints, err = BuildEndpoints()
	assert.Nil(t, err)
	endpoint = endpoints.Main
	assert.Equal(t, "azerty", endpoint.APIKey)
	assert.Equal(t, "baz", endpoint.Logset)
	assert.Equal(t, "agent-intake.logs.datadoghq.com", endpoint.Host)
	assert.Equal(t, 10516, endpoint.Port)
	assert.True(t, endpoint.UseSSL)
	assert.Equal(t, "boz:1234", endpoint.ProxyAddress)
	assert.Equal(t, 0, len(endpoints.Additionals))

	LogsAgent.Set("logs_config.use_port_443", true)
	endpoints, err = BuildEndpoints()
	assert.Nil(t, err)
	endpoint = endpoints.Main
	assert.Equal(t, "azerty", endpoint.APIKey)
	assert.Equal(t, "baz", endpoint.Logset)
	assert.Equal(t, "agent-443-intake.logs.datadoghq.com", endpoint.Host)
	assert.Equal(t, 443, endpoint.Port)
	assert.True(t, endpoint.UseSSL)
	assert.Equal(t, "boz:1234", endpoint.ProxyAddress)
	assert.Equal(t, 0, len(endpoints.Additionals))

	LogsAgent.Set("logs_config.logs_dd_url", "host:1234")
	LogsAgent.Set("logs_config.logs_no_ssl", true)
	endpoints, err = BuildEndpoints()
	assert.Nil(t, err)
	endpoint = endpoints.Main
	assert.Equal(t, "azerty", endpoint.APIKey)
	assert.Equal(t, "baz", endpoint.Logset)
	assert.Equal(t, "host", endpoint.Host)
	assert.Equal(t, 1234, endpoint.Port)
	assert.False(t, endpoint.UseSSL)
	assert.Equal(t, "boz:1234", endpoint.ProxyAddress)
	assert.Equal(t, 0, len(endpoints.Additionals))

	LogsAgent.Set("logs_config.logs_dd_url", ":1234")
	LogsAgent.Set("logs_config.logs_no_ssl", false)
	endpoints, err = BuildEndpoints()
	assert.Nil(t, err)
	endpoint = endpoints.Main
	assert.Equal(t, "azerty", endpoint.APIKey)
	assert.Equal(t, "baz", endpoint.Logset)
	assert.Equal(t, "", endpoint.Host)
	assert.Equal(t, 1234, endpoint.Port)
	assert.True(t, endpoint.UseSSL)
	assert.Equal(t, "boz:1234", endpoint.ProxyAddress)
	assert.Equal(t, 0, len(endpoints.Additionals))
}

func TestBuildEndpointsShouldFailWithInvalidOverride(t *testing.T) {
	invalidURLs := []string{
		"host:foo",
		"host",
	}

	for _, url := range invalidURLs {
		LogsAgent.Set("logs_config.logs_dd_url", url)
		_, err := BuildEndpoints()
		assert.NotNil(t, err)
	}
}
