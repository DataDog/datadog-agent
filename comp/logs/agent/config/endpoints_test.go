// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package config

import (
	"testing"
	"time"

	"github.com/stretchr/testify/suite"
	"go.uber.org/fx"

	"github.com/DataDog/datadog-agent/comp/core/config"
	pkgconfigsetup "github.com/DataDog/datadog-agent/pkg/config/setup"
	pkgconfigutils "github.com/DataDog/datadog-agent/pkg/config/utils"
	"github.com/DataDog/datadog-agent/pkg/util/fxutil"
)

type EndpointsTestSuite struct {
	suite.Suite
	config config.Mock
}

func (suite *EndpointsTestSuite) SetupTest() {
	suite.config = fxutil.Test[config.Component](suite.T(), fx.Options(
		config.MockModule(),
	)).(config.Mock)
}

func (suite *EndpointsTestSuite) TestLogsEndpointConfig() {
	suite.Equal("agent-intake.logs.datadoghq.com", pkgconfigutils.GetMainEndpoint(suite.config, tcpEndpointPrefix, "logs_config.dd_url"))
	endpoints, err := BuildEndpoints(suite.config, HTTPConnectivityFailure, "test-track", "test-proto", "test-source")
	suite.Nil(err)
	suite.Equal("agent-intake.logs.datadoghq.com", endpoints.Main.Host)
	suite.Equal(10516, endpoints.Main.Port)

	suite.config.SetWithoutSource("site", "datadoghq.com")
	suite.Equal("agent-intake.logs.datadoghq.com", pkgconfigutils.GetMainEndpoint(suite.config, tcpEndpointPrefix, "logs_config.dd_url"))
	endpoints, err = BuildEndpoints(suite.config, HTTPConnectivityFailure, "test-track", "test-proto", "test-source")
	suite.Nil(err)
	suite.Equal("agent-intake.logs.datadoghq.com", endpoints.Main.Host)
	suite.Equal(10516, endpoints.Main.Port)

	suite.config.SetWithoutSource("site", "datadoghq.eu")
	suite.Equal("agent-intake.logs.datadoghq.eu", pkgconfigutils.GetMainEndpoint(suite.config, tcpEndpointPrefix, "logs_config.dd_url"))
	endpoints, err = BuildEndpoints(suite.config, HTTPConnectivityFailure, "test-track", "test-proto", "test-source")
	suite.Nil(err)
	suite.Equal("agent-intake.logs.datadoghq.eu", endpoints.Main.Host)
	suite.Equal(443, endpoints.Main.Port)

	suite.config.SetWithoutSource("logs_config.dd_url", "lambda.logs.datadoghq.co.jp")
	suite.Equal("lambda.logs.datadoghq.co.jp", pkgconfigutils.GetMainEndpoint(suite.config, tcpEndpointPrefix, "logs_config.dd_url"))
	endpoints, err = BuildEndpoints(suite.config, HTTPConnectivityFailure, "test-track", "test-proto", "test-source")
	suite.Nil(err)
	suite.Equal("lambda.logs.datadoghq.co.jp", endpoints.Main.Host)
	suite.Equal(10516, endpoints.Main.Port)

	suite.config.SetWithoutSource("logs_config.logs_dd_url", "azure.logs.datadoghq.co.uk:1234")
	suite.Equal("azure.logs.datadoghq.co.uk:1234", pkgconfigutils.GetMainEndpoint(suite.config, tcpEndpointPrefix, "logs_config.logs_dd_url"))
	endpoints, err = BuildEndpoints(suite.config, HTTPConnectivityFailure, "test-track", "test-proto", "test-source")
	suite.Nil(err)
	suite.Equal("azure.logs.datadoghq.co.uk", endpoints.Main.Host)
	suite.Equal(1234, endpoints.Main.Port)
}

