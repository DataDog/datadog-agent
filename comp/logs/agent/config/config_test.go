// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package config

import (
	"bytes"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/suite"
	"go.uber.org/atomic"

	"github.com/DataDog/datadog-agent/comp/core/config"
	pkgconfigsetup "github.com/DataDog/datadog-agent/pkg/config/setup"
	"github.com/DataDog/datadog-agent/pkg/logs/types"
	pkglog "github.com/DataDog/datadog-agent/pkg/util/log"
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
	suite.Equal([]interface{}{}, suite.config.Get("logs_config.processing_rules"))
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

	suite.config.SetInTest("logs_config.processing_rules", nil)

	rules, err = GlobalProcessingRules(suite.config)
	suite.Nil(err)
	suite.Equal(0, len(rules))

	suite.config.SetInTest("logs_config.processing_rules", "")

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

	suite.config.SetInTest("logs_config.processing_rules", []map[string]interface{}{{"type": "exclude_at_match", "name": "exclude_foo", "pattern": "foo"}})

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

	suite.config.SetInTest("logs_config.processing_rules", `[{"type":"mask_sequences","name":"mask_api_keys","replace_placeholder":"****************************","pattern":"([A-Fa-f0-9]{28})"}]`)

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
	suite.config.SetInTest("logs_config.tagger_warmup_duration", 5)
	taggerWarmupDuration = TaggerWarmupDuration(suite.config)
	suite.Equal(5*time.Second, taggerWarmupDuration)
}

func (suite *ConfigTestSuite) TestGlobalFingerprintConfigCount() {
	suite.config.SetInTest("logs_config.fingerprint_config.fingerprint_strategy", "line_checksum")

	config, err := GlobalFingerprintConfig(suite.config)
	suite.Nil(err, "Expected no error")
	suite.NotNil(config, "Expected config to be set")
	suite.Equal(types.DefaultLinesCount, config.Count, "Expected count to be set to default lines count")

	suite.config.SetInTest("logs_config.fingerprint_config.fingerprint_strategy", "byte_checksum")

	config, err = GlobalFingerprintConfig(suite.config)
	suite.Nil(err, "Expected no error")
	suite.NotNil(config, "Expected config to be set")
	suite.Equal(types.DefaultBytesCount, config.Count, "Expected count to be set to default bytes count")
}

func (suite *ConfigTestSuite) TestGlobalFingerprintConfigShouldReturnConfigWithValidMap() {
	suite.config.SetInTest("logs_config.fingerprint_config.fingerprint_strategy", "line_checksum")
	suite.config.SetInTest("logs_config.fingerprint_config.count", 10)
	suite.config.SetInTest("logs_config.fingerprint_config.count_to_skip", 5)
	suite.config.SetInTest("logs_config.fingerprint_config.max_bytes", 1024)

	config, err := GlobalFingerprintConfig(suite.config)
	suite.Nil(err)
	suite.NotNil(config)
	suite.Equal(types.FingerprintStrategyLineChecksum, config.FingerprintStrategy)
	suite.Equal(10, config.Count)
	suite.Equal(5, config.CountToSkip)
	suite.Equal(1024, config.MaxBytes)
}

func (suite *ConfigTestSuite) TestGlobalFingerprintConfigShouldReturnStrategyDisabled() {
	suite.config.SetInTest("logs_config.fingerprint_config.fingerprint_strategy", "disabled")
	suite.config.SetInTest("logs_config.fingerprint_config.count", 10)
	suite.config.SetInTest("logs_config.fingerprint_config.count_to_skip", 5)
	suite.config.SetInTest("logs_config.fingerprint_config.max_bytes", 1024)

	config, err := GlobalFingerprintConfig(suite.config)
	suite.Nil(err)
	suite.Equal(types.FingerprintStrategyDisabled, config.FingerprintStrategy)
}

func (suite *ConfigTestSuite) TestGlobalFingerprintConfigShouldReturnErrorWithInvalidConfig() {
	suite.config.SetInTest("logs_config.fingerprint_config.fingerprint_strategy", "invalid_strategy") // Invalid: unknown strategy
	suite.config.SetInTest("logs_config.fingerprint_config.count", -1)                                // Invalid: negative value
	suite.config.SetInTest("logs_config.fingerprint_config.count_to_skip", 5)
	suite.config.SetInTest("logs_config.fingerprint_config.max_bytes", 1024)

	config, err := GlobalFingerprintConfig(suite.config)
	suite.NotNil(err)
	suite.Nil(config)
}

func TestConfigTestSuite(t *testing.T) {
	suite.Run(t, new(ConfigTestSuite))
}

