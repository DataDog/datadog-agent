// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package config

import (
	"runtime"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/suite"
	"go.uber.org/atomic"

	"github.com/DataDog/datadog-agent/comp/core/config"
	pkgconfigsetup "github.com/DataDog/datadog-agent/pkg/config/setup"
)

type ConfigTestSuite struct {
	suite.Suite
	config config.Component
}

func (suite *ConfigTestSuite) SetupTest() {
	suite.config = config.NewMock(suite.T())
}

func (suite *ConfigTestSuite) TestDefaultDatadogConfig() {
	suite.Equal(false, suite.config.GetBool("log_enabled"))
	suite.Equal(false, suite.config.GetBool("logs_enabled"))
	suite.Equal("", suite.config.GetString("logs_config.dd_url"))
	suite.Equal(10516, suite.config.GetInt("logs_config.dd_port"))
	suite.Equal(false, suite.config.GetBool("logs_config.dev_mode_no_ssl"))
	suite.Equal("agent-443-intake.logs.datadoghq.com", suite.config.GetString("logs_config.dd_url_443"))
	suite.Equal(false, suite.config.GetBool("logs_config.use_port_443"))
	suite.Equal(true, suite.config.GetBool("logs_config.dev_mode_use_proto"))
	if runtime.GOOS == "darwin" {
		suite.Equal(200, suite.config.GetInt("logs_config.open_files_limit"))
	} else {
		suite.Equal(500, suite.config.GetInt("logs_config.open_files_limit"))
	}
	suite.Equal(9000, suite.config.GetInt("logs_config.frame_size"))
	suite.Equal("", suite.config.GetString("logs_config.socks5_proxy_address"))
	suite.Equal("", suite.config.GetString("logs_config.logs_dd_url"))
	suite.Equal(false, suite.config.GetBool("logs_config.logs_no_ssl"))
	suite.Equal(30, suite.config.GetInt("logs_config.stop_grace_period"))
	suite.Equal(nil, suite.config.Get("logs_config.processing_rules"))
	suite.Equal("", suite.config.GetString("logs_config.processing_rules"))
	suite.Equal(false, suite.config.GetBool("logs_config.use_tcp"))
	suite.Equal(false, suite.config.GetBool("logs_config.force_use_tcp"))
	suite.Equal(false, suite.config.GetBool("logs_config.use_http"))
	suite.Equal(false, suite.config.GetBool("logs_config.force_use_http"))
	suite.Equal(false, suite.config.GetBool("logs_config.k8s_container_use_file"))
	suite.Equal(true, suite.config.GetBool("logs_config.use_v2_api"))
}

func compareEndpoint(t *testing.T, expected Endpoint, actual Endpoint) {
	assert.Equal(t, expected.GetAPIKey(), actual.GetAPIKey(), "GetAPIKey() is not Equal")
	assert.Equal(t, expected.IsReliable(), actual.IsReliable(), "IsReliable() is not Equal")
	assert.Equal(t, expected.UseSSL(), actual.UseSSL(), "UseSSL() is not Equal")
	assert.Equal(t, expected.Host, actual.Host, "Host is not Equal")
	assert.Equal(t, expected.Port, actual.Port, "Port is not Equal")
	assert.Equal(t, expected.UseCompression, actual.UseCompression, "UseCompression is not Equal")
	assert.Equal(t, expected.CompressionLevel, actual.CompressionLevel, "CompressionLevel is not Equal")
	assert.Equal(t, expected.ProxyAddress, actual.ProxyAddress, "ProxyAddress is not Equal")
	assert.Equal(t, expected.ConnectionResetInterval, actual.ConnectionResetInterval, "ConnectionResetInterval is not Equal")
	assert.Equal(t, expected.BackoffFactor, actual.BackoffFactor, "BackoffFactor is not Equal")
	assert.Equal(t, expected.BackoffBase, actual.BackoffBase, "BackoffBase is not Equal")
	assert.Equal(t, expected.BackoffMax, actual.BackoffMax, "BackoffMax is not Equal")
	assert.Equal(t, expected.RecoveryInterval, actual.RecoveryInterval, "RecoveryInterval is not Equal")
	assert.Equal(t, expected.RecoveryReset, actual.RecoveryReset, "RecoveryReset is not Equal")
	assert.Equal(t, expected.Version, actual.Version, "Version is not Equal")
	assert.Equal(t, expected.TrackType, actual.TrackType, "TrackType is not Equal")
	assert.Equal(t, expected.Protocol, actual.Protocol, "Protocol is not Equal")
	assert.Equal(t, expected.Origin, actual.Origin, "Origin is not Equal")
}

func (suite *ConfigTestSuite) compareEndpoints(expected *Endpoints, actual *Endpoints) {
	compareEndpoint(suite.T(), expected.Main, actual.Main)

	suite.Require().Equal(len(expected.Endpoints), len(actual.Endpoints), "Endpoint list as a different size than expected")
	for idx := range expected.Endpoints {
		compareEndpoint(suite.T(), expected.Endpoints[idx], actual.Endpoints[idx])
	}

	suite.Equal(expected.UseProto, actual.UseProto, "UseProto is not Equal")
	suite.Equal(expected.UseHTTP, actual.UseHTTP, "UseHTTP is not Equal")
	suite.Equal(expected.BatchWait, actual.BatchWait, "BatchWait is not Equal")
	suite.Equal(expected.BatchMaxConcurrentSend, actual.BatchMaxConcurrentSend, "BatchMaxConcurrentSend is not Equal")
	suite.Equal(expected.BatchMaxSize, actual.BatchMaxSize, "BatchMaxSize is not Equal")
	suite.Equal(expected.BatchMaxContentSize, actual.BatchMaxContentSize, "BatchMaxContentSize is not Equal")
	suite.Equal(expected.InputChanSize, actual.InputChanSize, "InputChanSize is not Equal")
}

