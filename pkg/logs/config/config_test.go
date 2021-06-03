// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package config

import (
	"os"
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
	suite.config = coreConfig.Mock()
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
	suite.Equal(100, suite.config.GetInt("logs_config.open_files_limit"))
	suite.Equal(9000, suite.config.GetInt("logs_config.frame_size"))
	suite.Equal("", suite.config.GetString("logs_config.socks5_proxy_address"))
	suite.Equal("", suite.config.GetString("logs_config.logs_dd_url"))
	suite.Equal(false, suite.config.GetBool("logs_config.logs_no_ssl"))
	suite.Equal(30, suite.config.GetInt("logs_config.stop_grace_period"))
	suite.Equal(nil, suite.config.Get("logs_config.processing_rules"))
	suite.Equal("", suite.config.GetString("logs_config.processing_rules"))
	suite.Equal(false, suite.config.GetBool("logs_config.use_http"))
	suite.Equal(false, suite.config.GetBool("logs_config.k8s_container_use_file"))
}

func (suite *ConfigTestSuite) TestDefaultSources() {
	// container collect all source

	source := ContainerCollectAllSource()
	suite.Nil(source)

	suite.config.Set("logs_config.container_collect_all", true)

	source = ContainerCollectAllSource()
	suite.NotNil(source)

	suite.Equal("container_collect_all", source.Name)
	suite.Equal(DockerType, source.Config.Type)
	suite.Equal("docker", source.Config.Source)
	suite.Equal("docker", source.Config.Service)
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

	os.Setenv("DD_LOGS_CONFIG_ADDITIONAL_ENDPOINTS", `[
	{"api_key": "456", "host": "additional.endpoint.1", "port": 1234, "use_compression": true, "compression_level": 2},
	{"api_key": "789", "host": "additional.endpoint.2", "port": 1234, "use_compression": true, "compression_level": 2}]`)
	defer os.Unsetenv("DD_LOGS_CONFIG_ADDITIONAL_ENDPOINTS")

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
	}
	expectedAdditionalEndpoint1 := Endpoint{
		APIKey:           "456",
		Host:             "additional.endpoint.1",
		Port:             1234,
		UseSSL:           true,
		UseCompression:   true,
		CompressionLevel: 2}
	expectedAdditionalEndpoint2 := Endpoint{
		APIKey:           "789",
		Host:             "additional.endpoint.2",
		Port:             1234,
		UseSSL:           true,
		UseCompression:   true,
		CompressionLevel: 2}

	expectedEndpoints := NewEndpointsWithBatchSettings(expectedMainEndpoint, []Endpoint{expectedAdditionalEndpoint1, expectedAdditionalEndpoint2}, false, true,
		1*time.Second, coreConfig.DefaultBatchMaxConcurrentSend, coreConfig.DefaultBatchMaxSize, coreConfig.DefaultBatchMaxContentSize)
	endpoints, err := BuildHTTPEndpoints()

	suite.Nil(err)
	suite.Equal(expectedEndpoints, endpoints)
}

func (suite *ConfigTestSuite) TestMultipleTCPEndpointsEnvVar() {
	suite.config.Set("api_key", "123")
	suite.config.Set("logs_config.logs_dd_url", "agent-http-intake.logs.datadoghq.com:443")
	suite.config.Set("logs_config.logs_no_ssl", false)
	suite.config.Set("logs_config.socks5_proxy_address", "proxy.test:3128")
	suite.config.Set("logs_config.dev_mode_use_proto", true)

	os.Setenv("DD_LOGS_CONFIG_ADDITIONAL_ENDPOINTS", `[{"api_key": "456      \n", "host": "additional.endpoint", "port": 1234}]`)
	defer os.Unsetenv("DD_LOGS_CONFIG_ADDITIONAL_ENDPOINTS")

	expectedMainEndpoint := Endpoint{
		APIKey:           "123",
		Host:             "agent-http-intake.logs.datadoghq.com",
		Port:             443,
		UseSSL:           true,
		UseCompression:   false,
		CompressionLevel: 0,
		ProxyAddress:     "proxy.test:3128"}
	expectedAdditionalEndpoint := Endpoint{
		APIKey:           "456",
		Host:             "additional.endpoint",
		Port:             1234,
		UseSSL:           true,
		UseCompression:   false,
		CompressionLevel: 0,
		ProxyAddress:     "proxy.test:3128"}

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
	}
	expectedAdditionalEndpoint1 := Endpoint{
		APIKey:           "456",
		Host:             "additional.endpoint.1",
		Port:             1234,
		UseSSL:           true,
		UseCompression:   true,
		CompressionLevel: 2}
	expectedAdditionalEndpoint2 := Endpoint{
		APIKey:           "789",
		Host:             "additional.endpoint.2",
		Port:             1234,
		UseSSL:           true,
		UseCompression:   true,
		CompressionLevel: 2}

	expectedEndpoints := NewEndpointsWithBatchSettings(expectedMainEndpoint, []Endpoint{expectedAdditionalEndpoint1, expectedAdditionalEndpoint2}, false, true,
		1*time.Second, coreConfig.DefaultBatchMaxConcurrentSend, coreConfig.DefaultBatchMaxSize, coreConfig.DefaultBatchMaxContentSize)
	endpoints, err := BuildHTTPEndpoints()

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
		ProxyAddress:     "proxy.test:3128"}
	expectedAdditionalEndpoint := Endpoint{
		APIKey:           "456",
		Host:             "additional.endpoint",
		Port:             1234,
		UseSSL:           true,
		UseCompression:   false,
		CompressionLevel: 0,
		ProxyAddress:     "proxy.test:3128"}

	expectedEndpoints := NewEndpoints(expectedMainEndpoint, []Endpoint{expectedAdditionalEndpoint}, true, false)
	endpoints, err := buildTCPEndpoints(defaultLogsConfigKeys())

	suite.Nil(err)
	suite.Equal(expectedEndpoints, endpoints)
}