func (suite *EndpointsTestSuite) TestBuildEndpointsShouldSucceedWithDefaultAndValidOverride() {
	var endpoints *Endpoints

	var err error
	var endpoint Endpoint

	suite.config.SetWithoutSource("api_key", "azerty")
	suite.config.SetWithoutSource("logs_config.socks5_proxy_address", "boz:1234")

	endpoints, err = BuildEndpoints(suite.config, HTTPConnectivityFailure, "test-track", "test-proto", "test-source")
	suite.Nil(err)
	endpoint = endpoints.Main
	suite.Equal("azerty", endpoint.GetAPIKey())
	suite.Equal("agent-intake.logs.datadoghq.com", endpoint.Host)
	suite.Equal(10516, endpoint.Port)
	suite.True(endpoint.UseSSL())
	suite.Equal("boz:1234", endpoint.ProxyAddress)
	suite.Equal(1, len(endpoints.Endpoints))

	suite.config.SetWithoutSource("logs_config.use_port_443", true)
	endpoints, err = BuildEndpoints(suite.config, HTTPConnectivityFailure, "test-track", "test-proto", "test-source")
	suite.Nil(err)
	endpoint = endpoints.Main
	suite.Equal("azerty", endpoint.GetAPIKey())
	suite.Equal("agent-443-intake.logs.datadoghq.com", endpoint.Host)
	suite.Equal(443, endpoint.Port)
	suite.True(endpoint.UseSSL())
	suite.Equal("boz:1234", endpoint.ProxyAddress)
	suite.Equal(1, len(endpoints.Endpoints))

	suite.config.SetWithoutSource("logs_config.logs_dd_url", "host:1234")
	suite.config.SetWithoutSource("logs_config.logs_no_ssl", true)
	endpoints, err = BuildEndpoints(suite.config, HTTPConnectivityFailure, "test-track", "test-proto", "test-source")
	suite.Nil(err)
	endpoint = endpoints.Main
	suite.Equal("azerty", endpoint.GetAPIKey())
	suite.Equal("host", endpoint.Host)
	suite.Equal(1234, endpoint.Port)
	suite.False(endpoint.UseSSL())
	suite.Equal("boz:1234", endpoint.ProxyAddress)
	suite.Equal(1, len(endpoints.Endpoints))

	suite.config.SetWithoutSource("logs_config.logs_dd_url", ":1234")
	suite.config.SetWithoutSource("logs_config.logs_no_ssl", false)
	endpoints, err = BuildEndpoints(suite.config, HTTPConnectivityFailure, "test-track", "test-proto", "test-source")
	suite.Nil(err)
	endpoint = endpoints.Main
	suite.Equal("azerty", endpoint.GetAPIKey())
	suite.Equal("", endpoint.Host)
	suite.Equal(1234, endpoint.Port)
	suite.True(endpoint.UseSSL())
	suite.Equal("boz:1234", endpoint.ProxyAddress)
	suite.Equal(1, len(endpoints.Endpoints))
}

func (suite *EndpointsTestSuite) TestBuildEndpointsShouldSucceedWithValidHTTPConfig() {
	var endpoints *Endpoints
	var endpoint Endpoint
	var err error

	suite.config.SetWithoutSource("logs_config.use_http", true)

	endpoints, err = BuildEndpoints(suite.config, HTTPConnectivityFailure, "test-track", "test-proto", "test-source")
	suite.Nil(err)
	suite.True(endpoints.UseHTTP)
	suite.Equal(endpoints.BatchWait, 5*time.Second)

	endpoint = endpoints.Main
	suite.True(endpoint.UseSSL())
	suite.Equal("agent-http-intake.logs.datadoghq.com", endpoint.Host)
}

func (suite *EndpointsTestSuite) TestBuildEndpointsShouldSucceedWithValidHTTPConfigAndCompression() {
	var endpoints *Endpoints
	var endpoint Endpoint
	var err error

	suite.config.SetWithoutSource("logs_config.use_http", true)
	suite.config.SetWithoutSource("logs_config.use_compression", true)

	endpoints, err = BuildEndpoints(suite.config, HTTPConnectivityFailure, "test-track", "test-proto", "test-source")
	suite.Nil(err)
	suite.True(endpoints.UseHTTP)

	endpoint = endpoints.Main
	suite.True(endpoint.UseCompression)
	suite.Equal(endpoint.CompressionLevel, 6)
}