func (suite *ConfigTestSuite) TestGlobalProcessingRulesShouldReturnNoRulesWithEmptyValues() {
	var (
		rules []*ProcessingRule
		err   error
	)

	suite.config.SetWithoutSource("logs_config.processing_rules", nil)

	rules, err = GlobalProcessingRules(suite.config)
	suite.Nil(err)
	suite.Equal(0, len(rules))

	suite.config.SetWithoutSource("logs_config.processing_rules", "")

	rules, err = GlobalProcessingRules(suite.config)
	suite.Nil(err)
	suite.Equal(0, len(rules))
}

func (suite *ConfigTestSuite) TestGlobalProcessingRulesShouldReturnRulesWithValidMap() {
	var (
		rules []*ProcessingRule
		rule  *ProcessingRule
		err   error
	)

	suite.config.SetWithoutSource("logs_config.processing_rules", []map[string]interface{}{
		{
			"type":    "exclude_at_match",
			"name":    "exclude_foo",
			"pattern": "foo",
		},
	})

	rules, err = GlobalProcessingRules(suite.config)
	suite.Nil(err)
	suite.Equal(1, len(rules))

	rule = rules[0]
	suite.Equal(ExcludeAtMatch, rule.Type)
	suite.Equal("exclude_foo", rule.Name)
	suite.Equal("foo", rule.Pattern)
	suite.NotNil(rule.Regex)
}

func (suite *ConfigTestSuite) TestGlobalProcessingRulesShouldReturnRulesWithValidJSONString() {
	var (
		rules []*ProcessingRule
		rule  *ProcessingRule
		err   error
	)

	suite.config.SetWithoutSource("logs_config.processing_rules", `[{"type":"mask_sequences","name":"mask_api_keys","replace_placeholder":"****************************","pattern":"([A-Fa-f0-9]{28})"}]`)

	rules, err = GlobalProcessingRules(suite.config)
	suite.Nil(err)
	suite.Equal(1, len(rules))

	rule = rules[0]
	suite.Equal(MaskSequences, rule.Type)
	suite.Equal("mask_api_keys", rule.Name)
	suite.Equal("([A-Fa-f0-9]{28})", rule.Pattern)
	suite.Equal("****************************", rule.ReplacePlaceholder)
	suite.NotNil(rule.Regex)
}

func (suite *ConfigTestSuite) TestTaggerWarmupDuration() {
	// assert TaggerWarmupDuration is disabled by default
	taggerWarmupDuration := TaggerWarmupDuration(suite.config)
	suite.Equal(0*time.Second, taggerWarmupDuration)

	// override
	suite.config.SetWithoutSource("logs_config.tagger_warmup_duration", 5)
	taggerWarmupDuration = TaggerWarmupDuration(suite.config)
	suite.Equal(5*time.Second, taggerWarmupDuration)
}

func TestConfigTestSuite(t *testing.T) {
	suite.Run(t, new(ConfigTestSuite))
}

func (suite *ConfigTestSuite) TestMultipleHttpEndpointsEnvVar() {
	// Set like an env var
	suite.config.SetWithoutSource("logs_config.additional_endpoints", `[
		{"api_key": "456", "host": "additional.endpoint.1", "port": 1234, "use_compression": true, "compression_level": 2},
		{"api_key": "789", "host": "additional.endpoint.2", "port": 1234, "use_compression": true, "compression_level": 2}]`)

	suite.config.SetWithoutSource("api_key", "123")
	suite.config.SetWithoutSource("logs_config.batch_wait", 1)
	suite.config.SetWithoutSource("logs_config.logs_dd_url", "agent-http-intake.logs.datadoghq.com:443")
	suite.config.SetWithoutSource("logs_config.use_compression", true)
	suite.config.SetWithoutSource("logs_config.compression_level", 6)
	suite.config.SetWithoutSource("logs_config.logs_no_ssl", false)
	suite.config.SetWithoutSource("logs_config.sender_backoff_factor", 3.0)
	suite.config.SetWithoutSource("logs_config.sender_backoff_base", 1.0)
	suite.config.SetWithoutSource("logs_config.sender_backoff_max", 2.0)
	suite.config.SetWithoutSource("logs_config.sender_recovery_interval", 10)
	suite.config.SetWithoutSource("logs_config.sender_recovery_reset", true)
	suite.config.SetWithoutSource("logs_config.use_v2_api", false)

	expectedMainEndpoint := Endpoint{
		apiKey:                 atomic.NewString("123"),
		configSettingPath:      "api_key",
		isAdditionalEndpoint:   false,
		additionalEndpointsIdx: 0,
		Host:                   "agent-http-intake.logs.datadoghq.com",
		Port:                   443,
		useSSL:                 true,
		UseCompression:         true,
		CompressionLevel:       6,
		BackoffFactor:          3,
		BackoffBase:            1.0,
		BackoffMax:             2.0,
		RecoveryInterval:       10,
		RecoveryReset:          true,
		Version:                EPIntakeVersion1,
		isReliable:             true,
	}
	expectedAdditionalEndpoint1 := Endpoint{
		apiKey:                 atomic.NewString("456"),
		configSettingPath:      "logs_config.additional_endpoints",
		isAdditionalEndpoint:   true,
		additionalEndpointsIdx: 0,
		Host:                   "additional.endpoint.1",
		Port:                   1234,
		useSSL:                 true,
		UseCompression:         true,
		CompressionLevel:       6,
		BackoffFactor:          3,
		BackoffBase:            1.0,
		BackoffMax:             2.0,
		RecoveryInterval:       10,
		RecoveryReset:          true,
		Version:                EPIntakeVersion1,
		isReliable:             true,
	}
	expectedAdditionalEndpoint2 := Endpoint{
		apiKey:                 atomic.NewString("789"),
		configSettingPath:      "logs_config.additional_endpoints",
		isAdditionalEndpoint:   true,
		additionalEndpointsIdx: 1,
		Host:                   "additional.endpoint.2",
		Port:                   1234,
		useSSL:                 true,
		UseCompression:         true,
		CompressionLevel:       6,
		BackoffFactor:          3,
		BackoffBase:            1.0,
		BackoffMax:             2.0,
		RecoveryInterval:       10,
		RecoveryReset:          true,
		Version:                EPIntakeVersion1,
		isReliable:             true,
	}

	expectedEndpoints := NewEndpointsWithBatchSettings(expectedMainEndpoint, []Endpoint{expectedAdditionalEndpoint1, expectedAdditionalEndpoint2}, false, true, 1*time.Second, pkgconfigsetup.DefaultBatchMaxConcurrentSend, pkgconfigsetup.DefaultBatchMaxSize, pkgconfigsetup.DefaultBatchMaxContentSize, pkgconfigsetup.DefaultInputChanSize)
	endpoints, err := BuildHTTPEndpoints(suite.config, "test-track", "test-proto", "test-source")

	suite.Nil(err)
	suite.compareEndpoints(expectedEndpoints, endpoints)
}