func (suite *ConfigTestSuite) TestMultipleHttpEndpointsEnvVar() {
	// Set like an env var
	suite.config.SetInTest("logs_config.additional_endpoints", `[
		{"api_key": "456", "host": "additional.endpoint.1", "port": 1234, "use_compression": true, "compression_level": 2},
		{"api_key": "789", "host": "additional.endpoint.2", "port": 1234, "use_compression": true, "compression_level": 2}]`)

	suite.config.SetInTest("api_key", "123")
	suite.config.SetInTest("logs_config.batch_wait", 1)
	suite.config.SetInTest("logs_config.logs_dd_url", "agent-http-intake.logs.datadoghq.com:443")
	suite.config.SetInTest("logs_config.use_compression", true)
	suite.config.SetInTest("logs_config.compression_level", 6)
	suite.config.SetInTest("logs_config.logs_no_ssl", false)
	suite.config.SetInTest("logs_config.sender_backoff_factor", 3.0)
	suite.config.SetInTest("logs_config.sender_backoff_base", 1.0)
	suite.config.SetInTest("logs_config.sender_backoff_max", 2.0)
	suite.config.SetInTest("logs_config.sender_recovery_interval", 10)
	suite.config.SetInTest("logs_config.sender_recovery_reset", true)
	suite.config.SetInTest("logs_config.use_v2_api", false)

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
	suite.config.SetInTest("logs_config.additional_endpoints", `[{"api_key": "456      \n", "host": "additional.endpoint", "port": 1234}]`)

	suite.config.SetInTest("api_key", "123")
	suite.config.SetInTest("logs_config.logs_dd_url", "agent-http-intake.logs.datadoghq.com:443")
	suite.config.SetInTest("logs_config.logs_no_ssl", false)
	suite.config.SetInTest("logs_config.socks5_proxy_address", "proxy.test:3128")
	suite.config.SetInTest("logs_config.dev_mode_use_proto", true)

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
	endpoints, err := buildTCPEndpoints(suite.config, defaultLogsConfigKeys(suite.config), true)

	suite.Nil(err)
	suite.compareEndpoints(expectedEndpoints, endpoints)
}

func (suite *ConfigTestSuite) TestMultipleHttpEndpointsInConfig() {
	suite.config.SetInTest("api_key", "123")
	suite.config.SetInTest("logs_config.batch_wait", 1)
	suite.config.SetInTest("logs_config.logs_dd_url", "agent-http-intake.logs.datadoghq.com:443")
	suite.config.SetInTest("logs_config.use_compression", true)
	suite.config.SetInTest("logs_config.compression_level", 6)
	suite.config.SetInTest("logs_config.logs_no_ssl", false)
	suite.config.SetInTest("logs_config.use_v2_api", false)

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
	suite.config.SetInTest("logs_config.additional_endpoints", endpointsInConfig)

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
	suite.config.SetInTest("api_key", "123")
	suite.config.SetInTest("logs_config.batch_wait", 1)
	suite.config.SetInTest("logs_config.logs_dd_url", "agent-http-intake.logs.datadoghq.com:443")
	suite.config.SetInTest("logs_config.use_compression", true)
	suite.config.SetInTest("logs_config.compression_level", 6)
	suite.config.SetInTest("logs_config.logs_no_ssl", false)
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
	suite.config.SetInTest("logs_config.additional_endpoints", endpointsInConfig)

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
	suite.config.SetInTest("api_key", "123")
	suite.config.SetInTest("logs_config.logs_dd_url", "agent-http-intake.logs.datadoghq.com:443")
	suite.config.SetInTest("logs_config.logs_no_ssl", false)
	suite.config.SetInTest("logs_config.socks5_proxy_address", "proxy.test:3128")
	suite.config.SetInTest("logs_config.dev_mode_use_proto", true)
	suite.config.SetInTest("logs_config.dev_mode_use_proto", true)
	endpointsInConfig := []map[string]interface{}{
		{
			"api_key": "456",
			"host":    "additional.endpoint",
			"port":    1234},
	}
	suite.config.SetInTest("logs_config.additional_endpoints", endpointsInConfig)

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
	endpoints, err := buildTCPEndpoints(suite.config, defaultLogsConfigKeys(suite.config), true)

	suite.Nil(err)
	suite.compareEndpoints(expectedEndpoints, endpoints)
}

