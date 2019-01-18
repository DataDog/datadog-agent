// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-2019 Datadog, Inc.

package config

import (
	"testing"

	"github.com/DataDog/datadog-agent/pkg/logs/types"
	"github.com/stretchr/testify/suite"

	coreConfig "github.com/DataDog/datadog-agent/pkg/config"

	"github.com/DataDog/datadog-agent/pkg/logs/client"
)

type ConfigTestSuite struct {
	suite.Suite
	config *coreConfig.MockConfig
}

func (suite *ConfigTestSuite) SetupTest() {
	suite.config = coreConfig.Mock()
}

func (suite *ConfigTestSuite) TestDefaultDatadogConfig() {
	suite.Equal(false, suite.config.GetBool("log_enabled"))
	suite.Equal(false, suite.config.GetBool("logs_enabled"))
	suite.Equal("", suite.config.GetString("logset"))
	suite.Equal("", suite.config.GetString("logs_config.dd_url"))
	suite.Equal(10516, suite.config.GetInt("logs_config.dd_port"))
	suite.Equal(false, suite.config.GetBool("logs_config.dev_mode_no_ssl"))
	suite.Equal("agent-443-intake.logs.datadoghq.com", suite.config.GetString("logs_config.dd_url_443"))
	suite.Equal(false, suite.config.GetBool("logs_config.use_port_443"))
	suite.Equal(true, suite.config.GetBool("logs_config.dev_mode_use_proto"))
	suite.Equal(100, suite.config.GetInt("logs_config.open_files_limit"))
	suite.Equal(9000, suite.config.GetInt("logs_config.frame_size"))
	suite.Equal("", suite.config.GetString("logs_config.socks5_proxy_address"))
	suite.Equal("", suite.config.GetString("logs_config.logs_dd_url"))
	suite.Equal(false, suite.config.GetBool("logs_config.logs_no_ssl"))
	suite.Equal(30, suite.config.GetInt("logs_config.stop_grace_period"))
}

func (suite *ConfigTestSuite) TestLogsEndpointConfig() {
	suite.Equal("agent-intake.logs.datadoghq.com", coreConfig.GetMainEndpoint(endpointPrefix, "logs_config.dd_url"))
	endpoints, err := BuildEndpoints()
	suite.Nil(err)
	suite.Equal("agent-intake.logs.datadoghq.com", endpoints.Main.Host)
	suite.Equal(10516, endpoints.Main.Port)

	suite.config.Set("site", "datadoghq.com")
	suite.Equal("agent-intake.logs.datadoghq.com", coreConfig.GetMainEndpoint(endpointPrefix, "logs_config.dd_url"))
	endpoints, err = BuildEndpoints()
	suite.Nil(err)
	suite.Equal("agent-intake.logs.datadoghq.com", endpoints.Main.Host)
	suite.Equal(10516, endpoints.Main.Port)

	suite.config.Set("site", "datadoghq.eu")
	suite.Equal("agent-intake.logs.datadoghq.eu", coreConfig.GetMainEndpoint(endpointPrefix, "logs_config.dd_url"))
	endpoints, err = BuildEndpoints()
	suite.Nil(err)
	suite.Equal("agent-intake.logs.datadoghq.eu", endpoints.Main.Host)
	suite.Equal(443, endpoints.Main.Port)

	suite.config.Set("logs_config.dd_url", "lambda.logs.datadoghq.co.jp")
	suite.Equal("lambda.logs.datadoghq.co.jp", coreConfig.GetMainEndpoint(endpointPrefix, "logs_config.dd_url"))
	endpoints, err = BuildEndpoints()
	suite.Nil(err)
	suite.Equal("lambda.logs.datadoghq.co.jp", endpoints.Main.Host)
	suite.Equal(10516, endpoints.Main.Port)

	suite.config.Set("logs_config.logs_dd_url", "azure.logs.datadoghq.co.uk:1234")
	suite.Equal("azure.logs.datadoghq.co.uk:1234", coreConfig.GetMainEndpoint(endpointPrefix, "logs_config.logs_dd_url"))
	endpoints, err = BuildEndpoints()
	suite.Nil(err)
	suite.Equal("azure.logs.datadoghq.co.uk", endpoints.Main.Host)
	suite.Equal(1234, endpoints.Main.Port)
}