func (suite *ConfigTestSuite) TestMultipleTCPEndpointsEnvVar() {
	suite.config.SetWithoutSource("logs_config.additional_endpoints", `[{"api_key": "456      \n", "host": "additional.endpoint", "port": 1234}]`)

	suite.config.SetWithoutSource("api_key", "123")
	suite.config.SetWithoutSource("logs_config.logs_dd_url", "agent-http-intake.logs.datadoghq.com:443")
	suite.config.SetWithoutSource("logs_config.logs_no_ssl", false)
	suite.config.SetWithoutSource("logs_config.socks5_proxy_address", "proxy.test:3128")
	suite.config.SetWithoutSource("logs_config.dev_mode_use_proto", true)

	expectedMainEndpoint := Endpoint{
		apiKey:                 atomic.NewString("123"),
		configSettingPath:      "api_key",
		isAdditionalEndpoint:   false,
		additionalEndpointsIdx: 0,
		Host:                   "agent-http-intake.logs.datadoghq.com",
		Port:                   443,
		useSSL:                 true,
		UseCompression:         false,
		CompressionLevel:       0,
		ProxyAddress:           "proxy.test:3128",
		isReliable:             true,
	}
	expectedAdditionalEndpoint := Endpoint{
		apiKey:                 atomic.NewString("456"),
		configSettingPath:      "logs_config.additional_endpoints",
		isAdditionalEndpoint:   true,
		additionalEndpointsIdx: 0,
		Host:                   "additional.endpoint",
		Port:                   1234,
		useSSL:                 true,
		UseCompression:         false,
		CompressionLevel:       0,
		ProxyAddress:           "proxy.test:3128",
		isReliable:             true,
	}

	expectedEndpoints := NewEndpoints(expectedMainEndpoint, []Endpoint{expectedAdditionalEndpoint}, true, false)
	endpoints, err := buildTCPEndpoints(suite.config, defaultLogsConfigKeys(suite.config))

	suite.Nil(err)
	suite.compareEndpoints(expectedEndpoints, endpoints)
}

func (suite *ConfigTestSuite) TestMultipleHttpEndpointsInConfig() {
	suite.config.SetWithoutSource("api_key", "123")
	suite.config.SetWithoutSource("logs_config.batch_wait", 1)
	suite.config.SetWithoutSource("logs_config.logs_dd_url", "agent-http-intake.logs.datadoghq.com:443")
	suite.config.SetWithoutSource("logs_config.use_compression", true)
	suite.config.SetWithoutSource("logs_config.compression_level", 6)
	suite.config.SetWithoutSource("logs_config.logs_no_ssl", false)
	suite.config.SetWithoutSource("logs_config.use_v2_api", false)

	endpointsInConfig := []map[string]interface{}{
		{
			"api_key":           "456     \n\n",
			"host":              "additional.endpoint.1",
			"port":              1234,
			"use_compression":   true,
			"compression_level": 2},
		{
			"api_key":           "789",
			"host":              "additional.endpoint.2",
			"port":              1234,
			"use_compression":   true,
			"compression_level": 2},
	}
	suite.config.SetWithoutSource("logs_config.additional_endpoints", endpointsInConfig)

	expectedMainEndpoint := Endpoint{
		apiKey:                 atomic.NewString("123"),
		configSettingPath:      "api_key",
		isAdditionalEndpoint:   false,
		additionalEndpointsIdx: 0,
		Host:                   "agent-http-intake.logs.datadoghq.com",
		Port:                   443,
		useSSL:                 true,
		UseCompression:         true,
		CompressionLevel:       6,
		BackoffFactor:          pkgconfigsetup.DefaultLogsSenderBackoffFactor,
		BackoffBase:            pkgconfigsetup.DefaultLogsSenderBackoffBase,
		BackoffMax:             pkgconfigsetup.DefaultLogsSenderBackoffMax,
		RecoveryInterval:       pkgconfigsetup.DefaultLogsSenderBackoffRecoveryInterval,
		Version:                EPIntakeVersion1,
		isReliable:             true,
	}
	expectedAdditionalEndpoint1 := Endpoint{
		apiKey:                 atomic.NewString("456"),
		configSettingPath:      "logs_config.additional_endpoints",
		isAdditionalEndpoint:   true,
		additionalEndpointsIdx: 0,
		Host:                   "additional.endpoint.1",
		Port:                   1234,
		useSSL:                 true,
		UseCompression:         true,
		CompressionLevel:       6,
		BackoffFactor:          pkgconfigsetup.DefaultLogsSenderBackoffFactor,
		BackoffBase:            pkgconfigsetup.DefaultLogsSenderBackoffBase,
		BackoffMax:             pkgconfigsetup.DefaultLogsSenderBackoffMax,
		RecoveryInterval:       pkgconfigsetup.DefaultLogsSenderBackoffRecoveryInterval,
		Version:                EPIntakeVersion1,
		isReliable:             true,
	}
	expectedAdditionalEndpoint2 := Endpoint{
		apiKey:                 atomic.NewString("789"),
		configSettingPath:      "logs_config.additional_endpoints",
		isAdditionalEndpoint:   true,
		additionalEndpointsIdx: 1,
		Host:                   "additional.endpoint.2",
		Port:                   1234,
		useSSL:                 true,
		UseCompression:         true,
		CompressionLevel:       6,
		BackoffFactor:          pkgconfigsetup.DefaultLogsSenderBackoffFactor,
		BackoffBase:            pkgconfigsetup.DefaultLogsSenderBackoffBase,
		BackoffMax:             pkgconfigsetup.DefaultLogsSenderBackoffMax,
		RecoveryInterval:       pkgconfigsetup.DefaultLogsSenderBackoffRecoveryInterval,
		Version:                EPIntakeVersion1,
		isReliable:             true,
	}

	expectedEndpoints := NewEndpointsWithBatchSettings(expectedMainEndpoint, []Endpoint{expectedAdditionalEndpoint1, expectedAdditionalEndpoint2}, false, true, 1*time.Second, pkgconfigsetup.DefaultBatchMaxConcurrentSend, pkgconfigsetup.DefaultBatchMaxSize, pkgconfigsetup.DefaultBatchMaxContentSize, pkgconfigsetup.DefaultInputChanSize)
	endpoints, err := BuildHTTPEndpoints(suite.config, "test-track", "test-proto", "test-source")

	suite.Nil(err)
	suite.compareEndpoints(expectedEndpoints, endpoints)
}