func (suite *EndpointsTestSuite) TestBuildEndpointsShouldSucceedWithValidHTTPConfigAndCompressionAndOverride() {
	var endpoints *Endpoints
	var endpoint Endpoint
	var err error

	suite.config.SetWithoutSource("logs_config.use_http", true)
	suite.config.SetWithoutSource("logs_config.use_compression", true)
	suite.config.SetWithoutSource("logs_config.compression_level", 1)

	endpoints, err = BuildEndpoints(suite.config, HTTPConnectivityFailure, "test-track", "test-proto", "test-source")
	suite.Nil(err)
	suite.True(endpoints.UseHTTP)

	endpoint = endpoints.Main
	suite.True(endpoint.UseCompression)
	suite.Equal(endpoint.CompressionLevel, 1)
}

func (suite *EndpointsTestSuite) TestBuildEndpointsShouldSucceedWithValidHTTPConfigAndOverride() {
	var endpoints *Endpoints
	var endpoint Endpoint
	var err error

	suite.config.SetWithoutSource("logs_config.use_http", true)
	suite.config.SetWithoutSource("logs_config.dd_url", "foo")
	suite.config.SetWithoutSource("logs_config.batch_wait", 9)

	endpoints, err = BuildEndpoints(suite.config, HTTPConnectivityFailure, "test-track", "test-proto", "test-source")
	suite.Nil(err)
	suite.True(endpoints.UseHTTP)
	suite.Equal(endpoints.BatchWait, 9*time.Second)

	endpoint = endpoints.Main
	suite.True(endpoint.UseSSL())
	suite.Equal("foo", endpoint.Host)
}

func (suite *EndpointsTestSuite) TestBuildEndpointsShouldSucceedWithValidProxyConfig() {
	var endpoints *Endpoints
	var endpoint Endpoint
	var err error

	suite.config.SetWithoutSource("logs_config.use_http", true)
	suite.config.SetWithoutSource("logs_config.logs_dd_url", "foo:1234")

	endpoints, err = BuildEndpoints(suite.config, HTTPConnectivityFailure, "test-track", "test-proto", "test-source")
	suite.Nil(err)
	suite.True(endpoints.UseHTTP)

	endpoint = endpoints.Main
	suite.True(endpoint.UseSSL())
	suite.Equal("foo", endpoint.Host)
	suite.Equal(1234, endpoint.Port)
}

func (suite *EndpointsTestSuite) TestBuildEndpointsShouldFailWithInvalidProxyConfig() {
	var err error

	suite.config.SetWithoutSource("logs_config.use_http", true)
	suite.config.SetWithoutSource("logs_config.logs_dd_url", "foo")

	_, err = BuildEndpoints(suite.config, HTTPConnectivityFailure, "test-track", "test-proto", "test-source")
	suite.NotNil(err)
}

func (suite *EndpointsTestSuite) TestBuildEndpointsShouldFailWithInvalidOverride() {
	invalidURLs := []string{
		"host:foo",
		"host",
	}
	for _, url := range invalidURLs {
		suite.config.SetWithoutSource("logs_config.logs_dd_url", url)
		_, err := BuildEndpoints(suite.config, HTTPConnectivityFailure, "test-track", "test-proto", "test-source")
		suite.NotNil(err)
	}
}

func (suite *EndpointsTestSuite) TestBuildEndpointsShouldFallbackOnDefaultWithInvalidBatchWait() {
	suite.config.SetWithoutSource("logs_config.use_http", true)

	invalidBatchWaits := []int{-1, 0, 11}
	for _, batchWait := range invalidBatchWaits {
		suite.config.SetWithoutSource("logs_config.batch_wait", batchWait)
		endpoints, err := BuildEndpoints(suite.config, HTTPConnectivityFailure, "test-track", "test-proto", "test-source")
		suite.Nil(err)
		suite.Equal(endpoints.BatchWait, pkgconfigsetup.DefaultBatchWait*time.Second)
	}
}