func (suite *ConfigTestSuite) TestEndpointsSetLogsDDUrl() {
	expectedHost := "my-proxy"
	expectedPort := 8888

	setupAndBuildEndpoints := func(url string, connectivity HTTPConnectivity) (*Endpoints, error) {
		suite.config.SetInTest("api_key", "123")
		suite.config.SetInTest("compliance_config.endpoints.logs_dd_url", url)
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
	suite.config.SetInTest("api_key", "123")

	suite.config.SetInTest("site", "mydomain.com")
	suite.config.SetInTest("compliance_config.endpoints_batch_wait", "mydomain.com")
	suite.config.SetInTest("compliance_config.endpoints.batch_wait", "10")

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
	suite.config.SetInTest("api_key", "123")
	suite.config.SetInTest("logs_config.batch_wait", 1)

	main := Endpoint{
		apiKey:                 atomic.NewString("123"),
		configSettingPath:      "api_key",
		isAdditionalEndpoint:   false,
		additionalEndpointsIdx: 0,
		Host:                   "http-intake.logs.datadoghq.com.",
		Port:                   0,
		useSSL:                 true,
		UseCompression:         true,
		CompressionKind:        ZstdCompressionKind,
		CompressionLevel:       ZstdCompressionLevel,
		BackoffFactor:          pkgconfigsetup.DefaultLogsSenderBackoffFactor,
		BackoffBase:            pkgconfigsetup.DefaultLogsSenderBackoffBase,
		BackoffMax:             pkgconfigsetup.DefaultLogsSenderBackoffMax,
		RecoveryInterval:       pkgconfigsetup.DefaultLogsSenderBackoffRecoveryInterval,
		Version:                EPIntakeVersion2,
		TrackType:              "test-track",
		Origin:                 "serverless",
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
	e := NewEndpoint("123", "", host, port, EmptyPathPrefix, ssl)
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
	suite.config.SetInTest("api_key", "123")
	suite.config.SetInTest("observability_pipelines_worker.logs.enabled", true)
	suite.config.SetInTest("observability_pipelines_worker.logs.url", "http://vector.host:8080/")
	endpoints, err := BuildHTTPEndpointsWithVectorOverride(suite.config, "test-track", "test-proto", "test-source")
	suite.Nil(err)
	expectedEndpoints := getTestEndpoints(getTestEndpoint("vector.host", 8080, false))
	suite.Nil(err)
	suite.compareEndpoints(expectedEndpoints, endpoints)
}

func (suite *ConfigTestSuite) TestBuildEndpointsWithVectorHttpsOverride() {
	suite.config.SetInTest("api_key", "123")
	suite.config.SetInTest("observability_pipelines_worker.logs.enabled", true)
	suite.config.SetInTest("observability_pipelines_worker.logs.url", "https://vector.host:8443/")
	endpoints, err := BuildHTTPEndpointsWithVectorOverride(suite.config, "test-track", "test-proto", "test-source")
	suite.Nil(err)
	expectedEndpoints := getTestEndpoints(getTestEndpoint("vector.host", 8443, true))
	suite.Nil(err)
	suite.compareEndpoints(expectedEndpoints, endpoints)
}

func (suite *ConfigTestSuite) TestBuildEndpointsWithVectorHostAndPortOverride() {
	suite.config.SetInTest("api_key", "123")
	suite.config.SetInTest("observability_pipelines_worker.logs.enabled", true)
	suite.config.SetInTest("observability_pipelines_worker.logs.url", "observability_pipelines_worker.host:8443")
	endpoints, err := BuildHTTPEndpointsWithVectorOverride(suite.config, "test-track", "test-proto", "test-source")
	suite.Nil(err)
	expectedEndpoints := getTestEndpoints(getTestEndpoint("observability_pipelines_worker.host", 8443, true))
	suite.Nil(err)
	suite.compareEndpoints(expectedEndpoints, endpoints)
}

func (suite *ConfigTestSuite) TestBuildEndpointsWithVectorHostAndPortNoSSLOverride() {
	suite.config.SetInTest("api_key", "123")
	suite.config.SetInTest("logs_config.logs_no_ssl", true)
	suite.config.SetInTest("observability_pipelines_worker.logs.enabled", true)
	suite.config.SetInTest("observability_pipelines_worker.logs.url", "observability_pipelines_worker.host:8443")
	endpoints, err := BuildHTTPEndpointsWithVectorOverride(suite.config, "test-track", "test-proto", "test-source")
	suite.Nil(err)
	expectedEndpoints := getTestEndpoints(getTestEndpoint("observability_pipelines_worker.host", 8443, false))
	suite.Nil(err)
	suite.compareEndpoints(expectedEndpoints, endpoints)
}

func (suite *ConfigTestSuite) TestBuildEndpointsWithoutVector() {
	suite.config.SetInTest("api_key", "123")
	suite.config.SetInTest("logs_config.logs_no_ssl", true)
	suite.config.SetInTest("observability_pipelines_worker.logs.enabled", true)
	suite.config.SetInTest("observability_pipelines_worker.logs.url", "observability_pipelines_worker.host:8443")
	endpoints, err := BuildHTTPEndpoints(suite.config, "test-track", "test-proto", "test-source")
	suite.Nil(err)
	expectedEndpoints := getTestEndpoints(getTestEndpoint("agent-http-intake.logs.datadoghq.com.", 0, true))
	suite.Nil(err)
	suite.compareEndpoints(expectedEndpoints, endpoints)
}

// TestBuildEndpointsWithOPWDualShip verifies that when dual_ship=true the primary DD endpoint
// is kept and OPW is added as an additional best-effort (unreliable) endpoint by default.
func (suite *ConfigTestSuite) TestBuildEndpointsWithOPWDualShip() {
	suite.config.SetInTest("api_key", "123")
	suite.config.SetInTest("observability_pipelines_worker.logs.enabled", true)
	suite.config.SetInTest("observability_pipelines_worker.logs.url", "https://opw.example.com:8443/")
	suite.config.SetInTest("observability_pipelines_worker.logs.dual_ship", true)

	endpoints, err := BuildHTTPEndpointsWithVectorOverride(suite.config, "test-track", "test-proto", "test-source")
	suite.Require().Nil(err)

	// Primary endpoint must be the Datadog intake, not OPW.
	suite.Equal("agent-http-intake.logs.datadoghq.com.", endpoints.Main.Host)

	// There must be exactly two endpoints: [main (DD), OPW additional].
	suite.Require().Len(endpoints.Endpoints, 2, "expected 2 endpoints (DD primary + OPW additional)")

	opwEndpoint := endpoints.Endpoints[1]
	suite.Equal("opw.example.com", opwEndpoint.Host)
	suite.Equal(8443, opwEndpoint.Port)
	suite.True(opwEndpoint.UseSSL())
	suite.False(opwEndpoint.IsReliable(), "OPW dual-ship endpoint must default to unreliable so an unhealthy OPW cannot stall delivery to Datadog")
	suite.True(opwEndpoint.isAdditionalEndpoint)

	// OPW must inherit v2-API metadata from the main endpoint so traffic uses
	// /api/v2/logs and the DD-PROTOCOL / DD-EVP-ORIGIN headers — same semantics as
	// the primary Datadog destination and as user-supplied additional_endpoints.
	suite.Equal(endpoints.Main.Version, opwEndpoint.Version)
	suite.Equal(endpoints.Main.TrackType, opwEndpoint.TrackType)
	suite.Equal(endpoints.Main.Protocol, opwEndpoint.Protocol)
	suite.Equal(endpoints.Main.Origin, opwEndpoint.Origin)
	suite.Equal(EPIntakeVersion2, opwEndpoint.Version)
	suite.Equal(IntakeTrackType("test-track"), opwEndpoint.TrackType)
}

// TestBuildEndpointsWithOPWDualShipReliable verifies that the dual_ship_reliable opt-in flips the
// OPW additional endpoint to reliable mode (so OPW failures apply backpressure to the main pipeline).
func (suite *ConfigTestSuite) TestBuildEndpointsWithOPWDualShipReliable() {
	suite.config.SetInTest("api_key", "123")
	suite.config.SetInTest("observability_pipelines_worker.logs.enabled", true)
	suite.config.SetInTest("observability_pipelines_worker.logs.url", "https://opw.example.com:8443/")
	suite.config.SetInTest("observability_pipelines_worker.logs.dual_ship", true)
	suite.config.SetInTest("observability_pipelines_worker.logs.dual_ship_reliable", true)

	endpoints, err := BuildHTTPEndpointsWithVectorOverride(suite.config, "test-track", "test-proto", "test-source")
	suite.Require().Nil(err)

	suite.Require().Len(endpoints.Endpoints, 2)
	opwEndpoint := endpoints.Endpoints[1]
	suite.Equal("opw.example.com", opwEndpoint.Host)
	suite.True(opwEndpoint.IsReliable(), "dual_ship_reliable=true must make OPW a reliable additional endpoint")
}

// TestBuildEndpointsWithOPWDualShipAndAdditionalEndpoints verifies that when dual_ship=true is
// combined with explicit additional_endpoints the user-configured endpoints are preserved and
// the final list is: [DD primary, OPW additional, ...user additional_endpoints].
func (suite *ConfigTestSuite) TestBuildEndpointsWithOPWDualShipAndAdditionalEndpoints() {
	suite.config.SetInTest("api_key", "123")
	suite.config.SetInTest("observability_pipelines_worker.logs.enabled", true)
	suite.config.SetInTest("observability_pipelines_worker.logs.url", "https://opw.example.com:8443/")
	suite.config.SetInTest("observability_pipelines_worker.logs.dual_ship", true)
	suite.config.SetInTest("logs_config.additional_endpoints", []map[string]interface{}{
		{"api_key": "456", "host": "extra.logs.example.com", "port": 443, "use_ssl": true},
	})

	endpoints, err := BuildHTTPEndpointsWithVectorOverride(suite.config, "test-track", "test-proto", "test-source")
	suite.Require().Nil(err)

	// Primary endpoint must be Datadog.
	suite.Equal("agent-http-intake.logs.datadoghq.com.", endpoints.Main.Host)

	// [DD primary, OPW additional, user-configured additional] = 3 total.
	suite.Require().Len(endpoints.Endpoints, 3, "expected 3 endpoints (DD + OPW + user additional)")

	opwEndpoint := endpoints.Endpoints[1]
	suite.Equal("opw.example.com", opwEndpoint.Host)

	userEndpoint := endpoints.Endpoints[2]
	suite.Equal("extra.logs.example.com", userEndpoint.Host)
}

// TestBuildEndpointsWithOPWNoDualShipReplacesPrimary verifies that when dual_ship is absent
// (default false) OPW continues to replace the primary Datadog endpoint — the default OPW mode.
func (suite *ConfigTestSuite) TestBuildEndpointsWithOPWNoDualShipReplacesPrimary() {
	suite.config.SetInTest("api_key", "123")
	suite.config.SetInTest("observability_pipelines_worker.logs.enabled", true)
	suite.config.SetInTest("observability_pipelines_worker.logs.url", "https://opw.example.com:8443/")
	// dual_ship is intentionally NOT set — should default to false.

	endpoints, err := BuildHTTPEndpointsWithVectorOverride(suite.config, "test-track", "test-proto", "test-source")
	suite.Require().Nil(err)

	// Primary endpoint must be OPW (the default OPW-replaces-primary behaviour).
	suite.Equal("opw.example.com", endpoints.Main.Host)
	suite.Equal(8443, endpoints.Main.Port)

	// Only one endpoint in total (no additional endpoints).
	suite.Require().Len(endpoints.Endpoints, 1, "expected 1 endpoint (OPW as primary, default mode)")
}

// TestBuildEndpointsWithOPWDualShipInheritsCompressionOverride verifies that a compression override
// passed via BuildHTTPEndpointsWithCompressionOverride is propagated to the OPW dual-ship endpoint.
// Without the copy block the OPW endpoint would use the pre-override compression settings while the
// primary DD endpoint uses the overridden ones — causing the two endpoints to compress differently.
func (suite *ConfigTestSuite) TestBuildEndpointsWithOPWDualShipInheritsCompressionOverride() {
	suite.config.SetInTest("api_key", "123")
	suite.config.SetInTest("observability_pipelines_worker.logs.enabled", true)
	suite.config.SetInTest("observability_pipelines_worker.logs.url", "https://opw.example.com:8443/")
	suite.config.SetInTest("observability_pipelines_worker.logs.dual_ship", true)

	logsConfig := NewLogsConfigKeysWithVector("logs_config.", "logs.", suite.config)
	compressionOverride := EndpointCompressionOptions{
		CompressionKind:  "zstd",
		CompressionLevel: 9,
	}

	endpoints, err := BuildHTTPEndpointsWithCompressionOverride(suite.config, logsConfig, httpEndpointPrefix, "test-track", "test-proto", "test-source", compressionOverride)
	suite.Require().Nil(err)

	suite.Require().Len(endpoints.Endpoints, 2)
	opwEndpoint := endpoints.Endpoints[1]
	suite.Equal("opw.example.com", opwEndpoint.Host)

	// OPW must use the same compression as the primary DD endpoint.
	suite.Equal(endpoints.Main.UseCompression, opwEndpoint.UseCompression, "OPW must inherit UseCompression from main")
	suite.Equal("zstd", opwEndpoint.CompressionKind, "OPW must inherit CompressionKind override from main")
	suite.Equal(9, opwEndpoint.CompressionLevel, "OPW must inherit CompressionLevel override from main")
	suite.Equal(endpoints.Main.BackoffFactor, opwEndpoint.BackoffFactor, "OPW must inherit BackoffFactor from main")
	suite.Equal(endpoints.Main.BackoffBase, opwEndpoint.BackoffBase, "OPW must inherit BackoffBase from main")
	suite.Equal(endpoints.Main.BackoffMax, opwEndpoint.BackoffMax, "OPW must inherit BackoffMax from main")
	suite.Equal(endpoints.Main.RecoveryInterval, opwEndpoint.RecoveryInterval, "OPW must inherit RecoveryInterval from main")
	suite.Equal(endpoints.Main.RecoveryReset, opwEndpoint.RecoveryReset, "OPW must inherit RecoveryReset from main")
	suite.Equal(endpoints.Main.ConnectionResetInterval, opwEndpoint.ConnectionResetInterval, "OPW must inherit ConnectionResetInterval from main")
}

// TestBuildEndpointsWithLegacyVectorDualShip verifies that the legacy vector.* prefix is
// honoured for dual_ship so that users still on the old config are not silently broken.
// When vector.logs.dual_ship=true the OPW endpoint must appear as an additional endpoint
// (dual-ship mode) exactly as it would when the modern observability_pipelines_worker key is set.
func (suite *ConfigTestSuite) TestBuildEndpointsWithLegacyVectorDualShip() {
	suite.config.SetInTest("api_key", "123")
	// Use the legacy vector.* prefix for all three OPW settings.
	suite.config.SetInTest("vector.logs.enabled", true)
	suite.config.SetInTest("vector.logs.url", "https://opw.example.com:8443/")
	suite.config.SetInTest("vector.logs.dual_ship", true)

	endpoints, err := BuildHTTPEndpointsWithVectorOverride(suite.config, "test-track", "test-proto", "test-source")
	suite.Require().Nil(err)

	// Primary endpoint must be the Datadog intake, not OPW.
	suite.Equal("agent-http-intake.logs.datadoghq.com.", endpoints.Main.Host)

	// There must be exactly two endpoints: [main (DD), OPW additional].
	suite.Require().Len(endpoints.Endpoints, 2, "expected 2 endpoints (DD primary + OPW additional) when legacy vector.logs.dual_ship=true")

	opwEndpoint := endpoints.Endpoints[1]
	suite.Equal("opw.example.com", opwEndpoint.Host)
	suite.Equal(8443, opwEndpoint.Port)
	suite.True(opwEndpoint.UseSSL())
	suite.True(opwEndpoint.isAdditionalEndpoint)
	suite.False(opwEndpoint.IsReliable(), "OPW dual-ship endpoint must default to unreliable even via legacy prefix")
}

// TestBuildEndpointsWithLegacyVectorDualShipReliable verifies that the legacy
// vector.logs.dual_ship_reliable key is honoured via the fallback path.
func (suite *ConfigTestSuite) TestBuildEndpointsWithLegacyVectorDualShipReliable() {
	suite.config.SetInTest("api_key", "123")
	suite.config.SetInTest("vector.logs.enabled", true)
	suite.config.SetInTest("vector.logs.url", "https://opw.example.com:8443/")
	suite.config.SetInTest("vector.logs.dual_ship", true)
	suite.config.SetInTest("vector.logs.dual_ship_reliable", true)

	endpoints, err := BuildHTTPEndpointsWithVectorOverride(suite.config, "test-track", "test-proto", "test-source")
	suite.Require().Nil(err)

	suite.Require().Len(endpoints.Endpoints, 2)
	opwEndpoint := endpoints.Endpoints[1]
	suite.Equal("opw.example.com", opwEndpoint.Host)
	suite.True(opwEndpoint.IsReliable(), "vector.logs.dual_ship_reliable=true must make OPW a reliable additional endpoint via legacy prefix")
}

// TestBuildEndpointsWithOPWDualShipNoOPWEnabledWarns verifies that setting dual_ship=true when
// OPW is not enabled (or has no URL) emits a startup warning, because dual_ship has no effect
// in that configuration — the OPW block is skipped entirely.
func TestBuildEndpointsWithOPWDualShipNoOPWEnabledWarns(t *testing.T) {
	cfg := config.NewMock(t)
	cfg.SetInTest("api_key", "123")
	// OPW is NOT enabled — dual_ship=true should warn and silently no-op.
	cfg.SetInTest("observability_pipelines_worker.logs.dual_ship", true)

	var buf bytes.Buffer
	logger, err := pkglog.LoggerFromWriterWithMinLevelAndLvlMsgFormat(&buf, pkglog.WarnLvl)
	assert.NoError(t, err)
	pkglog.SetupLogger(logger, "warn")

	_, buildErr := BuildHTTPEndpointsWithVectorOverride(cfg, "test-track", "test-proto", "test-source")
	assert.NoError(t, buildErr)

	assert.True(t, strings.Contains(buf.String(), "dual_ship=true has no effect"), "expected startup warning when dual_ship=true and OPW is not enabled; got: %s", buf.String())
}

// TestBuildEndpointsWithOPWDualShipReliableWithoutDualShipWarns verifies that setting
// dual_ship_reliable=true without dual_ship=true emits a startup warning, because the
// reliability setting is silently ignored in that configuration.
func TestBuildEndpointsWithOPWDualShipReliableWithoutDualShipWarns(t *testing.T) {
	cfg := config.NewMock(t)
	cfg.SetInTest("api_key", "123")
	cfg.SetInTest("observability_pipelines_worker.logs.enabled", true)
	cfg.SetInTest("observability_pipelines_worker.logs.url", "https://opw.example.com:8443/")
	// dual_ship_reliable=true but dual_ship is false (default) — should warn.
	cfg.SetInTest("observability_pipelines_worker.logs.dual_ship_reliable", true)

	var buf bytes.Buffer
	logger, err := pkglog.LoggerFromWriterWithMinLevelAndLvlMsgFormat(&buf, pkglog.WarnLvl)
	assert.NoError(t, err)
	pkglog.SetupLogger(logger, "warn")

	_, buildErr := BuildHTTPEndpointsWithVectorOverride(cfg, "test-track", "test-proto", "test-source")
	assert.NoError(t, buildErr)

	assert.True(t, strings.Contains(buf.String(), "dual_ship_reliable=true has no effect"), "expected startup warning when dual_ship_reliable=true and dual_ship=false; got: %s", buf.String())
}

func (suite *ConfigTestSuite) TestEndpointsSetNonDefaultCustomConfigs() {
	suite.config.SetInTest("api_key", "123")

	suite.config.SetInTest("network_devices.netflow.forwarder.use_compression", false)
	suite.config.SetInTest("network_devices.netflow.forwarder.zstd_compression_level", 10)
	suite.config.SetInTest("network_devices.netflow.forwarder.batch_wait", 10)
	suite.config.SetInTest("network_devices.netflow.forwarder.connection_reset_interval", 3)
	suite.config.SetInTest("network_devices.netflow.forwarder.logs_no_ssl", true)
	suite.config.SetInTest("network_devices.netflow.forwarder.batch_max_concurrent_send", 15)
	suite.config.SetInTest("network_devices.netflow.forwarder.batch_max_content_size", 6000000)
	suite.config.SetInTest("network_devices.netflow.forwarder.batch_max_size", 2000)
	suite.config.SetInTest("network_devices.netflow.forwarder.input_chan_size", 5000)
	suite.config.SetInTest("network_devices.netflow.forwarder.sender_backoff_factor", 4.0)
	suite.config.SetInTest("network_devices.netflow.forwarder.sender_backoff_base", 2.0)
	suite.config.SetInTest("network_devices.netflow.forwarder.sender_backoff_max", 150.0)
	suite.config.SetInTest("network_devices.netflow.forwarder.sender_recovery_interval", 5)
	suite.config.SetInTest("network_devices.netflow.forwarder.sender_recovery_reset", true)
	suite.config.SetInTest("network_devices.netflow.forwarder.use_v2_api", true)

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
	suite.config.SetInTest("api_key", "123")
	suite.config.SetInTest("compliance_config.endpoints.logs_dd_url", "https://my-proxy.com:443")

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
	suite.config.SetInTest("api_key", "123")
	suite.config.SetInTest("compliance_config.endpoints.dd_url", "https://my-proxy.com:443")

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
		name           string
		args           args
		wantHost       string
		wantPort       int
		wantPathPrefix string
		wantUseSSL     bool
		wantErr        bool
	}{
		{
			name: "url without scheme and port",
			args: args{
				address:       "localhost:8080",
				defaultNoSSL:  true,
				defaultParser: parseAddress,
			},
			wantHost:       "localhost",
			wantPort:       8080,
			wantPathPrefix: "",
			wantUseSSL:     false,
			wantErr:        false,
		},
		{
			name: "url with https prefix",
			args: args{
				address:       "https://localhost",
				defaultNoSSL:  true,
				defaultParser: parseAddress,
			},
			wantHost:       "localhost",
			wantPort:       0,
			wantPathPrefix: "",
			wantUseSSL:     true,
			wantErr:        false,
		},
		{
			name: "url with https prefix and port",
			args: args{
				address:       "https://localhost:443",
				defaultParser: parseAddress,
			},
			wantHost:       "localhost",
			wantPort:       443,
			wantPathPrefix: "",
			wantUseSSL:     true,
			wantErr:        false,
		},
		{
			name: "invalid url",
			args: args{
				address:       "https://localhost:443-8080",
				defaultNoSSL:  true,
				defaultParser: parseAddressAsHost,
			},
			wantHost:       "",
			wantPort:       0,
			wantPathPrefix: "",
			wantUseSSL:     false,
			wantErr:        true,
		},
		{
			name: "allow emptyPort",
			args: args{
				address:       "https://localhost",
				defaultNoSSL:  true,
				defaultParser: parseAddressAsHost,
			},
			wantHost:       "localhost",
			wantPort:       0,
			wantPathPrefix: "",
			wantUseSSL:     true,
			wantErr:        false,
		},
		{
			name: "no schema, not port emptyPort",
			args: args{
				address:       "localhost",
				defaultNoSSL:  false,
				defaultParser: parseAddressAsHost,
			},
			wantHost:       "localhost",
			wantPort:       0,
			wantPathPrefix: "",
			wantUseSSL:     true,
			wantErr:        false,
		},
		{
			name: "path prefix",
			args: args{
				address:       "https://localhost:8080/path/prefix",
				defaultNoSSL:  true,
				defaultParser: parseAddress,
			},
			wantHost:       "localhost",
			wantPort:       8080,
			wantPathPrefix: "/path/prefix",
			wantUseSSL:     true,
			wantErr:        false,
		},
		{
			name: "legacy path v1 prefix",
			args: args{
				address:       "https://localhost:8080/v1/input",
				defaultNoSSL:  true,
				defaultParser: parseAddress,
			},
			wantHost:       "localhost",
			wantPort:       8080,
			wantPathPrefix: "",
			wantUseSSL:     true,
			wantErr:        false,
		},
		{
			name: "legacy path v2 prefix",
			args: args{
				address:       "https://localhost:8080/api/v2/logs",
				defaultNoSSL:  true,
				defaultParser: parseAddress,
			},
			wantHost:       "localhost",
			wantPort:       8080,
			wantPathPrefix: "",
			wantUseSSL:     true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotHost, gotPort, gotPathPrefix, gotUseSSL, err := parseAddressWithScheme(tt.args.address, tt.args.defaultNoSSL, tt.args.defaultParser)
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
			if gotPathPrefix != tt.wantPathPrefix {
				t.Errorf("parseAddressWithScheme() gotPathPrefix = %v, want %v", gotPathPrefix, tt.wantPathPrefix)
			}
			if gotUseSSL != tt.wantUseSSL {
				t.Errorf("parseAddressWithScheme() gotUseSSL = %v, want %v", gotUseSSL, tt.wantUseSSL)
			}
		})
	}
}