func (suite *ConfigTestSuite) TestMultipleHttpEndpointsInConfig2() {
	suite.config.SetWithoutSource("api_key", "123")
	suite.config.SetWithoutSource("logs_config.batch_wait", 1)
	suite.config.SetWithoutSource("logs_config.logs_dd_url", "agent-http-intake.logs.datadoghq.com:443")
	suite.config.SetWithoutSource("logs_config.use_compression", true)
	suite.config.SetWithoutSource("logs_config.compression_level", 6)
	suite.config.SetWithoutSource("logs_config.logs_no_ssl", false)
	endpointsInConfig := []map[string]interface{}{
		{
			"api_key":           "456     \n\n",
			"host":              "additional.endpoint.1",
			"port":              1234,
			"use_compression":   true,
			"compression_level": 2,
			"version":           1,
		},
		{
			"api_key":           "789",
			"host":              "additional.endpoint.2",
			"port":              1234,
			"use_compression":   true,
			"compression_level": 2},
	}
	suite.config.SetWithoutSource("logs_config.additional_endpoints", endpointsInConfig)

	expectedMainEndpoint := Endpoint{
		apiKey:                 atomic.NewString("123"),
		configSettingPath:      "api_key",
		isAdditionalEndpoint:   false,
		additionalEndpointsIdx: 0,
		Host:                   "agent-http-intake.logs.datadoghq.com",
		Port:                   443,
		useSSL:                 true,
		UseCompression:         true,
		CompressionLevel:       6,
		BackoffFactor:          pkgconfigsetup.DefaultLogsSenderBackoffFactor,
		BackoffBase:            pkgconfigsetup.DefaultLogsSenderBackoffBase,
		BackoffMax:             pkgconfigsetup.DefaultLogsSenderBackoffMax,
		RecoveryInterval:       pkgconfigsetup.DefaultLogsSenderBackoffRecoveryInterval,
		Version:                EPIntakeVersion2,
		TrackType:              "test-track",
		Protocol:               "test-proto",
		Origin:                 "test-source",
		isReliable:             true,
	}
	expectedAdditionalEndpoint1 := Endpoint{
		apiKey:                 atomic.NewString("456"),
		configSettingPath:      "logs_config.additional_endpoints",
		isAdditionalEndpoint:   true,
		additionalEndpointsIdx: 0,
		Host:                   "additional.endpoint.1",
		Port:                   1234,
		useSSL:                 true,
		UseCompression:         true,
		CompressionLevel:       6,
		BackoffFactor:          pkgconfigsetup.DefaultLogsSenderBackoffFactor,
		BackoffBase:            pkgconfigsetup.DefaultLogsSenderBackoffBase,
		BackoffMax:             pkgconfigsetup.DefaultLogsSenderBackoffMax,
		RecoveryInterval:       pkgconfigsetup.DefaultLogsSenderBackoffRecoveryInterval,
		Version:                EPIntakeVersion1,
		isReliable:             true,
	}
	expectedAdditionalEndpoint2 := Endpoint{
		apiKey:                 atomic.NewString("789"),
		configSettingPath:      "logs_config.additional_endpoints",
		isAdditionalEndpoint:   true,
		additionalEndpointsIdx: 1,
		Host:                   "additional.endpoint.2",
		Port:                   1234,
		useSSL:                 true,
		UseCompression:         true,
		CompressionLevel:       6,
		BackoffFactor:          pkgconfigsetup.DefaultLogsSenderBackoffFactor,
		BackoffBase:            pkgconfigsetup.DefaultLogsSenderBackoffBase,
		BackoffMax:             pkgconfigsetup.DefaultLogsSenderBackoffMax,
		RecoveryInterval:       pkgconfigsetup.DefaultLogsSenderBackoffRecoveryInterval,
		Version:                EPIntakeVersion2,
		TrackType:              "test-track",
		Protocol:               "test-proto",
		Origin:                 "test-source",
		isReliable:             true,
	}

	expectedEndpoints := NewEndpointsWithBatchSettings(expectedMainEndpoint, []Endpoint{expectedAdditionalEndpoint1, expectedAdditionalEndpoint2}, false, true, 1*time.Second, pkgconfigsetup.DefaultBatchMaxConcurrentSend, pkgconfigsetup.DefaultBatchMaxSize, pkgconfigsetup.DefaultBatchMaxContentSize, pkgconfigsetup.DefaultInputChanSize)
	endpoints, err := BuildHTTPEndpoints(suite.config, "test-track", "test-proto", "test-source")

	suite.Nil(err)
	suite.compareEndpoints(expectedEndpoints, endpoints)
}

