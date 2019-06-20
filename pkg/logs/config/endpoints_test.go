// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-2019 Datadog, Inc.

package config

import (
	"testing"

	"github.com/stretchr/testify/suite"

	coreConfig "github.com/DataDog/datadog-agent/pkg/config"
)

type EndpointsTestSuite struct {
	suite.Suite
	config *coreConfig.MockConfig
}

func (suite *EndpointsTestSuite) SetupTest() {
	suite.config = coreConfig.Mock()
}

func (suite *EndpointsTestSuite) TestLogsEndpointConfig() {
	suite.Equal("agent-intake.logs.datadoghq.com", coreConfig.GetMainEndpoint(tcpEndpointPrefix, "logs_config.dd_url"))
	endpoints, err := BuildEndpoints()
	suite.Nil(err)
	suite.Equal("agent-intake.logs.datadoghq.com", endpoints.Main.Host)
	suite.Equal(10516, endpoints.Main.Port)

	suite.config.Set("site", "datadoghq.com")
	suite.Equal("agent-intake.logs.datadoghq.com", coreConfig.GetMainEndpoint(tcpEndpointPrefix, "logs_config.dd_url"))
	endpoints, err = BuildEndpoints()
	suite.Nil(err)
	suite.Equal("agent-intake.logs.datadoghq.com", endpoints.Main.Host)
	suite.Equal(10516, endpoints.Main.Port)

	suite.config.Set("site", "datadoghq.eu")
	suite.Equal("agent-intake.logs.datadoghq.eu", coreConfig.GetMainEndpoint(tcpEndpointPrefix, "logs_config.dd_url"))
	endpoints, err = BuildEndpoints()
	suite.Nil(err)
	suite.Equal("agent-intake.logs.datadoghq.eu", endpoints.Main.Host)
	suite.Equal(443, endpoints.Main.Port)

	suite.config.Set("logs_config.dd_url", "lambda.logs.datadoghq.co.jp")
	suite.Equal("lambda.logs.datadoghq.co.jp", coreConfig.GetMainEndpoint(tcpEndpointPrefix, "logs_config.dd_url"))
	endpoints, err = BuildEndpoints()
	suite.Nil(err)
	suite.Equal("lambda.logs.datadoghq.co.jp", endpoints.Main.Host)
	suite.Equal(10516, endpoints.Main.Port)

	suite.config.Set("logs_config.logs_dd_url", "azure.logs.datadoghq.co.uk:1234")
	suite.Equal("azure.logs.datadoghq.co.uk:1234", coreConfig.GetMainEndpoint(tcpEndpointPrefix, "logs_config.logs_dd_url"))
	endpoints, err = BuildEndpoints()
	suite.Nil(err)
	suite.Equal("azure.logs.datadoghq.co.uk", endpoints.Main.Host)
	suite.Equal(1234, endpoints.Main.Port)
}

func (suite *EndpointsTestSuite) TestBuildEndpointsShouldSucceedWithDefaultAndValidOverride() {
	var endpoints *Endpoints

	var err error
	var endpoint Endpoint

	suite.config.Set("api_key", "azerty")
	suite.config.Set("logs_config.socks5_proxy_address", "boz:1234")

	endpoints, err = BuildEndpoints()
	suite.Nil(err)
	endpoint = endpoints.Main
	suite.Equal("azerty", endpoint.APIKey)
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
	suite.Equal("", endpoint.Host)
	suite.Equal(1234, endpoint.Port)
	suite.True(endpoint.UseSSL)
	suite.Equal("boz:1234", endpoint.ProxyAddress)
	suite.Equal(0, len(endpoints.Additionals))
}

func (suite *EndpointsTestSuite) TestBuildEndpointsShouldSucceedWithValidHTTPConfig() {
	var endpoints *Endpoints
	var endpoint Endpoint
	var err error

	suite.config.Set("logs_config.use_http", true)

	endpoints, err = BuildEndpoints()
	suite.Nil(err)
	suite.True(endpoints.UseHTTP)

	endpoint = endpoints.Main
	suite.True(endpoint.UseSSL)
	suite.Equal("agent-http-intake.logs.datadoghq.com", endpoint.Host)
}