// When migrating the agent v5 to v6, logs_dd_url is set to empty. Default to the dd_url/site already set instead.
func (suite *EndpointsTestSuite) TestBuildEndpointsShouldSucceedWhenMigratingToAgentV6() {
	suite.config.SetWithoutSource("logs_config.logs_dd_url", "")
	endpoints, err := BuildEndpoints(suite.config, HTTPConnectivityFailure, "test-track", "test-proto", "test-source")
	suite.Nil(err)
	suite.Equal("agent-intake.logs.datadoghq.com", endpoints.Main.Host)
	suite.Equal(10516, endpoints.Main.Port)
}

func (suite *EndpointsTestSuite) TestBuildEndpointsShouldTakeIntoAccountHTTPConnectivity() {

	resetHTTPConfigValuesToFalse := func() {
		suite.config.SetWithoutSource("logs_config.use_tcp", "false")
		suite.config.SetWithoutSource("logs_config.force_use_tcp", "false")
		suite.config.SetWithoutSource("logs_config.use_http", "false")
		suite.config.SetWithoutSource("logs_config.force_use_http", "false")
		suite.config.SetWithoutSource("logs_config.socks5_proxy_address", "")
		suite.config.SetWithoutSource("logs_config.additional_endpoints", []map[string]interface{}{})
	}

	suite.Run("When use_http is true always create HTTP endpoints", func() {
		defer resetHTTPConfigValuesToFalse()
		suite.config.SetWithoutSource("logs_config.use_http", "true")
		endpoints, err := BuildEndpoints(suite.config, HTTPConnectivitySuccess, "test-track", "test-proto", "test-source")
		suite.Nil(err)
		suite.True(endpoints.UseHTTP)
		endpoints, err = BuildEndpoints(suite.config, HTTPConnectivityFailure, "test-track", "test-proto", "test-source")
		suite.Nil(err)
		suite.True(endpoints.UseHTTP)
	})

	suite.Run("When force_use_http is true always create HTTP endpoints", func() {
		defer resetHTTPConfigValuesToFalse()
		suite.config.SetWithoutSource("logs_config.force_use_http", "true")
		endpoints, err := BuildEndpoints(suite.config, HTTPConnectivitySuccess, "test-track", "test-proto", "test-source")
		suite.Nil(err)
		suite.True(endpoints.UseHTTP)
		endpoints, err = BuildEndpoints(suite.config, HTTPConnectivityFailure, "test-track", "test-proto", "test-source")
		suite.Nil(err)
		suite.True(endpoints.UseHTTP)
	})

	suite.Run("When use_tcp is true always create TCP endpoints", func() {
		defer resetHTTPConfigValuesToFalse()
		suite.config.SetWithoutSource("logs_config.use_tcp", "true")
		endpoints, err := BuildEndpoints(suite.config, HTTPConnectivitySuccess, "test-track", "test-proto", "test-source")
		suite.Nil(err)
		suite.False(endpoints.UseHTTP)
		endpoints, err = BuildEndpoints(suite.config, HTTPConnectivityFailure, "test-track", "test-proto", "test-source")
		suite.Nil(err)
		suite.False(endpoints.UseHTTP)
	})

	suite.Run("When force_use_tcp is true always create TCP endpoints", func() {
		defer resetHTTPConfigValuesToFalse()
		suite.config.SetWithoutSource("logs_config.force_use_tcp", "true")
		endpoints, err := BuildEndpoints(suite.config, HTTPConnectivitySuccess, "test-track", "test-proto", "test-source")
		suite.Nil(err)
		suite.False(endpoints.UseHTTP)
		endpoints, err = BuildEndpoints(suite.config, HTTPConnectivityFailure, "test-track", "test-proto", "test-source")
		suite.Nil(err)
		suite.False(endpoints.UseHTTP)
	})

	suite.Run("When (force_)use_http & (force_)use_tcp are false create HTTP endpoints if HTTP connectivity is successful", func() {
		defer resetHTTPConfigValuesToFalse()
		endpoints, err := BuildEndpoints(suite.config, HTTPConnectivitySuccess, "test-track", "test-proto", "test-source")
		suite.Nil(err)
		suite.True(endpoints.UseHTTP)
		endpoints, err = BuildEndpoints(suite.config, HTTPConnectivityFailure, "test-track", "test-proto", "test-source")
		suite.Nil(err)
		suite.False(endpoints.UseHTTP)
	})

	suite.Run("When socks5_proxy_address is set always create TCP endpoints", func() {
		defer resetHTTPConfigValuesToFalse()
		suite.config.SetWithoutSource("logs_config.socks5_proxy_address", "my-address")
		endpoints, err := BuildEndpoints(suite.config, HTTPConnectivitySuccess, "test-track", "test-proto", "test-source")
		suite.Nil(err)
		suite.False(endpoints.UseHTTP)
		endpoints, err = BuildEndpoints(suite.config, HTTPConnectivityFailure, "test-track", "test-proto", "test-source")
		suite.Nil(err)
		suite.False(endpoints.UseHTTP)
		suite.config.SetWithoutSource("logs_config.socks5_proxy_address", "")
	})

	suite.Run("When additional_endpoints is not empty always create TCP endpoints", func() {
		defer resetHTTPConfigValuesToFalse()
		suite.config.SetWithoutSource("logs_config.additional_endpoints", []map[string]interface{}{
			{
				"host":              "foo",
				"api_key":           "1234",
				"use_compression":   true,
				"compression_level": 1,
			},
		})
		endpoints, err := BuildEndpoints(suite.config, HTTPConnectivitySuccess, "test-track", "test-proto", "test-source")
		suite.Nil(err)
		suite.False(endpoints.UseHTTP)
		endpoints, err = BuildEndpoints(suite.config, HTTPConnectivityFailure, "test-track", "test-proto", "test-source")
		suite.Nil(err)
		suite.False(endpoints.UseHTTP)
	})
}