func (suite *ConfigTestSuite) TestMultipleTCPEndpointsInConf() {
	suite.config.SetWithoutSource("api_key", "123")
	suite.config.SetWithoutSource("logs_config.logs_dd_url", "agent-http-intake.logs.datadoghq.com:443")
	suite.config.SetWithoutSource("logs_config.logs_no_ssl", false)
	suite.config.SetWithoutSource("logs_config.socks5_proxy_address", "proxy.test:3128")
	suite.config.SetWithoutSource("logs_config.dev_mode_use_proto", true)
	suite.config.SetWithoutSource("logs_config.dev_mode_use_proto", true)
	endpointsInConfig := []map[string]interface{}{
		{
			"api_key": "456",
			"host":    "additional.endpoint",
			"port":    1234},
	}
	suite.config.SetWithoutSource("logs_config.additional_endpoints", endpointsInConfig)

	expectedMainEndpoint := Endpoint{
		apiKey:                 atomic.NewString("123"),
		configSettingPath:      "api_key",
		isAdditionalEndpoint:   false,
		additionalEndpointsIdx: 0,
		Host:                   "agent-http-intake.logs.datadoghq.com",
		Port:                   443,
		useSSL:                 true,
		UseCompression:         false,
		CompressionLevel:       0,
		ProxyAddress:           "proxy.test:3128",
		isReliable:             true,
	}
	expectedAdditionalEndpoint := Endpoint{
		apiKey:                 atomic.NewString("456"),
		configSettingPath:      "logs_config.additional_endpoints",
		isAdditionalEndpoint:   true,
		additionalEndpointsIdx: 0,
		Host:                   "additional.endpoint",
		Port:                   1234,
		useSSL:                 true,
		UseCompression:         false,
		CompressionLevel:       0,
		ProxyAddress:           "proxy.test:3128",
		isReliable:             true,
	}

	expectedEndpoints := NewEndpoints(expectedMainEndpoint, []Endpoint{expectedAdditionalEndpoint}, true, false)
	endpoints, err := buildTCPEndpoints(suite.config, defaultLogsConfigKeys(suite.config))

	suite.Nil(err)
	suite.compareEndpoints(expectedEndpoints, endpoints)
}

func (suite *ConfigTestSuite) TestEndpointsSetLogsDDUrl() {
	expectedHost := "my-proxy"
	expectedPort := 8888

	setupAndBuildEndpoints := func(url string, connectivity HTTPConnectivity) (*Endpoints, error) {
		suite.config.SetWithoutSource("api_key", "123")
		suite.config.SetWithoutSource("compliance_config.endpoints.logs_dd_url", url)
		logsConfig := NewLogsConfigKeys("compliance_config.endpoints.", suite.config)

		return BuildEndpointsWithConfig(suite.config, logsConfig, "default-intake.mydomain.", connectivity, "test-track", "test-proto", "test-source")
	}

	testCases := []struct {
		name         string
		ddURL        string
		connectivity HTTPConnectivity
		useSSL       bool
		useHTTP      bool
	}{
		{
			name:         "basic host:port format",
			ddURL:        "my-proxy:8888",
			connectivity: HTTPConnectivitySuccess,
			useSSL:       true,
			useHTTP:      true,
		},
		{
			name:         "http scheme with path",
			ddURL:        "http://my-proxy:8888/logs/intake",
			connectivity: HTTPConnectivitySuccess,
			useSSL:       false,
			useHTTP:      true,
		},
		{
			name:         "https scheme with path",
			ddURL:        "https://my-proxy:8888/logs/intake",
			connectivity: HTTPConnectivitySuccess,
			useSSL:       true,
			useHTTP:      true,
		},
		{
			name:         "basic host:port format with connectivity failure",
			ddURL:        "my-proxy:8888",
			connectivity: HTTPConnectivityFailure,
			useSSL:       true,
			useHTTP:      false,
		},
		{
			name:         "http scheme with connectivity failure",
			ddURL:        "http://my-proxy:8888",
			connectivity: HTTPConnectivityFailure,
			useSSL:       false,
			useHTTP:      true,
		},
		{
			name:         "https scheme with connectivity failure",
			ddURL:        "https://my-proxy:8888",
			connectivity: HTTPConnectivityFailure,
			useSSL:       true,
			useHTTP:      true,
		},
	}

	for _, testCase := range testCases {
		suite.Run(testCase.name, func() {
			endpoints, err := setupAndBuildEndpoints(testCase.ddURL, testCase.connectivity)
			suite.Nil(err)
			suite.Equal(testCase.useHTTP, endpoints.UseHTTP)
			suite.Equal(expectedHost, endpoints.Main.Host)
			suite.Equal(expectedPort, endpoints.Main.Port)
			suite.Equal(testCase.useSSL, endpoints.Main.useSSL)
		})
	}
}

func (suite *ConfigTestSuite) TestEndpointsSetDDSite() {
	suite.config.SetWithoutSource("api_key", "123")

	suite.config.SetWithoutSource("site", "mydomain.com")
	suite.config.SetWithoutSource("compliance_config.endpoints_batch_wait", "mydomain.com")
	suite.config.SetWithoutSource("compliance_config.endpoints.batch_wait", "10")

	logsConfig := NewLogsConfigKeys("compliance_config.endpoints.", suite.config)
	endpoints, err := BuildHTTPEndpointsWithConfig(suite.config, logsConfig, "default-intake.logs.", "test-track", "test-proto", "test-source")

	suite.Nil(err)

	main := Endpoint{
		apiKey:                 atomic.NewString("123"),
		configSettingPath:      "api_key",
		isAdditionalEndpoint:   false,
		additionalEndpointsIdx: 0,
		Host:                   "default-intake.logs.mydomain.com",
		Port:                   0,
		useSSL:                 true,
		UseCompression:         true,
		CompressionLevel:       ZstdCompressionLevel,
		BackoffFactor:          pkgconfigsetup.DefaultLogsSenderBackoffFactor,
		BackoffBase:            pkgconfigsetup.DefaultLogsSenderBackoffBase,
		BackoffMax:             pkgconfigsetup.DefaultLogsSenderBackoffMax,
		RecoveryInterval:       pkgconfigsetup.DefaultLogsSenderBackoffRecoveryInterval,
		Version:                EPIntakeVersion2,
		TrackType:              "test-track",
		Origin:                 "test-source",
		Protocol:               "test-proto",
		isReliable:             true,
	}

	expectedEndpoints := &Endpoints{
		UseHTTP:                true,
		BatchWait:              10 * time.Second,
		Main:                   main,
		Endpoints:              []Endpoint{main},
		BatchMaxSize:           pkgconfigsetup.DefaultBatchMaxSize,
		BatchMaxContentSize:    pkgconfigsetup.DefaultBatchMaxContentSize,
		BatchMaxConcurrentSend: pkgconfigsetup.DefaultBatchMaxConcurrentSend,
		InputChanSize:          pkgconfigsetup.DefaultInputChanSize,
	}

	suite.Nil(err)
	suite.compareEndpoints(expectedEndpoints, endpoints)
}