func (suite *EndpointsTestSuite) TestBuildEndpointsShouldSucceedWithValidHTTPConfigAndOverride() {
	var endpoints *Endpoints
	var endpoint Endpoint
	var err error

	suite.config.Set("logs_config.use_http", true)
	suite.config.Set("logs_config.dd_url", "foo")

	endpoints, err = BuildEndpoints()
	suite.Nil(err)
	suite.True(endpoints.UseHTTP)

	endpoint = endpoints.Main
	suite.True(endpoint.UseSSL)
	suite.Equal("foo", endpoint.Host)
}

func (suite *EndpointsTestSuite) TestBuildEndpointsShouldSucceedWithValidProxyConfig() {
	var endpoints *Endpoints
	var endpoint Endpoint
	var err error

	suite.config.Set("logs_config.use_http", true)
	suite.config.Set("logs_config.logs_dd_url", "foo:1234")

	endpoints, err = BuildEndpoints()
	suite.Nil(err)
	suite.True(endpoints.UseHTTP)

	endpoint = endpoints.Main
	suite.True(endpoint.UseSSL)
	suite.Equal("foo", endpoint.Host)
	suite.Equal(1234, endpoint.Port)
}

func (suite *EndpointsTestSuite) TestBuildEndpointsShouldFailWithInvalidProxyConfig() {
	var err error

	suite.config.Set("logs_config.use_http", true)
	suite.config.Set("logs_config.logs_dd_url", "foo")

	_, err = BuildEndpoints()
	suite.NotNil(err)
}

func (suite *EndpointsTestSuite) TestBuildEndpointsShouldFailWithInvalidOverride() {
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

//When migrating the agent v5 to v6, logs_dd_url is set to empty. Default to the dd_url/site already set instead.
func (suite *EndpointsTestSuite) TestBuildEndpointsShouldSucceedWhenMigratingToAgentV6() {
	suite.config.Set("logs_config.logs_dd_url", "")
	endpoints, err := BuildEndpoints()
	suite.Nil(err)
	suite.Equal("agent-intake.logs.datadoghq.com", endpoints.Main.Host)
	suite.Equal(10516, endpoints.Main.Port)
}

func (suite *EndpointsTestSuite) TestIsSetAndNotEmpty() {
	suite.config.Set("bob", "vanilla")
	suite.config.Set("empty", "")
	suite.True(isSetAndNotEmpty(suite.config, "bob"))
	suite.False(isSetAndNotEmpty(suite.config, "empty"))
	suite.False(isSetAndNotEmpty(suite.config, "wassup"))
}

func (suite *EndpointsTestSuite) TestDefaultApiKey() {
	suite.config.Set("api_key", "wassupkey")
	suite.Equal("wassupkey", getLogsAPIKey(suite.config))
	endpoints, err := BuildEndpoints()
	suite.Nil(err)
	suite.Equal("wassupkey", endpoints.Main.APIKey)
}

func (suite *EndpointsTestSuite) TestOverrideApiKey() {
	suite.config.Set("api_key", "wassupkey")
	suite.config.Set("logs_config.api_key", "wassuplogskey")
	suite.Equal("wassuplogskey", getLogsAPIKey(suite.config))
	endpoints, err := BuildEndpoints()
	suite.Nil(err)
	suite.Equal("wassuplogskey", endpoints.Main.APIKey)
}

func (suite *EndpointsTestSuite) TestAdditionalEndpoints() {
	var (
		endpoints *Endpoints
		endpoint  Endpoint
		err       error
	)

	suite.config.Set("logs_config.additional_endpoints", []map[string]interface{}{
		{
			"host":    "foo",
			"api_key": "1234",
		},
	})

	endpoints, err = BuildEndpoints()
	suite.Nil(err)
	suite.Len(endpoints.Additionals, 1)

	endpoint = endpoints.Additionals[0]
	suite.Equal("foo", endpoint.Host)
	suite.Equal("1234", endpoint.APIKey)
	suite.True(endpoint.UseSSL)

	suite.config.Set("logs_config.use_http", true)
	endpoints, err = BuildEndpoints()
	suite.Nil(err)
	suite.Len(endpoints.Additionals, 1)

	endpoint = endpoints.Additionals[0]
	suite.Equal("foo", endpoint.Host)
	suite.Equal("1234", endpoint.APIKey)
	suite.True(endpoint.UseSSL)
}

func TestEndpointsTestSuite(t *testing.T) {
	suite.Run(t, new(EndpointsTestSuite))
}