func TestLogsNoSSLWarnLog(t *testing.T) {
	tests := []struct {
		name         string
		address      string
		defaultNoSSL bool
		wantWarn     bool
	}{
		{
			name:         "https url with logs_no_ssl=true warns (genuine conflict)",
			address:      "https://agent-http-intake.logs.datadoghq.com",
			defaultNoSSL: true,
			wantWarn:     true,
		},
		{
			name:         "https url with logs_no_ssl=false does not warn (otel-agent default)",
			address:      "https://agent-http-intake.logs.datadoghq.com",
			defaultNoSSL: false,
			wantWarn:     false,
		},
		{
			name:         "http url with logs_no_ssl=true does not warn",
			address:      "http://agent-http-intake.logs.datadoghq.com",
			defaultNoSSL: true,
			wantWarn:     false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var buf bytes.Buffer
			logger, err := pkglog.LoggerFromWriterWithMinLevelAndLvlMsgFormat(&buf, pkglog.WarnLvl)
			assert.NoError(t, err)
			pkglog.SetupLogger(logger, "warn")

			_, _, _, _, _ = parseAddressWithScheme(tt.address, tt.defaultNoSSL, parseAddressAsHost)

			got := strings.Contains(buf.String(), "logs_no_ssl set to true")
			assert.Equal(t, tt.wantWarn, got, "unexpected warning log presence")
		})
	}
}