func (suite *ConfigTestSuite) TestBuildServerlessEndpoints() {
	suite.config.SetWithoutSource("api_key", "123")
	suite.config.SetWithoutSource("logs_config.batch_wait", 1)

	main := Endpoint{
		apiKey:                 atomic.NewString("123"),
		configSettingPath:      "api_key",
		isAdditionalEndpoint:   false,
		additionalEndpointsIdx: 0,
		Host:                   "http-intake.logs.datadoghq.com.",
		Port:                   0,
		useSSL:                 true,
		UseCompression:         true,
		CompressionKind:        GzipCompressionKind,
		CompressionLevel:       GzipCompressionLevel,
		BackoffFactor:          pkgconfigsetup.DefaultLogsSenderBackoffFactor,
		BackoffBase:            pkgconfigsetup.DefaultLogsSenderBackoffBase,
		BackoffMax:             pkgconfigsetup.DefaultLogsSenderBackoffMax,
		RecoveryInterval:       pkgconfigsetup.DefaultLogsSenderBackoffRecoveryInterval,
		Version:                EPIntakeVersion2,
		TrackType:              "test-track",
		Origin:                 "lambda-extension",
		Protocol:               "test-proto",
		isReliable:             true,
	}

	expectedEndpoints := &Endpoints{
		UseHTTP:                true,
		BatchWait:              1 * time.Second,
		Main:                   main,
		Endpoints:              []Endpoint{main},
		BatchMaxSize:           pkgconfigsetup.DefaultBatchMaxSize,
		BatchMaxContentSize:    pkgconfigsetup.DefaultBatchMaxContentSize,
		BatchMaxConcurrentSend: pkgconfigsetup.DefaultBatchMaxConcurrentSend,
		InputChanSize:          pkgconfigsetup.DefaultInputChanSize,
	}

	endpoints, err := BuildServerlessEndpoints(suite.config, "test-track", "test-proto")

	suite.Nil(err)
	suite.compareEndpoints(expectedEndpoints, endpoints)
}

func getTestEndpoint(host string, port int, ssl bool) Endpoint {
	e := NewEndpoint("123", "", host, port, ssl)
	e.UseCompression = true
	e.CompressionLevel = ZstdCompressionLevel // by default endpoints uses zstd
	e.BackoffFactor = pkgconfigsetup.DefaultLogsSenderBackoffFactor
	e.BackoffBase = pkgconfigsetup.DefaultLogsSenderBackoffBase
	e.BackoffMax = pkgconfigsetup.DefaultLogsSenderBackoffMax
	e.RecoveryInterval = pkgconfigsetup.DefaultLogsSenderBackoffRecoveryInterval
	e.Version = EPIntakeVersion2
	e.TrackType = "test-track"
	e.Protocol = "test-proto"
	e.Origin = "test-source"
	return e
}

func getTestEndpoints(e Endpoint) *Endpoints {
	return &Endpoints{
		UseHTTP:                true,
		BatchWait:              pkgconfigsetup.DefaultBatchWait * time.Second,
		Main:                   e,
		Endpoints:              []Endpoint{e},
		BatchMaxSize:           pkgconfigsetup.DefaultBatchMaxSize,
		BatchMaxContentSize:    pkgconfigsetup.DefaultBatchMaxContentSize,
		BatchMaxConcurrentSend: pkgconfigsetup.DefaultBatchMaxConcurrentSend,
		InputChanSize:          pkgconfigsetup.DefaultInputChanSize,
	}
}
func (suite *ConfigTestSuite) TestBuildEndpointsWithVectorHttpOverride() {
	suite.config.SetWithoutSource("api_key", "123")
	suite.config.SetWithoutSource("observability_pipelines_worker.logs.enabled", true)
	suite.config.SetWithoutSource("observability_pipelines_worker.logs.url", "http://vector.host:8080/")
	endpoints, err := BuildHTTPEndpointsWithVectorOverride(suite.config, "test-track", "test-proto", "test-source")
	suite.Nil(err)
	expectedEndpoints := getTestEndpoints(getTestEndpoint("vector.host", 8080, false))
	suite.Nil(err)
	suite.compareEndpoints(expectedEndpoints, endpoints)
}

func (suite *ConfigTestSuite) TestBuildEndpointsWithVectorHttpsOverride() {
	suite.config.SetWithoutSource("api_key", "123")
	suite.config.SetWithoutSource("observability_pipelines_worker.logs.enabled", true)
	suite.config.SetWithoutSource("observability_pipelines_worker.logs.url", "https://vector.host:8443/")
	endpoints, err := BuildHTTPEndpointsWithVectorOverride(suite.config, "test-track", "test-proto", "test-source")
	suite.Nil(err)
	expectedEndpoints := getTestEndpoints(getTestEndpoint("vector.host", 8443, true))
	suite.Nil(err)
	suite.compareEndpoints(expectedEndpoints, endpoints)
}

func (suite *ConfigTestSuite) TestBuildEndpointsWithVectorHostAndPortOverride() {
	suite.config.SetWithoutSource("api_key", "123")
	suite.config.SetWithoutSource("observability_pipelines_worker.logs.enabled", true)
	suite.config.SetWithoutSource("observability_pipelines_worker.logs.url", "observability_pipelines_worker.host:8443")
	endpoints, err := BuildHTTPEndpointsWithVectorOverride(suite.config, "test-track", "test-proto", "test-source")
	suite.Nil(err)
	expectedEndpoints := getTestEndpoints(getTestEndpoint("observability_pipelines_worker.host", 8443, true))
	suite.Nil(err)
	suite.compareEndpoints(expectedEndpoints, endpoints)
}