func (suite *ConfigTestSuite) TestDefaultSources() {
	var sources []*LogSource
	var source *LogSource

	suite.config.Set("logs_config.container_collect_all", true)

	sources = DefaultSources()
	suite.Equal(1, len(sources))

	source = sources[0]
	suite.Equal("container_collect_all", source.Name)
	suite.Equal(types.DockerType, source.Config.GetType())
	suite.Equal("docker", source.Config.GetSource())
	suite.Equal("docker", source.Config.GetService())
}

func (suite *ConfigTestSuite) TestBuildEndpointsShouldSucceedWithDefaultAndValidOverride() {
	var endpoints *client.Endpoints

	var err error
	var endpoint client.Endpoint

	suite.config.Set("api_key", "azerty")
	suite.config.Set("logset", "baz")
	suite.config.Set("logs_config.socks5_proxy_address", "boz:1234")

	endpoints, err = BuildEndpoints()
	suite.Nil(err)
	endpoint = endpoints.Main
	suite.Equal("azerty", endpoint.APIKey)
	suite.Equal("baz", endpoint.Logset)
	suite.Equal("agent-intake.logs.datadoghq.com", endpoint.Host)
	suite.Equal(10516, endpoint.Port)
	suite.True(endpoint.UseSSL)
	suite.Equal("boz:1234", endpoint.ProxyAddress)
	suite.Equal(0, len(endpoints.Additionals))

	suite.config.Set("logs_config.use_port_443", true)
	endpoints, err = BuildEndpoints()
	suite.Nil(err)
	endpoint = endpoints.Main
	suite.Equal("azerty", endpoint.APIKey)
	suite.Equal("baz", endpoint.Logset)
	suite.Equal("agent-443-intake.logs.datadoghq.com", endpoint.Host)
	suite.Equal(443, endpoint.Port)
	suite.True(endpoint.UseSSL)
	suite.Equal("boz:1234", endpoint.ProxyAddress)
	suite.Equal(0, len(endpoints.Additionals))

	suite.config.Set("logs_config.logs_dd_url", "host:1234")
	suite.config.Set("logs_config.logs_no_ssl", true)
	endpoints, err = BuildEndpoints()
	suite.Nil(err)
	endpoint = endpoints.Main
	suite.Equal("azerty", endpoint.APIKey)
	suite.Equal("baz", endpoint.Logset)
	suite.Equal("host", endpoint.Host)
	suite.Equal(1234, endpoint.Port)
	suite.False(endpoint.UseSSL)
	suite.Equal("boz:1234", endpoint.ProxyAddress)
	suite.Equal(0, len(endpoints.Additionals))

	suite.config.Set("logs_config.logs_dd_url", ":1234")
	suite.config.Set("logs_config.logs_no_ssl", false)
	endpoints, err = BuildEndpoints()
	suite.Nil(err)
	endpoint = endpoints.Main
	suite.Equal("azerty", endpoint.APIKey)
	suite.Equal("baz", endpoint.Logset)
	suite.Equal("", endpoint.Host)
	suite.Equal(1234, endpoint.Port)
	suite.True(endpoint.UseSSL)
	suite.Equal("boz:1234", endpoint.ProxyAddress)
	suite.Equal(0, len(endpoints.Additionals))
}

func (suite *ConfigTestSuite) TestBuildEndpointsShouldFailWithInvalidOverride() {
	invalidURLs := []string{
		"host:foo",
		"host",
	}

	for _, url := range invalidURLs {
		suite.config.Set("logs_config.logs_dd_url", url)
		_, err := BuildEndpoints()
		suite.NotNil(err)
	}
}

func TestConfigTestSuite(t *testing.T) {
	suite.Run(t, new(ConfigTestSuite))
}