func TestShouldUseTCP(t *testing.T) {
	tests := []struct {
		name                string
		forceTCP            bool
		socks5Proxy         string
		additionalEndpoints bool
		expectedTCPRequired bool
	}{
		{
			name:                "no TCP requirements",
			forceTCP:            false,
			socks5Proxy:         "",
			additionalEndpoints: false,
			expectedTCPRequired: false,
		},
		{
			name:                "force_use_tcp enabled",
			forceTCP:            true,
			socks5Proxy:         "",
			additionalEndpoints: false,
			expectedTCPRequired: true,
		},
		{
			name:                "socks5 proxy set",
			forceTCP:            false,
			socks5Proxy:         "localhost:1080",
			additionalEndpoints: false,
			expectedTCPRequired: true,
		},
		{
			name:                "additional endpoints configured",
			forceTCP:            false,
			socks5Proxy:         "",
			additionalEndpoints: true,
			expectedTCPRequired: true,
		},
		{
			name:                "all TCP requirements set",
			forceTCP:            true,
			socks5Proxy:         "localhost:1080",
			additionalEndpoints: true,
			expectedTCPRequired: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := config.NewMock(t)
			cfg.SetInTest("api_key", "test-key")
			cfg.SetInTest("logs_config.force_use_tcp", tt.forceTCP)
			cfg.SetInTest("logs_config.socks5_proxy_address", tt.socks5Proxy)

			if tt.additionalEndpoints {
				cfg.SetInTest("logs_config.additional_endpoints", []map[string]interface{}{{"host": "additional-host.com", "port": 10516, "api_key": "additional-key"}})

			}

			result := ShouldUseTCP(cfg)
			if result != tt.expectedTCPRequired {
				t.Errorf("ShouldUseTCP() = %v, want %v", result, tt.expectedTCPRequired)
			}
		})
	}
}