func (suite *ConfigTestSuite) TestBuildEndpointsWithVectorHostAndPortNoSSLOverride() {
	suite.config.SetWithoutSource("api_key", "123")
	suite.config.SetWithoutSource("logs_config.logs_no_ssl", true)
	suite.config.SetWithoutSource("observability_pipelines_worker.logs.enabled", true)
	suite.config.SetWithoutSource("observability_pipelines_worker.logs.url", "observability_pipelines_worker.host:8443")
	endpoints, err := BuildHTTPEndpointsWithVectorOverride(suite.config, "test-track", "test-proto", "test-source")
	suite.Nil(err)
	expectedEndpoints := getTestEndpoints(getTestEndpoint("observability_pipelines_worker.host", 8443, false))
	suite.Nil(err)
	suite.compareEndpoints(expectedEndpoints, endpoints)
}

func (suite *ConfigTestSuite) TestBuildEndpointsWithoutVector() {
	suite.config.SetWithoutSource("api_key", "123")
	suite.config.SetWithoutSource("logs_config.logs_no_ssl", true)
	suite.config.SetWithoutSource("observability_pipelines_worker.logs.enabled", true)
	suite.config.SetWithoutSource("observability_pipelines_worker.logs.url", "observability_pipelines_worker.host:8443")
	endpoints, err := BuildHTTPEndpoints(suite.config, "test-track", "test-proto", "test-source")
	suite.Nil(err)
	expectedEndpoints := getTestEndpoints(getTestEndpoint("agent-http-intake.logs.datadoghq.com.", 0, true))
	suite.Nil(err)
	suite.compareEndpoints(expectedEndpoints, endpoints)
}

func (suite *ConfigTestSuite) TestEndpointsSetNonDefaultCustomConfigs() {
	suite.config.SetWithoutSource("api_key", "123")

	suite.config.SetWithoutSource("network_devices.netflow.forwarder.use_compression", false)
	suite.config.SetWithoutSource("network_devices.netflow.forwarder.zstd_compression_level", 10)
	suite.config.SetWithoutSource("network_devices.netflow.forwarder.batch_wait", 10)
	suite.config.SetWithoutSource("network_devices.netflow.forwarder.connection_reset_interval", 3)
	suite.config.SetWithoutSource("network_devices.netflow.forwarder.logs_no_ssl", true)
	suite.config.SetWithoutSource("network_devices.netflow.forwarder.batch_max_concurrent_send", 15)
	suite.config.SetWithoutSource("network_devices.netflow.forwarder.batch_max_content_size", 6000000)
	suite.config.SetWithoutSource("network_devices.netflow.forwarder.batch_max_size", 2000)
	suite.config.SetWithoutSource("network_devices.netflow.forwarder.input_chan_size", 5000)
	suite.config.SetWithoutSource("network_devices.netflow.forwarder.sender_backoff_factor", 4.0)
	suite.config.SetWithoutSource("network_devices.netflow.forwarder.sender_backoff_base", 2.0)
	suite.config.SetWithoutSource("network_devices.netflow.forwarder.sender_backoff_max", 150.0)
	suite.config.SetWithoutSource("network_devices.netflow.forwarder.sender_recovery_interval", 5)
	suite.config.SetWithoutSource("network_devices.netflow.forwarder.sender_recovery_reset", true)
	suite.config.SetWithoutSource("network_devices.netflow.forwarder.use_v2_api", true)

	logsConfig := NewLogsConfigKeys("network_devices.netflow.forwarder.", suite.config)
	endpoints, err := BuildHTTPEndpointsWithConfig(suite.config, logsConfig, "ndmflow-intake.", "ndmflow", "test-proto", "test-origin")

	suite.Nil(err)

	main := Endpoint{
		apiKey:                  atomic.NewString("123"),
		configSettingPath:       "api_key",
		isAdditionalEndpoint:    false,
		additionalEndpointsIdx:  0,
		Host:                    "ndmflow-intake.datadoghq.com.",
		Port:                    0,
		useSSL:                  true,
		UseCompression:          false,
		CompressionLevel:        10,
		BackoffFactor:           4,
		BackoffBase:             2,
		BackoffMax:              150,
		RecoveryInterval:        5,
		Version:                 EPIntakeVersion2,
		TrackType:               "ndmflow",
		ConnectionResetInterval: 3000000000,
		RecoveryReset:           true,
		Protocol:                "test-proto",
		Origin:                  "test-origin",
		isReliable:              true,
	}

	expectedEndpoints := &Endpoints{
		UseHTTP:                true,
		BatchWait:              10 * time.Second,
		Main:                   main,
		Endpoints:              []Endpoint{main},
		BatchMaxSize:           2000,
		BatchMaxContentSize:    6000000,
		BatchMaxConcurrentSend: 15,
		InputChanSize:          5000,
	}

	suite.Nil(err)
	suite.compareEndpoints(expectedEndpoints, endpoints)
}

func (suite *ConfigTestSuite) TestEndpointsSetLogsDDUrlWithPrefix() {
	suite.config.SetWithoutSource("api_key", "123")
	suite.config.SetWithoutSource("compliance_config.endpoints.logs_dd_url", "https://my-proxy.com:443")

	logsConfig := NewLogsConfigKeys("compliance_config.endpoints.", suite.config)
	endpoints, err := BuildHTTPEndpointsWithConfig(suite.config, logsConfig, "default-intake.mydomain.", "test-track", "test-proto", "test-source")

	suite.Nil(err)

	main := Endpoint{
		apiKey:                 atomic.NewString("123"),
		configSettingPath:      "api_key",
		isAdditionalEndpoint:   false,
		additionalEndpointsIdx: 0,
		Host:                   "my-proxy.com",
		Port:                   443,
		useSSL:                 true,
		UseCompression:         true,
		CompressionLevel:       ZstdCompressionLevel,
		BackoffFactor:          pkgconfigsetup.DefaultLogsSenderBackoffFactor,
		BackoffBase:            pkgconfigsetup.DefaultLogsSenderBackoffBase,
		BackoffMax:             pkgconfigsetup.DefaultLogsSenderBackoffMax,
		RecoveryInterval:       pkgconfigsetup.DefaultLogsSenderBackoffRecoveryInterval,
		Version:                EPIntakeVersion2,
		TrackType:              "test-track",
		Protocol:               "test-proto",
		Origin:                 "test-source",
		isReliable:             true,
	}

	expectedEndpoints := &Endpoints{
		UseHTTP:                true,
		BatchWait:              pkgconfigsetup.DefaultBatchWait * time.Second,
		Main:                   main,
		Endpoints:              []Endpoint{main},
		BatchMaxSize:           pkgconfigsetup.DefaultBatchMaxSize,
		BatchMaxContentSize:    pkgconfigsetup.DefaultBatchMaxContentSize,
		BatchMaxConcurrentSend: pkgconfigsetup.DefaultBatchMaxConcurrentSend,
		InputChanSize:          pkgconfigsetup.DefaultInputChanSize,
	}

	suite.Nil(err)
	suite.compareEndpoints(expectedEndpoints, endpoints)
}