func (suite *EndpointsTestSuite) TestIsSetAndNotEmpty() {
	suite.config.SetWithoutSource("bob", "vanilla")
	suite.config.SetWithoutSource("empty", "")
	suite.True(isSetAndNotEmpty(suite.config, "bob"))
	suite.False(isSetAndNotEmpty(suite.config, "empty"))
	suite.False(isSetAndNotEmpty(suite.config, "wassup"))
}

func (suite *EndpointsTestSuite) TestDefaultApiKey() {
	suite.config.SetWithoutSource("api_key", "wassupkey")
	suite.Equal("wassupkey", defaultLogsConfigKeys(suite.config).getAPIKeyGetter()())
	endpoints, err := BuildEndpoints(suite.config, HTTPConnectivityFailure, "test-track", "test-proto", "test-source")
	suite.Nil(err)
	suite.Equal("wassupkey", endpoints.Main.GetAPIKey())
}

func (suite *EndpointsTestSuite) TestOverrideApiKey() {
	suite.config.SetWithoutSource("api_key", "wassupkey")
	suite.config.SetWithoutSource("logs_config.api_key", "wassuplogskey")
	suite.Equal("wassuplogskey", defaultLogsConfigKeys(suite.config).getAPIKeyGetter()())
	endpoints, err := BuildEndpoints(suite.config, HTTPConnectivityFailure, "test-track", "test-proto", "test-source")
	suite.Nil(err)
	suite.Equal("wassuplogskey", endpoints.Main.GetAPIKey())
}

func (suite *EndpointsTestSuite) TestAdditionalEndpoints() {
	var (
		endpoints *Endpoints
		endpoint  Endpoint
		err       error
	)

	suite.config.SetWithoutSource("logs_config.additional_endpoints", []map[string]interface{}{
		{
			"host":              "foo",
			"api_key":           "1234",
			"use_compression":   false,
			"compression_level": 4,
		},
	})

	endpoints, err = BuildEndpoints(suite.config, HTTPConnectivityFailure, "test-track", "test-proto", "test-source")
	suite.Nil(err)
	suite.Len(endpoints.Endpoints, 2)

	endpoint = endpoints.Endpoints[1]
	suite.Equal("foo", endpoint.Host)
	suite.Equal("1234", endpoint.GetAPIKey())
	suite.True(endpoint.UseSSL())

	suite.config.SetWithoutSource("logs_config.use_http", true)
	endpoints, err = BuildEndpoints(suite.config, HTTPConnectivityFailure, "test-track", "test-proto", "test-source")
	suite.Nil(err)
	suite.Len(endpoints.Endpoints, 2)

	endpoint = endpoints.Endpoints[1]
	suite.Equal("foo", endpoint.Host)
	suite.Equal("1234", endpoint.GetAPIKey())

	// Main should override the compression settings
	suite.True(endpoint.UseCompression)
	suite.Equal(6, endpoint.CompressionLevel)

	suite.True(endpoint.UseSSL())
}

