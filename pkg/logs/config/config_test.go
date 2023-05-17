// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package config

import (
	"runtime"
	"testing"
	"time"

	coreConfig "github.com/DataDog/datadog-agent/pkg/config"
	"github.com/stretchr/testify/suite"
)

type ConfigTestSuite struct {
	suite.Suite
	config *coreConfig.MockConfig
}

func (suite *ConfigTestSuite) SetupTest() {
	suite.config = coreConfig.Mock(nil)
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
	if runtime.GOOS == "windows" || runtime.GOOS == "darwin" {
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

func (suite *ConfigTestSuite) TestGlobalProcessingRulesShouldReturnNoRulesWithEmptyValues() {
	var (
		rules []*ProcessingRule
		err   error
	)

	suite.config.Set("logs_config.processing_rules", nil)

	rules, err = GlobalProcessingRules()
	suite.Nil(err)
	suite.Equal(0, len(rules))

	suite.config.Set("logs_config.processing_rules", "")

	rules, err = GlobalProcessingRules()
	suite.Nil(err)
	suite.Equal(0, len(rules))
}

func (suite *ConfigTestSuite) TestGlobalProcessingRulesShouldReturnRulesWithValidMap() {
	var (
		rules []*ProcessingRule
		rule  *ProcessingRule
		err   error
	)

	suite.config.Set("logs_config.processing_rules", []map[string]interface{}{
		{
			"type":    "exclude_at_match",
			"name":    "exclude_foo",
			"pattern": "foo",
		},
	})

	rules, err = GlobalProcessingRules()
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

	suite.config.Set("logs_config.processing_rules", `[{"type":"mask_sequences","name":"mask_api_keys","replace_placeholder":"****************************","pattern":"([A-Fa-f0-9]{28})"}]`)

	rules, err = GlobalProcessingRules()
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
	taggerWarmupDuration := TaggerWarmupDuration()
	suite.Equal(0*time.Second, taggerWarmupDuration)

	// override
	suite.config.Set("logs_config.tagger_warmup_duration", 5)
	taggerWarmupDuration = TaggerWarmupDuration()
	suite.Equal(5*time.Second, taggerWarmupDuration)
}

func TestConfigTestSuite(t *testing.T) {
	suite.Run(t, new(ConfigTestSuite))
}

func (suite *ConfigTestSuite) TestMultipleHttpEndpointsEnvVar() {
	suite.config.Set("api_key", "123")
	suite.config.Set("logs_config.batch_wait", 1)
	suite.config.Set("logs_config.logs_dd_url", "agent-http-intake.logs.datadoghq.com:443")
	suite.config.Set("logs_config.use_compression", true)
	suite.config.Set("logs_config.compression_level", 6)
	suite.config.Set("logs_config.logs_no_ssl", false)
	suite.config.Set("logs_config.sender_backoff_factor", 3.0)
	suite.config.Set("logs_config.sender_backoff_base", 1.0)
	suite.config.Set("logs_config.sender_backoff_max", 2.0)
	suite.config.Set("logs_config.sender_recovery_interval", 10)
	suite.config.Set("logs_config.sender_recovery_reset", true)
	suite.config.Set("logs_config.use_v2_api", false)

	suite.T().Setenv("DD_LOGS_CONFIG_ADDITIONAL_ENDPOINTS", `[
	{"api_key": "456", "host": "additional.endpoint.1", "port": 1234, "use_compression": true, "compression_level": 2},
	{"api_key": "789", "host": "additional.endpoint.2", "port": 1234, "use_compression": true, "compression_level": 2}]`)

	expectedMainEndpoint := Endpoint{
		APIKey:           "123",
		Host:             "agent-http-intake.logs.datadoghq.com",
		Port:             443,
		UseSSL:           true,
		UseCompression:   true,
		CompressionLevel: 6,
		BackoffFactor:    3,
		BackoffBase:      1.0,
		BackoffMax:       2.0,
		RecoveryInterval: 10,
		RecoveryReset:    true,
		Version:          EPIntakeVersion1,
	}
	expectedAdditionalEndpoint1 := Endpoint{
		APIKey:           "456",
		Host:             "additional.endpoint.1",
		Port:             1234,
		UseSSL:           true,
		UseCompression:   true,
		CompressionLevel: 6,
		BackoffFactor:    3,
		BackoffBase:      1.0,
		BackoffMax:       2.0,
		RecoveryInterval: 10,
		RecoveryReset:    true,
		Version:          EPIntakeVersion1,
	}
	expectedAdditionalEndpoint2 := Endpoint{
		APIKey:           "789",
		Host:             "additional.endpoint.2",
		Port:             1234,
		UseSSL:           true,
		UseCompression:   true,
		CompressionLevel: 6,
		BackoffFactor:    3,
		BackoffBase:      1.0,
		BackoffMax:       2.0,
		RecoveryInterval: 10,
		RecoveryReset:    true,
		Version:          EPIntakeVersion1,
	}

	expectedEndpoints := NewEndpointsWithBatchSettings(expectedMainEndpoint, []Endpoint{expectedAdditionalEndpoint1, expectedAdditionalEndpoint2}, false, true, 1*time.Second, coreConfig.DefaultBatchMaxConcurrentSend, coreConfig.DefaultBatchMaxSize, coreConfig.DefaultBatchMaxContentSize, coreConfig.DefaultInputChanSize)
	endpoints, err := BuildHTTPEndpoints("test-track", "test-proto", "test-source")

	suite.Nil(err)
	suite.Equal(expectedEndpoints, endpoints)
}

func (suite *ConfigTestSuite) TestMultipleTCPEndpointsEnvVar() {
	suite.config.Set("api_key", "123")
	suite.config.Set("logs_config.logs_dd_url", "agent-http-intake.logs.datadoghq.com:443")
	suite.config.Set("logs_config.logs_no_ssl", false)
	suite.config.Set("logs_config.socks5_proxy_address", "proxy.test:3128")
	suite.config.Set("logs_config.dev_mode_use_proto", true)

	suite.T().Setenv("DD_LOGS_CONFIG_ADDITIONAL_ENDPOINTS", `[{"api_key": "456      \n", "host": "additional.endpoint", "port": 1234}]`)

	expectedMainEndpoint := Endpoint{
		APIKey:           "123",
		Host:             "agent-http-intake.logs.datadoghq.com",
		Port:             443,
		UseSSL:           true,
		UseCompression:   false,
		CompressionLevel: 0,
		ProxyAddress:     "proxy.test:3128",
	}
	expectedAdditionalEndpoint := Endpoint{
		APIKey:           "456",
		Host:             "additional.endpoint",
		Port:             1234,
		UseSSL:           true,
		UseCompression:   false,
		CompressionLevel: 0,
		ProxyAddress:     "proxy.test:3128",
	}

	expectedEndpoints := NewEndpoints(expectedMainEndpoint, []Endpoint{expectedAdditionalEndpoint}, true, false)
	endpoints, err := buildTCPEndpoints(defaultLogsConfigKeys())

	suite.Nil(err)
	suite.Equal(expectedEndpoints, endpoints)
}

func (suite *ConfigTestSuite) TestMultipleHttpEndpointsInConfig() {
	suite.config.Set("api_key", "123")
	suite.config.Set("logs_config.batch_wait", 1)
	suite.config.Set("logs_config.logs_dd_url", "agent-http-intake.logs.datadoghq.com:443")
	suite.config.Set("logs_config.use_compression", true)
	suite.config.Set("logs_config.compression_level", 6)
	suite.config.Set("logs_config.logs_no_ssl", false)
	suite.config.Set("logs_config.use_v2_api", false)

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
	suite.config.Set("logs_config.additional_endpoints", endpointsInConfig)

	expectedMainEndpoint := Endpoint{
		APIKey:           "123",
		Host:             "agent-http-intake.logs.datadoghq.com",
		Port:             443,
		UseSSL:           true,
		UseCompression:   true,
		CompressionLevel: 6,
		BackoffFactor:    coreConfig.DefaultLogsSenderBackoffFactor,
		BackoffBase:      coreConfig.DefaultLogsSenderBackoffBase,
		BackoffMax:       coreConfig.DefaultLogsSenderBackoffMax,
		RecoveryInterval: coreConfig.DefaultLogsSenderBackoffRecoveryInterval,
		Version:          EPIntakeVersion1,
	}
	expectedAdditionalEndpoint1 := Endpoint{
		APIKey:           "456",
		Host:             "additional.endpoint.1",
		Port:             1234,
		UseSSL:           true,
		UseCompression:   true,
		CompressionLevel: 6,
		BackoffFactor:    coreConfig.DefaultLogsSenderBackoffFactor,
		BackoffBase:      coreConfig.DefaultLogsSenderBackoffBase,
		BackoffMax:       coreConfig.DefaultLogsSenderBackoffMax,
		RecoveryInterval: coreConfig.DefaultLogsSenderBackoffRecoveryInterval,
		Version:          EPIntakeVersion1,
	}
	expectedAdditionalEndpoint2 := Endpoint{
		APIKey:           "789",
		Host:             "additional.endpoint.2",
		Port:             1234,
		UseSSL:           true,
		UseCompression:   true,
		CompressionLevel: 6,
		BackoffFactor:    coreConfig.DefaultLogsSenderBackoffFactor,
		BackoffBase:      coreConfig.DefaultLogsSenderBackoffBase,
		BackoffMax:       coreConfig.DefaultLogsSenderBackoffMax,
		RecoveryInterval: coreConfig.DefaultLogsSenderBackoffRecoveryInterval,
		Version:          EPIntakeVersion1,
	}

	expectedEndpoints := NewEndpointsWithBatchSettings(expectedMainEndpoint, []Endpoint{expectedAdditionalEndpoint1, expectedAdditionalEndpoint2}, false, true, 1*time.Second, coreConfig.DefaultBatchMaxConcurrentSend, coreConfig.DefaultBatchMaxSize, coreConfig.DefaultBatchMaxContentSize, coreConfig.DefaultInputChanSize)
	endpoints, err := BuildHTTPEndpoints("test-track", "test-proto", "test-source")

	suite.Nil(err)
	suite.Equal(expectedEndpoints, endpoints)
}

func (suite *ConfigTestSuite) TestMultipleHttpEndpointsInConfig2() {
	suite.config.Set("api_key", "123")
	suite.config.Set("logs_config.batch_wait", 1)
	suite.config.Set("logs_config.logs_dd_url", "agent-http-intake.logs.datadoghq.com:443")
	suite.config.Set("logs_config.use_compression", true)
	suite.config.Set("logs_config.compression_level", 6)
	suite.config.Set("logs_config.logs_no_ssl", false)
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
	suite.config.Set("logs_config.additional_endpoints", endpointsInConfig)

	expectedMainEndpoint := Endpoint{
		APIKey:           "123",
		Host:             "agent-http-intake.logs.datadoghq.com",
		Port:             443,
		UseSSL:           true,
		UseCompression:   true,
		CompressionLevel: 6,
		BackoffFactor:    coreConfig.DefaultLogsSenderBackoffFactor,
		BackoffBase:      coreConfig.DefaultLogsSenderBackoffBase,
		BackoffMax:       coreConfig.DefaultLogsSenderBackoffMax,
		RecoveryInterval: coreConfig.DefaultLogsSenderBackoffRecoveryInterval,
		Version:          EPIntakeVersion2,
		TrackType:        "test-track",
		Protocol:         "test-proto",
		Origin:           "test-source",
	}
	expectedAdditionalEndpoint1 := Endpoint{
		APIKey:           "456",
		Host:             "additional.endpoint.1",
		Port:             1234,
		UseSSL:           true,
		UseCompression:   true,
		CompressionLevel: 6,
		BackoffFactor:    coreConfig.DefaultLogsSenderBackoffFactor,
		BackoffBase:      coreConfig.DefaultLogsSenderBackoffBase,
		BackoffMax:       coreConfig.DefaultLogsSenderBackoffMax,
		RecoveryInterval: coreConfig.DefaultLogsSenderBackoffRecoveryInterval,
		Version:          EPIntakeVersion1,
	}
	expectedAdditionalEndpoint2 := Endpoint{
		APIKey:           "789",
		Host:             "additional.endpoint.2",
		Port:             1234,
		UseSSL:           true,
		UseCompression:   true,
		CompressionLevel: 6,
		BackoffFactor:    coreConfig.DefaultLogsSenderBackoffFactor,
		BackoffBase:      coreConfig.DefaultLogsSenderBackoffBase,
		BackoffMax:       coreConfig.DefaultLogsSenderBackoffMax,
		RecoveryInterval: coreConfig.DefaultLogsSenderBackoffRecoveryInterval,
		Version:          EPIntakeVersion2,
		TrackType:        "test-track",
		Protocol:         "test-proto",
		Origin:           "test-source",
	}

	expectedEndpoints := NewEndpointsWithBatchSettings(expectedMainEndpoint, []Endpoint{expectedAdditionalEndpoint1, expectedAdditionalEndpoint2}, false, true, 1*time.Second, coreConfig.DefaultBatchMaxConcurrentSend, coreConfig.DefaultBatchMaxSize, coreConfig.DefaultBatchMaxContentSize, coreConfig.DefaultInputChanSize)
	endpoints, err := BuildHTTPEndpoints("test-track", "test-proto", "test-source")

	suite.Nil(err)
	suite.Equal(expectedEndpoints, endpoints)
}

func (suite *ConfigTestSuite) TestMultipleTCPEndpointsInConf() {
	suite.config.Set("api_key", "123")
	suite.config.Set("logs_config.logs_dd_url", "agent-http-intake.logs.datadoghq.com:443")
	suite.config.Set("logs_config.logs_no_ssl", false)
	suite.config.Set("logs_config.socks5_proxy_address", "proxy.test:3128")
	suite.config.Set("logs_config.dev_mode_use_proto", true)
	suite.config.Set("logs_config.dev_mode_use_proto", true)
	endpointsInConfig := []map[string]interface{}{
		{
			"api_key": "456",
			"host":    "additional.endpoint",
			"port":    1234},
	}
	suite.config.Set("logs_config.additional_endpoints", endpointsInConfig)

	expectedMainEndpoint := Endpoint{
		APIKey:           "123",
		Host:             "agent-http-intake.logs.datadoghq.com",
		Port:             443,
		UseSSL:           true,
		UseCompression:   false,
		CompressionLevel: 0,
		ProxyAddress:     "proxy.test:3128",
	}
	expectedAdditionalEndpoint := Endpoint{
		APIKey:           "456",
		Host:             "additional.endpoint",
		Port:             1234,
		UseSSL:           true,
		UseCompression:   false,
		CompressionLevel: 0,
		ProxyAddress:     "proxy.test:3128",
	}

	expectedEndpoints := NewEndpoints(expectedMainEndpoint, []Endpoint{expectedAdditionalEndpoint}, true, false)
	endpoints, err := buildTCPEndpoints(defaultLogsConfigKeys())

	suite.Nil(err)
	suite.Equal(expectedEndpoints, endpoints)
}

func (suite *ConfigTestSuite) TestEndpointsSetLogsDDUrl() {
	suite.config.Set("api_key", "123")
	suite.config.Set("compliance_config.endpoints.logs_dd_url", "my-proxy:443")

	logsConfig := NewLogsConfigKeys("compliance_config.endpoints.", suite.config)
	endpoints, err := BuildHTTPEndpointsWithConfig(logsConfig, "default-intake.mydomain.", "test-track", "test-proto", "test-source")

	suite.Nil(err)

	main := Endpoint{
		APIKey:           "123",
		Host:             "my-proxy",
		Port:             443,
		UseSSL:           true,
		UseCompression:   true,
		CompressionLevel: 6,
		BackoffFactor:    coreConfig.DefaultLogsSenderBackoffFactor,
		BackoffBase:      coreConfig.DefaultLogsSenderBackoffBase,
		BackoffMax:       coreConfig.DefaultLogsSenderBackoffMax,
		RecoveryInterval: coreConfig.DefaultLogsSenderBackoffRecoveryInterval,
		Version:          EPIntakeVersion2,
		TrackType:        "test-track",
		Protocol:         "test-proto",
		Origin:           "test-source",
	}

	expectedEndpoints := &Endpoints{
		UseHTTP:                true,
		BatchWait:              coreConfig.DefaultBatchWait * time.Second,
		Main:                   main,
		Endpoints:              []Endpoint{main},
		BatchMaxSize:           coreConfig.DefaultBatchMaxSize,
		BatchMaxContentSize:    coreConfig.DefaultBatchMaxContentSize,
		BatchMaxConcurrentSend: coreConfig.DefaultBatchMaxConcurrentSend,
		InputChanSize:          coreConfig.DefaultInputChanSize,
	}

	suite.Nil(err)
	suite.Equal(expectedEndpoints, endpoints)
}

func (suite *ConfigTestSuite) TestEndpointsSetDDSite() {
	suite.config.Set("api_key", "123")

	suite.T().Setenv("DD_SITE", "mydomain.com")

	suite.T().Setenv("DD_COMPLIANCE_CONFIG_ENDPOINTS_BATCH_WAIT", "10")

	logsConfig := NewLogsConfigKeys("compliance_config.endpoints.", suite.config)
	endpoints, err := BuildHTTPEndpointsWithConfig(logsConfig, "default-intake.logs.", "test-track", "test-proto", "test-source")

	suite.Nil(err)

	main := Endpoint{
		APIKey:           "123",
		Host:             "default-intake.logs.mydomain.com",
		Port:             0,
		UseSSL:           true,
		UseCompression:   true,
		CompressionLevel: 6,
		BackoffFactor:    coreConfig.DefaultLogsSenderBackoffFactor,
		BackoffBase:      coreConfig.DefaultLogsSenderBackoffBase,
		BackoffMax:       coreConfig.DefaultLogsSenderBackoffMax,
		RecoveryInterval: coreConfig.DefaultLogsSenderBackoffRecoveryInterval,
		Version:          EPIntakeVersion2,
		TrackType:        "test-track",
		Origin:           "test-source",
		Protocol:         "test-proto",
	}

	expectedEndpoints := &Endpoints{
		UseHTTP:                true,
		BatchWait:              10 * time.Second,
		Main:                   main,
		Endpoints:              []Endpoint{main},
		BatchMaxSize:           coreConfig.DefaultBatchMaxSize,
		BatchMaxContentSize:    coreConfig.DefaultBatchMaxContentSize,
		BatchMaxConcurrentSend: coreConfig.DefaultBatchMaxConcurrentSend,
		InputChanSize:          coreConfig.DefaultInputChanSize,
	}

	suite.Nil(err)
	suite.Equal(expectedEndpoints, endpoints)
}

func (suite *ConfigTestSuite) TestBuildServerlessEndpoints() {
	suite.config.Set("api_key", "123")
	suite.config.Set("logs_config.batch_wait", 1)

	main := Endpoint{
		APIKey:           "123",
		Host:             "http-intake.logs.datadoghq.com",
		Port:             0,
		UseSSL:           true,
		UseCompression:   true,
		CompressionLevel: 6,
		BackoffFactor:    coreConfig.DefaultLogsSenderBackoffFactor,
		BackoffBase:      coreConfig.DefaultLogsSenderBackoffBase,
		BackoffMax:       coreConfig.DefaultLogsSenderBackoffMax,
		RecoveryInterval: coreConfig.DefaultLogsSenderBackoffRecoveryInterval,
		Version:          EPIntakeVersion2,
		TrackType:        "test-track",
		Origin:           "lambda-extension",
		Protocol:         "test-proto",
	}

	expectedEndpoints := &Endpoints{
		UseHTTP:                true,
		BatchWait:              1 * time.Second,
		Main:                   main,
		Endpoints:              []Endpoint{main},
		BatchMaxSize:           coreConfig.DefaultBatchMaxSize,
		BatchMaxContentSize:    coreConfig.DefaultBatchMaxContentSize,
		BatchMaxConcurrentSend: coreConfig.DefaultBatchMaxConcurrentSend,
		InputChanSize:          coreConfig.DefaultInputChanSize,
	}

	endpoints, err := BuildServerlessEndpoints("test-track", "test-proto")

	suite.Nil(err)
	suite.Equal(expectedEndpoints, endpoints)
}

func getTestEndpoint(host string, port int, ssl bool) Endpoint {
	return Endpoint{
		APIKey:           "123",
		Host:             host,
		Port:             port,
		UseSSL:           ssl,
		UseCompression:   true,
		CompressionLevel: 6,
		BackoffFactor:    coreConfig.DefaultLogsSenderBackoffFactor,
		BackoffBase:      coreConfig.DefaultLogsSenderBackoffBase,
		BackoffMax:       coreConfig.DefaultLogsSenderBackoffMax,
		RecoveryInterval: coreConfig.DefaultLogsSenderBackoffRecoveryInterval,
		Version:          EPIntakeVersion2,
		TrackType:        "test-track",
		Protocol:         "test-proto",
		Origin:           "test-source",
	}
}

func getTestEndpoints(e Endpoint) *Endpoints {
	return &Endpoints{
		UseHTTP:                true,
		BatchWait:              coreConfig.DefaultBatchWait * time.Second,
		Main:                   e,
		Endpoints:              []Endpoint{e},
		BatchMaxSize:           coreConfig.DefaultBatchMaxSize,
		BatchMaxContentSize:    coreConfig.DefaultBatchMaxContentSize,
		BatchMaxConcurrentSend: coreConfig.DefaultBatchMaxConcurrentSend,
		InputChanSize:          coreConfig.DefaultInputChanSize,
	}
}
func (suite *ConfigTestSuite) TestBuildEndpointsWithVectorHttpOverride() {
	suite.config.Set("api_key", "123")
	suite.config.Set("observability_pipelines_worker.logs.enabled", true)
	suite.config.Set("observability_pipelines_worker.logs.url", "http://vector.host:8080/")
	endpoints, err := BuildHTTPEndpointsWithVectorOverride("test-track", "test-proto", "test-source")
	suite.Nil(err)
	expectedEndpoints := getTestEndpoints(getTestEndpoint("vector.host", 8080, false))
	suite.Nil(err)
	suite.Equal(expectedEndpoints, endpoints)
}

func (suite *ConfigTestSuite) TestBuildEndpointsWithVectorHttpsOverride() {
	suite.config.Set("api_key", "123")
	suite.config.Set("observability_pipelines_worker.logs.enabled", true)
	suite.config.Set("observability_pipelines_worker.logs.url", "https://vector.host:8443/")
	endpoints, err := BuildHTTPEndpointsWithVectorOverride("test-track", "test-proto", "test-source")
	suite.Nil(err)
	expectedEndpoints := getTestEndpoints(getTestEndpoint("vector.host", 8443, true))
	suite.Nil(err)
	suite.Equal(expectedEndpoints, endpoints)
}

func (suite *ConfigTestSuite) TestBuildEndpointsWithVectorHostAndPortOverride() {
	suite.config.Set("api_key", "123")
	suite.config.Set("observability_pipelines_worker.logs.enabled", true)
	suite.config.Set("observability_pipelines_worker.logs.url", "observability_pipelines_worker.host:8443")
	endpoints, err := BuildHTTPEndpointsWithVectorOverride("test-track", "test-proto", "test-source")
	suite.Nil(err)
	expectedEndpoints := getTestEndpoints(getTestEndpoint("observability_pipelines_worker.host", 8443, true))
	suite.Nil(err)
	suite.Equal(expectedEndpoints, endpoints)
}

func (suite *ConfigTestSuite) TestBuildEndpointsWithVectorHostAndPortNoSSLOverride() {
	suite.config.Set("api_key", "123")
	suite.config.Set("logs_config.logs_no_ssl", true)
	suite.config.Set("observability_pipelines_worker.logs.enabled", true)
	suite.config.Set("observability_pipelines_worker.logs.url", "observability_pipelines_worker.host:8443")
	endpoints, err := BuildHTTPEndpointsWithVectorOverride("test-track", "test-proto", "test-source")
	suite.Nil(err)
	expectedEndpoints := getTestEndpoints(getTestEndpoint("observability_pipelines_worker.host", 8443, false))
	suite.Nil(err)
	suite.Equal(expectedEndpoints, endpoints)
}

func (suite *ConfigTestSuite) TestBuildEndpointsWithoutVector() {
	suite.config.Set("api_key", "123")
	suite.config.Set("logs_config.logs_no_ssl", true)
	suite.config.Set("observability_pipelines_worker.logs.enabled", true)
	suite.config.Set("observability_pipelines_worker.logs.url", "observability_pipelines_worker.host:8443")
	endpoints, err := BuildHTTPEndpoints("test-track", "test-proto", "test-source")
	suite.Nil(err)
	expectedEndpoints := getTestEndpoints(getTestEndpoint("agent-http-intake.logs.datadoghq.com", 0, true))
	suite.Nil(err)
	suite.Equal(expectedEndpoints, endpoints)
}

func (suite *ConfigTestSuite) TestEndpointsSetNonDefaultCustomConfigs() {
	suite.config.Set("api_key", "123")

	suite.config.Set("network_devices.netflow.forwarder.use_compression", false)
	suite.config.Set("network_devices.netflow.forwarder.compression_level", 10)
	suite.config.Set("network_devices.netflow.forwarder.batch_wait", 10)
	suite.config.Set("network_devices.netflow.forwarder.connection_reset_interval", 3)
	suite.config.Set("network_devices.netflow.forwarder.logs_no_ssl", true)
	suite.config.Set("network_devices.netflow.forwarder.batch_max_concurrent_send", 15)
	suite.config.Set("network_devices.netflow.forwarder.batch_max_content_size", 6000000)
	suite.config.Set("network_devices.netflow.forwarder.batch_max_size", 2000)
	suite.config.Set("network_devices.netflow.forwarder.input_chan_size", 5000)
	suite.config.Set("network_devices.netflow.forwarder.sender_backoff_factor", 4.0)
	suite.config.Set("network_devices.netflow.forwarder.sender_backoff_base", 2.0)
	suite.config.Set("network_devices.netflow.forwarder.sender_backoff_max", 150.0)
	suite.config.Set("network_devices.netflow.forwarder.sender_recovery_interval", 5)
	suite.config.Set("network_devices.netflow.forwarder.sender_recovery_reset", true)
	suite.config.Set("network_devices.netflow.forwarder.use_v2_api", true)

	logsConfig := NewLogsConfigKeys("network_devices.netflow.forwarder.", suite.config)
	endpoints, err := BuildHTTPEndpointsWithConfig(logsConfig, "ndmflow-intake.", "ndmflow", "test-proto", "test-origin")

	suite.Nil(err)

	main := Endpoint{
		APIKey:                  "123",
		Host:                    "ndmflow-intake.datadoghq.com",
		Port:                    0,
		UseSSL:                  true,
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
	suite.Equal(expectedEndpoints, endpoints)
}

func (suite *ConfigTestSuite) TestEndpointsSetLogsDDUrlWithPrefix() {
	suite.config.Set("api_key", "123")
	suite.config.Set("compliance_config.endpoints.logs_dd_url", "https://my-proxy.com:443")

	logsConfig := NewLogsConfigKeys("compliance_config.endpoints.", suite.config)
	endpoints, err := BuildHTTPEndpointsWithConfig(logsConfig, "default-intake.mydomain.", "test-track", "test-proto", "test-source")

	suite.Nil(err)

	main := Endpoint{
		APIKey:           "123",
		Host:             "my-proxy.com",
		Port:             443,
		UseSSL:           true,
		UseCompression:   true,
		CompressionLevel: 6,
		BackoffFactor:    coreConfig.DefaultLogsSenderBackoffFactor,
		BackoffBase:      coreConfig.DefaultLogsSenderBackoffBase,
		BackoffMax:       coreConfig.DefaultLogsSenderBackoffMax,
		RecoveryInterval: coreConfig.DefaultLogsSenderBackoffRecoveryInterval,
		Version:          EPIntakeVersion2,
		TrackType:        "test-track",
		Protocol:         "test-proto",
		Origin:           "test-source",
	}

	expectedEndpoints := &Endpoints{
		UseHTTP:                true,
		BatchWait:              coreConfig.DefaultBatchWait * time.Second,
		Main:                   main,
		Endpoints:              []Endpoint{main},
		BatchMaxSize:           coreConfig.DefaultBatchMaxSize,
		BatchMaxContentSize:    coreConfig.DefaultBatchMaxContentSize,
		BatchMaxConcurrentSend: coreConfig.DefaultBatchMaxConcurrentSend,
		InputChanSize:          coreConfig.DefaultInputChanSize,
	}

	suite.Nil(err)
	suite.Equal(expectedEndpoints, endpoints)
}

func (suite *ConfigTestSuite) TestEndpointsSetDDUrlWithPrefix() {
	suite.config.Set("api_key", "123")
	suite.config.Set("compliance_config.endpoints.dd_url", "https://my-proxy.com:443")

	logsConfig := NewLogsConfigKeys("compliance_config.endpoints.", suite.config)
	endpoints, err := BuildHTTPEndpointsWithConfig(logsConfig, "default-intake.mydomain.", "test-track", "test-proto", "test-source")

	suite.Nil(err)

	main := Endpoint{
		APIKey:           "123",
		Host:             "my-proxy.com",
		Port:             443,
		UseSSL:           true,
		UseCompression:   true,
		CompressionLevel: 6,
		BackoffFactor:    coreConfig.DefaultLogsSenderBackoffFactor,
		BackoffBase:      coreConfig.DefaultLogsSenderBackoffBase,
		BackoffMax:       coreConfig.DefaultLogsSenderBackoffMax,
		RecoveryInterval: coreConfig.DefaultLogsSenderBackoffRecoveryInterval,
		Version:          EPIntakeVersion2,
		TrackType:        "test-track",
		Protocol:         "test-proto",
		Origin:           "test-source",
	}

	expectedEndpoints := &Endpoints{
		UseHTTP:                true,
		BatchWait:              coreConfig.DefaultBatchWait * time.Second,
		Main:                   main,
		Endpoints:              []Endpoint{main},
		BatchMaxSize:           coreConfig.DefaultBatchMaxSize,
		BatchMaxContentSize:    coreConfig.DefaultBatchMaxContentSize,
		BatchMaxConcurrentSend: coreConfig.DefaultBatchMaxConcurrentSend,
		InputChanSize:          coreConfig.DefaultInputChanSize,
	}

	suite.Nil(err)
	suite.Equal(expectedEndpoints, endpoints)
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
