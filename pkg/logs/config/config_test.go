// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2018 Datadog, Inc.

package config

import (
	"testing"

	"github.com/stretchr/testify/assert"

	coreConfig "github.com/DataDog/datadog-agent/pkg/config"

	"github.com/DataDog/datadog-agent/pkg/logs/client"
)

func TestDefaultDatadogConfig(t *testing.T) {
	assert.Equal(t, false, coreConfig.Datadog.GetBool("log_enabled"))
	assert.Equal(t, false, coreConfig.Datadog.GetBool("logs_enabled"))
	assert.Equal(t, "", coreConfig.Datadog.GetString("logset"))
	assert.Equal(t, "agent-intake.logs.datadoghq.com", coreConfig.Datadog.GetString("logs_config.dd_url"))
	assert.Equal(t, 10516, coreConfig.Datadog.GetInt("logs_config.dd_port"))
	assert.Equal(t, false, coreConfig.Datadog.GetBool("logs_config.dev_mode_no_ssl"))
	assert.Equal(t, "agent-443-intake.logs.datadoghq.com", coreConfig.Datadog.GetString("logs_config.dd_url_443"))
	assert.Equal(t, false, coreConfig.Datadog.GetBool("logs_config.use_port_443"))
	assert.Equal(t, true, coreConfig.Datadog.GetBool("logs_config.dev_mode_use_proto"))
	assert.Equal(t, 100, coreConfig.Datadog.GetInt("logs_config.open_files_limit"))
	assert.Equal(t, 9000, coreConfig.Datadog.GetInt("logs_config.frame_size"))
	assert.Equal(t, "", coreConfig.Datadog.GetString("logs_config.socks5_proxy_address"))
	assert.Equal(t, "", coreConfig.Datadog.GetString("logs_config.logs_dd_url"))
	assert.Equal(t, false, coreConfig.Datadog.GetBool("logs_config.logs_no_ssl"))
	assert.Equal(t, 30, coreConfig.Datadog.GetInt("logs_config.stop_grace_period"))
}

func TestDefaultSources(t *testing.T) {
	mockConfig := coreConfig.Mock()

	var sources []*LogSource
	var source *LogSource

	mockConfig.Set("logs_config.container_collect_all", true)

	sources = DefaultSources()
	assert.Equal(t, 1, len(sources))

	source = sources[0]
	assert.Equal(t, "container_collect_all", source.Name)
	assert.Equal(t, DockerType, source.Config.Type)
	assert.Equal(t, "docker", source.Config.Source)
	assert.Equal(t, "docker", source.Config.Service)
}

func TestBuildEndpointsShouldSucceedWithDefaultAndValidOverride(t *testing.T) {
	mockConfig := coreConfig.Mock()

	var endpoints *client.Endpoints

	var err error
	var endpoint client.Endpoint

	mockConfig.Set("api_key", "azerty")
	mockConfig.Set("logset", "baz")
	mockConfig.Set("logs_config.socks5_proxy_address", "boz:1234")

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

	mockConfig.Set("logs_config.use_port_443", true)
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

	mockConfig.Set("logs_config.logs_dd_url", "host:1234")
	mockConfig.Set("logs_config.logs_no_ssl", true)
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

	mockConfig.Set("logs_config.logs_dd_url", ":1234")
	mockConfig.Set("logs_config.logs_no_ssl", false)
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
	mockConfig := coreConfig.Mock()

	invalidURLs := []string{
		"host:foo",
		"host",
	}

	for _, url := range invalidURLs {
		mockConfig.Set("logs_config.logs_dd_url", url)
		_, err := BuildEndpoints()
		assert.NotNil(t, err)
	}
}