func (suite *ConfigTestSuite) TestBatchWaitSubsecondValues() {
	suite.config.SetInTest("api_key", "123")

	// Test with 0.1 seconds (100ms)
	suite.config.SetInTest("logs_config.batch_wait", 0.1)

	logsConfig := NewLogsConfigKeys("logs_config.", suite.config)
	endpoints, err := BuildHTTPEndpointsWithConfig(suite.config, logsConfig, "http-intake.logs.", "test-track", "test-proto", "test-source")

	suite.Nil(err)
	suite.Equal(100*time.Millisecond, endpoints.BatchWait, "BatchWait should be 100ms")

	// Test with 0.5 seconds (500ms)
	suite.config.SetInTest("logs_config.batch_wait", 0.5)
	endpoints, err = BuildHTTPEndpointsWithConfig(suite.config, logsConfig, "http-intake.logs.", "test-track", "test-proto", "test-source")

	suite.Nil(err)
	suite.Equal(500*time.Millisecond, endpoints.BatchWait, "BatchWait should be 500ms")

	// Test with 1.5 seconds
	suite.config.SetInTest("logs_config.batch_wait", 1.5)
	endpoints, err = BuildHTTPEndpointsWithConfig(suite.config, logsConfig, "http-intake.logs.", "test-track", "test-proto", "test-source")

	suite.Nil(err)
	suite.Equal(1500*time.Millisecond, endpoints.BatchWait, "BatchWait should be 1.5 seconds")

	// Test with integer value for backwards compatibility
	suite.config.SetInTest("logs_config.batch_wait", 5)
	endpoints, err = BuildHTTPEndpointsWithConfig(suite.config, logsConfig, "http-intake.logs.", "test-track", "test-proto", "test-source")

	suite.Nil(err)
	suite.Equal(5*time.Second, endpoints.BatchWait, "BatchWait should be 5 seconds (integer value)")

	// Test with value below minimum (should fallback to default)
	suite.config.SetInTest("logs_config.batch_wait", 0.05) // 50ms, below 100ms minimum
	endpoints, err = BuildHTTPEndpointsWithConfig(suite.config, logsConfig, "http-intake.logs.", "test-track", "test-proto", "test-source")

	suite.Nil(err)
	suite.Equal(pkgconfigsetup.DefaultBatchWait*time.Second, endpoints.BatchWait, "BatchWait should fallback to default for too-small values")

	// Test with value above maximum (should fallback to default)
	suite.config.SetInTest("logs_config.batch_wait", 15) // Above 10 second maximum
	endpoints, err = BuildHTTPEndpointsWithConfig(suite.config, logsConfig, "http-intake.logs.", "test-track", "test-proto", "test-source")

	suite.Nil(err)
	suite.Equal(pkgconfigsetup.DefaultBatchWait*time.Second, endpoints.BatchWait, "BatchWait should fallback to default for too-large values")
}