func (suite *EndpointsTestSuite) TestAdditionalEndpointsMappedCorrectly() {
	var (
		endpoints *Endpoints
		endpoint  Endpoint
		err       error
	)

	suite.config.SetWithoutSource("logs_config.additional_endpoints", []map[string]interface{}{
		{
			"host":        "a",
			"api_key":     "1",
			"is_reliable": false,
		},
		{
			"host":        "b",
			"api_key":     "2",
			"is_reliable": true,
		},
		{
			"host":        "c",
			"api_key":     "3",
			"is_reliable": false,
		},
	})

	endpoints, err = BuildEndpoints(suite.config, HTTPConnectivityFailure, "test-track", "test-proto", "test-source")
	suite.Nil(err)
	suite.Len(endpoints.Endpoints, 4)
	suite.Len(endpoints.GetUnReliableEndpoints(), 2)
	suite.Len(endpoints.GetReliableEndpoints(), 2)

	endpoint = endpoints.GetUnReliableEndpoints()[0]
	suite.Equal("a", endpoint.Host)
	suite.Equal("1", endpoint.GetAPIKey())

	endpoint = endpoints.GetUnReliableEndpoints()[1]
	suite.Equal("c", endpoint.Host)
	suite.Equal("3", endpoint.GetAPIKey())

	endpoint = endpoints.GetReliableEndpoints()[1]
	suite.Equal("b", endpoint.Host)
	suite.Equal("2", endpoint.GetAPIKey())
}

func (suite *EndpointsTestSuite) TestIsReliableDefaultTrue() {
	var (
		endpoints *Endpoints
		err       error
	)

	suite.config.SetWithoutSource("logs_config.additional_endpoints", []map[string]interface{}{
		{
			"host":    "a",
			"api_key": "1",
		},
		{
			"host":        "b",
			"api_key":     "2",
			"is_reliable": true,
		},
		{
			"host":        "c",
			"api_key":     "3",
			"is_reliable": false,
		},
	})

	endpoints, err = BuildEndpoints(suite.config, HTTPConnectivityFailure, "test-track", "test-proto", "test-source")
	suite.NoError(err)
	suite.Len(endpoints.Endpoints, 4)
	suite.Len(endpoints.GetUnReliableEndpoints(), 1)
	suite.Equal("c", endpoints.GetUnReliableEndpoints()[0].Host)
	suite.Len(endpoints.GetReliableEndpoints(), 3)
}

func (suite *EndpointsTestSuite) TestAdditionalEndpointsUseSSLTCPMainEndpointTrue() {
	var (
		endpoints *Endpoints
		err       error
	)

	suite.config.SetWithoutSource("logs_config.logs_no_ssl", "true")
	suite.config.SetWithoutSource("logs_config.logs_dd_url", "rand_url.com:1")

	suite.config.SetWithoutSource("logs_config.additional_endpoints", []map[string]interface{}{
		{
			"host":    "a",
			"api_key": "1",
		},
		{
			"host":    "b",
			"api_key": "2",
			"use_ssl": true,
		},
		{
			"host":    "c",
			"api_key": "3",
			"use_ssl": false,
		},
	})

	endpoints, err = BuildEndpoints(suite.config, HTTPConnectivityFailure, "test-track", "test-proto", "test-source")
	suite.Nil(err)
	suite.Len(endpoints.Endpoints, 4)
	suite.False(endpoints.Endpoints[1].UseSSL())
	suite.True(endpoints.Endpoints[2].UseSSL())
	suite.False(endpoints.Endpoints[3].UseSSL())
}