func (suite *ConfigTestSuite) TestEndpointsSetDDUrlWithPrefix() {
	suite.config.SetWithoutSource("api_key", "123")
	suite.config.SetWithoutSource("compliance_config.endpoints.dd_url", "https://my-proxy.com:443")

	logsConfig := NewLogsConfigKeys("compliance_config.endpoints.", suite.config)
	endpoints, err := BuildHTTPEndpointsWithConfig(suite.config, logsConfig, "default-intake.mydomain.", "test-track", "test-proto", "test-source")

	suite.Nil(err)

	main := Endpoint{
		apiKey:                 atomic.NewString("123"),
		configSettingPath:      "api_key",
		isAdditionalEndpoint:   false,
		additionalEndpointsIdx: 0,
		Host:                   "my-proxy.com",
		Port:                   443,
		useSSL:                 true,
		UseCompression:         true,
		CompressionLevel:       ZstdCompressionLevel,
		BackoffFactor:          pkgconfigsetup.DefaultLogsSenderBackoffFactor,
		BackoffBase:            pkgconfigsetup.DefaultLogsSenderBackoffBase,
		BackoffMax:             pkgconfigsetup.DefaultLogsSenderBackoffMax,
		RecoveryInterval:       pkgconfigsetup.DefaultLogsSenderBackoffRecoveryInterval,
		Version:                EPIntakeVersion2,
		TrackType:              "test-track",
		Protocol:               "test-proto",
		Origin:                 "test-source",
		isReliable:             true,
	}

	expectedEndpoints := &Endpoints{
		UseHTTP:                true,
		BatchWait:              pkgconfigsetup.DefaultBatchWait * time.Second,
		Main:                   main,
		Endpoints:              []Endpoint{main},
		BatchMaxSize:           pkgconfigsetup.DefaultBatchMaxSize,
		BatchMaxContentSize:    pkgconfigsetup.DefaultBatchMaxContentSize,
		BatchMaxConcurrentSend: pkgconfigsetup.DefaultBatchMaxConcurrentSend,
		InputChanSize:          pkgconfigsetup.DefaultInputChanSize,
	}

	suite.Nil(err)
	suite.compareEndpoints(expectedEndpoints, endpoints)
}

func Test_parseAddressWithScheme(t *testing.T) {
	type args struct {
		address       string
		defaultNoSSL  bool
		defaultParser defaultParseAddressFunc
	}
	tests := []struct {
		name       string
		args       args
		wantHost   string
		wantPort   int
		wantUseSSL bool
		wantErr    bool
	}{
		{
			name: "url without scheme and port",
			args: args{
				address:       "localhost:8080",
				defaultNoSSL:  true,
				defaultParser: parseAddress,
			},
			wantHost:   "localhost",
			wantPort:   8080,
			wantUseSSL: false,
			wantErr:    false,
		},
		{
			name: "url with https prefix",
			args: args{
				address:       "https://localhost",
				defaultNoSSL:  true,
				defaultParser: parseAddress,
			},
			wantHost:   "localhost",
			wantPort:   0,
			wantUseSSL: true,
			wantErr:    false,
		},
		{
			name: "url with https prefix and port",
			args: args{
				address:       "https://localhost:443",
				defaultParser: parseAddress,
			},
			wantHost:   "localhost",
			wantPort:   443,
			wantUseSSL: true,
			wantErr:    false,
		},
		{
			name: "invalid url",
			args: args{
				address:       "https://localhost:443-8080",
				defaultNoSSL:  true,
				defaultParser: parseAddressAsHost,
			},
			wantHost:   "",
			wantPort:   0,
			wantUseSSL: false,
			wantErr:    true,
		},
		{
			name: "allow emptyPort",
			args: args{
				address:       "https://localhost",
				defaultNoSSL:  true,
				defaultParser: parseAddressAsHost,
			},
			wantHost:   "localhost",
			wantPort:   0,
			wantUseSSL: true,
			wantErr:    false,
		},
		{
			name: "no schema, not port emptyPort",
			args: args{
				address:       "localhost",
				defaultNoSSL:  false,
				defaultParser: parseAddressAsHost,
			},
			wantHost:   "localhost",
			wantPort:   0,
			wantUseSSL: true,
			wantErr:    false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotHost, gotPort, gotUseSSL, err := parseAddressWithScheme(tt.args.address, tt.args.defaultNoSSL, tt.args.defaultParser)
			if (err != nil) != tt.wantErr {
				t.Errorf("parseAddressWithScheme() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if gotHost != tt.wantHost {
				t.Errorf("parseAddressWithScheme() gotHost = %v, want %v", gotHost, tt.wantHost)
			}
			if gotPort != tt.wantPort {
				t.Errorf("parseAddressWithScheme() gotPort = %v, want %v", gotPort, tt.wantPort)
			}
			if gotUseSSL != tt.wantUseSSL {
				t.Errorf("parseAddressWithScheme() gotUseSSL = %v, want %v", gotUseSSL, tt.wantUseSSL)
			}
		})
	}
}