func (suite *ConfigTestSuite) TestTCPEndpointsPortLookup() {
	// This test verifies that TCP endpoints are constructed with the correct ports
	// when the logsEndpoints map is looked up with hostnames that have trailing dots (FQDNs).
	// The trailing dots are added by GetMainEndpoint when convert_dd_site_fqdn.enabled is true.

	tests := []struct {
		name         string
		site         string
		expectedHost string
		expectedPort int
	}{
		{
			name:         "datadoghq.com site",
			site:         "datadoghq.com",
			expectedHost: "agent-intake.logs.datadoghq.com.",
			expectedPort: 10516,
		},
		{
			name:         "datadoghq.eu site",
			site:         "datadoghq.eu",
			expectedHost: "agent-intake.logs.datadoghq.eu.",
			expectedPort: 443,
		},
		{
			name:         "datadoghq.eu with-a-dot site",
			site:         "datadoghq.eu.",
			expectedHost: "agent-intake.logs.datadoghq.eu.",
			expectedPort: 443,
		},
		{
			name:         "datad0g.com site",
			site:         "datad0g.com",
			expectedHost: "agent-intake.logs.datad0g.com.",
			expectedPort: 10516,
		},
		{
			name:         "datad0g.eu site",
			site:         "datad0g.eu",
			expectedHost: "agent-intake.logs.datad0g.eu.",
			expectedPort: 443,
		},
	}

	for _, tt := range tests {
		suite.Run(tt.name, func() {
			suite.config.SetInTest("api_key", "test-key")
			suite.config.SetInTest("site", tt.site)
			suite.config.SetInTest("convert_dd_site_fqdn.enabled", true) // FQDN is enabled by default

			endpoints, err := buildTCPEndpoints(suite.config, defaultLogsConfigKeys(suite.config), true)

			suite.Nil(err)
			suite.Equal(tt.expectedHost, endpoints.Main.Host, "Host should match expected FQDN with trailing dot")
			suite.Equal(tt.expectedPort, endpoints.Main.Port, "Port should match the value from logsEndpoints map")

			suite.config.SetInTest("api_key", "test-key")
			suite.config.SetInTest("site", tt.site)
			suite.config.SetInTest("convert_dd_site_fqdn.enabled", false)

			endpoints, err = buildTCPEndpoints(suite.config, defaultLogsConfigKeys(suite.config), true)

			suite.Nil(err)
			suite.Equal(tt.expectedPort, endpoints.Main.Port, "Port should match the value from logsEndpoints map")
		})
	}
}