func (suite *EndpointsTestSuite) TestAdditionalEndpointsUseSSLTCPMainEndpointFalse() {
	var (
		endpoints *Endpoints
		err       error
	)

	suite.config.SetWithoutSource("logs_config.logs_no_ssl", "false")
	suite.config.SetWithoutSource("logs_config.logs_dd_url", "rand_url.com:1")

	suite.config.SetWithoutSource("logs_config.additional_endpoints", []map[string]interface{}{
		{
			"host":    "a",
			"api_key": "1",
		},
		{
			"host":    "b",
			"api_key": "2",
			"use_ssl": true,
		},
		{
			"host":    "c",
			"api_key": "3",
			"use_ssl": false,
		},
	})

	endpoints, err = BuildEndpoints(suite.config, HTTPConnectivityFailure, "test-track", "test-proto", "test-source")
	suite.Nil(err)
	suite.Len(endpoints.Endpoints, 4)
	suite.True(endpoints.Endpoints[1].UseSSL())
	suite.True(endpoints.Endpoints[2].UseSSL())
	suite.False(endpoints.Endpoints[3].UseSSL())
}

func (suite *EndpointsTestSuite) TestAdditionalEndpointsUseSSLHTTPMainEndpointTrue() {
	var (
		endpoints *Endpoints
		err       error
	)

	suite.config.SetWithoutSource("logs_config.logs_no_ssl", "true")
	suite.config.SetWithoutSource("logs_config.use_http", "true")
	suite.config.SetWithoutSource("logs_config.logs_dd_url", "http://rand_url.com:1")

	suite.config.SetWithoutSource("logs_config.additional_endpoints", []map[string]interface{}{
		{
			"host":    "a",
			"api_key": "1",
		},
		{
			"host":    "b",
			"api_key": "2",
			"use_ssl": true,
		},
		{
			"host":    "c",
			"api_key": "3",
			"use_ssl": false,
		},
	})

	endpoints, err = BuildEndpoints(suite.config, HTTPConnectivitySuccess, "test-track", "test-proto", "test-source")
	suite.Nil(err)
	suite.Len(endpoints.Endpoints, 4)
	suite.False(endpoints.Endpoints[1].UseSSL())
	suite.True(endpoints.Endpoints[2].UseSSL())
	suite.False(endpoints.Endpoints[3].UseSSL())
}

func (suite *EndpointsTestSuite) TestAdditionalEndpointsUseSSLHTTPMainEndpointFalse() {
	var (
		endpoints *Endpoints
		err       error
	)

	suite.config.SetWithoutSource("logs_config.logs_no_ssl", "false")
	suite.config.SetWithoutSource("logs_config.use_http", "true")
	suite.config.SetWithoutSource("logs_config.logs_dd_url", "http://rand_url.com:1")

	suite.config.SetWithoutSource("logs_config.additional_endpoints", []map[string]interface{}{
		{
			"host":    "a",
			"api_key": "1",
		},
		{
			"host":    "b",
			"api_key": "2",
			"use_ssl": true,
		},
		{
			"host":    "c",
			"api_key": "3",
			"use_ssl": false,
		},
	})

	endpoints, err = BuildEndpoints(suite.config, HTTPConnectivitySuccess, "test-track", "test-proto", "test-source")
	suite.Nil(err)
	suite.Len(endpoints.Endpoints, 4)
	suite.False(endpoints.Endpoints[1].UseSSL())
	suite.True(endpoints.Endpoints[2].UseSSL())
	suite.False(endpoints.Endpoints[3].UseSSL())
}

func (suite *EndpointsTestSuite) TestMainApiKeyRotation() {
	suite.config.SetWithoutSource("api_key", "1234")
	logsConfig := defaultLogsConfigKeys(suite.config)

	tcp := NewTCPEndpoint(logsConfig)
	http := NewHTTPEndpoint(logsConfig)

	suite.Equal("1234", tcp.GetAPIKey())
	suite.Equal("1234", http.GetAPIKey())

	// change API key at runtime
	suite.config.SetWithoutSource("api_key", "5678")
	suite.Equal("5678", tcp.GetAPIKey())
	suite.Equal("5678", http.GetAPIKey())
}