func (suite *ConfigTestSuite) TestEndpointsSetLogsDDUrl() {
	suite.config.Set("api_key", "123")
	suite.config.Set("compliance_config.endpoints.logs_dd_url", "my-proxy:443")

	logsConfig := NewLogsConfigKeys("compliance_config.endpoints.", suite.config)
	endpoints, err := BuildHTTPEndpointsWithConfig(logsConfig, "default-intake.mydomain.")

	suite.Nil(err)

	expectedEndpoints := &Endpoints{
		UseHTTP:   true,
		BatchWait: coreConfig.DefaultBatchWait * time.Second,
		Main: Endpoint{
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
		},
		BatchMaxSize:           coreConfig.DefaultBatchMaxSize,
		BatchMaxContentSize:    coreConfig.DefaultBatchMaxContentSize,
		BatchMaxConcurrentSend: coreConfig.DefaultBatchMaxConcurrentSend,
	}

	suite.Nil(err)
	suite.Equal(expectedEndpoints, endpoints)
}

func (suite *ConfigTestSuite) TestEndpointsSetDDSite() {
	suite.config.Set("api_key", "123")

	os.Setenv("DD_SITE", "mydomain.com")
	defer os.Unsetenv("DD_SITE")

	os.Setenv("DD_COMPLIANCE_CONFIG_ENDPOINTS_BATCH_WAIT", "10")
	defer os.Unsetenv("DD_COMPLIANCE_CONFIG_ENDPOINTS_BATCH_WAIT")

	logsConfig := NewLogsConfigKeys("compliance_config.endpoints.", suite.config)
	endpoints, err := BuildHTTPEndpointsWithConfig(logsConfig, "default-intake.logs.")

	suite.Nil(err)

	expectedEndpoints := &Endpoints{
		UseHTTP:   true,
		BatchWait: 10 * time.Second,
		Main: Endpoint{
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
		},
		BatchMaxSize:           coreConfig.DefaultBatchMaxSize,
		BatchMaxContentSize:    coreConfig.DefaultBatchMaxContentSize,
		BatchMaxConcurrentSend: coreConfig.DefaultBatchMaxConcurrentSend,
	}

	suite.Nil(err)
	suite.Equal(expectedEndpoints, endpoints)
}

func (suite *ConfigTestSuite) TestBuildServerlessEndpoints() {
	suite.config.Set("api_key", "123")
	suite.config.Set("logs_config.batch_wait", 1)

	expectedEndpoints := &Endpoints{
		UseHTTP:   true,
		BatchWait: 1 * time.Second,
		Main: Endpoint{
			APIKey:           "123",
			Host:             "lambda-http-intake.logs.datadoghq.com",
			Port:             0,
			UseSSL:           true,
			UseCompression:   true,
			CompressionLevel: 6,
			BackoffFactor:    coreConfig.DefaultLogsSenderBackoffFactor,
			BackoffBase:      coreConfig.DefaultLogsSenderBackoffBase,
			BackoffMax:       coreConfig.DefaultLogsSenderBackoffMax,
			RecoveryInterval: coreConfig.DefaultLogsSenderBackoffRecoveryInterval,
		},
		BatchMaxSize:           coreConfig.DefaultBatchMaxSize,
		BatchMaxContentSize:    coreConfig.DefaultBatchMaxContentSize,
		BatchMaxConcurrentSend: coreConfig.DefaultBatchMaxConcurrentSend,
	}

	endpoints, err := BuildServerlessEndpoints()

	suite.Nil(err)
	suite.Equal(expectedEndpoints, endpoints)
}