func (suite *EndpointsTestSuite) TestLogsConfigApiKeyRotation() {
	suite.config.SetWithoutSource("api_key", "abcd")
	suite.config.SetWithoutSource("logs_config.api_key", "1234")
	logsConfig := defaultLogsConfigKeys(suite.config)

	tcp := NewTCPEndpoint(logsConfig)
	http := NewHTTPEndpoint(logsConfig)

	suite.Equal("1234", tcp.GetAPIKey())
	suite.Equal("1234", http.GetAPIKey())

	// change API key at runtime
	suite.config.SetWithoutSource("api_key", "5678")
	suite.Equal("1234", tcp.GetAPIKey())
	suite.Equal("1234", http.GetAPIKey())

	// change API key at runtime
	suite.config.SetWithoutSource("logs_config.api_key", "5678")
	suite.Equal("5678", tcp.GetAPIKey())
	suite.Equal("5678", http.GetAPIKey())
}

func (suite *EndpointsTestSuite) TestloadTCPAdditionalEndpoints() {
	jsonString := `[{
			"api_key":           "apiKey2",
			"Host":              "localhost1",
			"Port":              1234,
			"is_reliable":       true,
			"use_compression":   true,
			"compression_level": 12
		},
		{
			"api_key":     "apiKey3",
			"Host":        "localhost2",
			"Port":        5678,
			"use_ssl":     false,
			"is_reliable": false
		}]`
	suite.config.SetWithoutSource("logs_config.additional_endpoints", jsonString)

	expected1 := Endpoint{
		apiKeyGetter:     func() string { return "apiKey2" },
		isReliable:       true,
		useSSL:           true,
		Host:             "localhost1",
		Port:             1234,
		UseCompression:   true,
		CompressionLevel: 12,
	}
	expected2 := Endpoint{
		apiKeyGetter: func() string { return "apiKey3" },
		isReliable:   false,
		Host:         "localhost2",
		Port:         5678,
	}

	main := Endpoint{useSSL: true}
	logsConfig := defaultLogsConfigKeys(suite.config)
	endpoints := loadTCPAdditionalEndpoints(main, logsConfig)

	suite.Suite.Require().Len(endpoints, 2)
	compareEndpoint(suite.T(), expected1, endpoints[0])
	compareEndpoint(suite.T(), expected2, endpoints[1])
}

func (suite *EndpointsTestSuite) TestloadHTTPAdditionalEndpoints() {
	jsonString := `[{
			"api_key":           "apiKey2",
			"Host":              "localhost1",
			"Port":              1234,
			"is_reliable":       true,
			"version":           123
		},
		{
			"api_key":     "apiKey3",
			"Host":        "localhost2",
			"Port":        5678,
			"use_ssl":     false,
			"is_reliable": false
		}]`
	suite.config.SetWithoutSource("logs_config.additional_endpoints", jsonString)

	expected1 := Endpoint{
		apiKeyGetter:     func() string { return "apiKey2" },
		isReliable:       true,
		useSSL:           true,
		Host:             "localhost1",
		Port:             1234,
		UseCompression:   true, // compression from main overwrite the config
		CompressionLevel: 123,
		Version:          123,
	}
	expected2 := Endpoint{
		apiKeyGetter:     func() string { return "apiKey3" },
		isReliable:       false,
		Host:             "localhost2",
		Port:             5678,
		UseCompression:   true,
		CompressionLevel: 123,
		Version:          EPIntakeVersion2,
		// Those are enforce when EPIntakeVersion2 is used
		TrackType: "some track type",
		Protocol:  "some intake protocol",
		Origin:    "some intake origin",
	}

	main := Endpoint{
		useSSL:           true,
		UseCompression:   true,
		CompressionLevel: 123,
		Version:          EPIntakeVersion2,
	}

	logsConfig := defaultLogsConfigKeys(suite.config)
	endpoints := loadHTTPAdditionalEndpoints(
		main,
		logsConfig,
		"some track type",
		"some intake protocol",
		"some intake origin",
	)

	suite.Suite.Require().Len(endpoints, 2)
	compareEndpoint(suite.T(), expected1, endpoints[0])
	compareEndpoint(suite.T(), expected2, endpoints[1])
}

func TestEndpointsTestSuite(t *testing.T) {
	suite.Run(t, new(EndpointsTestSuite))
}
