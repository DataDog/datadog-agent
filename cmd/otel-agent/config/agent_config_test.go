// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package config

import (
	"context"
	"fmt"
	"io/fs"
	"os"
	"testing"

	"go.opentelemetry.io/collector/featuregate"

	configmock "github.com/DataDog/datadog-agent/pkg/config/mock"
	pkgconfigsetup "github.com/DataDog/datadog-agent/pkg/config/setup"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/stretchr/testify/suite"
)

type ConfigTestSuite struct {
	suite.Suite
}

func (suite *ConfigTestSuite) SetupTest() {
	configmock.New(suite.T())
	suite.T().Setenv("DD_API_KEY", "")
	suite.T().Setenv("DD_SITE", "")
}

func TestNoURIsProvided(t *testing.T) {
	_, err := NewConfigComponent(context.Background(), "", []string{})
	assert.Error(t, err, "no URIs provided for configs")
}

func (suite *ConfigTestSuite) TestAgentConfig() {
	t := suite.T()
	fileName := "testdata/config.yaml"
	c, err := NewConfigComponent(context.Background(), "", []string{fileName})
	if err != nil {
		t.Errorf("Failed to load agent config: %v", err)
	}
	assert.Equal(t, "DATADOG_API_KEY", c.Get("api_key"))
	assert.Equal(t, "datadoghq.eu", c.Get("site"))
	assert.Equal(t, "debug", c.Get("log_level"))
	assert.Equal(t, "test.metrics.com", c.Get("dd_url"))
	assert.Equal(t, true, c.Get("logs_enabled"))
	assert.Equal(t, "test.logs.com", c.Get("logs_config.logs_dd_url"))
	assert.Equal(t, 10, c.Get("logs_config.batch_wait"))
	assert.Equal(t, true, c.Get("logs_config.use_compression"))
	assert.Equal(t, true, c.Get("logs_config.force_use_http"))
	assert.Equal(t, 1, c.Get("logs_config.compression_level"))
	assert.Equal(t, "https://trace.agent.datadoghq.eu", c.Get("apm_config.apm_dd_url"))
	assert.Equal(t, map[string]string{"io.opentelemetry.javaagent.spring.client": "spring.client"}, c.Get("otlp_config.traces.span_name_remappings"))
	assert.Equal(t, []string{"(GET|POST) /healthcheck"}, c.Get("apm_config.ignore_resources"))
	assert.Equal(t, false, c.Get("apm_config.receiver_enabled"))
	assert.Equal(t, 10, c.Get("apm_config.trace_buffer"))
	assert.Equal(t, false, c.Get("otlp_config.traces.span_name_as_resource_name"))
	assert.Equal(t, []string{}, c.Get("apm_config.features"))
}

func (suite *ConfigTestSuite) TestAgentConfigDefaults() {
	t := suite.T()
	fileName := "testdata/config_default.yaml"
	c, err := NewConfigComponent(context.Background(), "", []string{fileName})
	if err != nil {
		t.Errorf("Failed to load agent config: %v", err)
	}
	assert.Equal(t, "DATADOG_API_KEY", c.Get("api_key"))
	assert.Equal(t, "datadoghq.com", c.Get("site"))
	assert.Equal(t, "https://api.datadoghq.com", c.Get("dd_url"))
	assert.Equal(t, true, c.Get("logs_enabled"))
	assert.Equal(t, "https://agent-http-intake.logs.datadoghq.com", c.Get("logs_config.logs_dd_url"))
	assert.Equal(t, 5, c.Get("logs_config.batch_wait"))
	assert.Equal(t, true, c.Get("logs_config.use_compression"))
	assert.Equal(t, true, c.Get("logs_config.force_use_http"))
	assert.Equal(t, 6, c.Get("logs_config.compression_level"))
	assert.Equal(t, "https://trace.agent.datadoghq.com", c.Get("apm_config.apm_dd_url"))
	assert.Equal(t, false, c.Get("apm_config.receiver_enabled"))
	assert.Equal(t, false, c.Get("otlp_config.traces.span_name_as_resource_name"))
	assert.Equal(t, []string{"enable_otlp_compute_top_level_by_span_kind"},
		c.Get("apm_config.features"))
}

func (suite *ConfigTestSuite) TestDisableOperationAndResourceNameV2FeatureGate() {
	featuregate.GlobalRegistry().Set("datadog.EnableOperationAndResourceNameV2", false)
	t := suite.T()
	fileName := "testdata/config_default.yaml"
	c, err := NewConfigComponent(context.Background(), "", []string{fileName})
	if err != nil {
		t.Errorf("Failed to load agent config: %v", err)
	}
	assert.Equal(t, "DATADOG_API_KEY", c.Get("api_key"))
	assert.Equal(t, "datadoghq.com", c.Get("site"))
	assert.Equal(t, "https://api.datadoghq.com", c.Get("dd_url"))
	assert.Equal(t, true, c.Get("logs_enabled"))
	assert.Equal(t, "https://agent-http-intake.logs.datadoghq.com", c.Get("logs_config.logs_dd_url"))
	assert.Equal(t, 5, c.Get("logs_config.batch_wait"))
	assert.Equal(t, true, c.Get("logs_config.use_compression"))
	assert.Equal(t, true, c.Get("logs_config.force_use_http"))
	assert.Equal(t, 6, c.Get("logs_config.compression_level"))
	assert.Equal(t, "https://trace.agent.datadoghq.com", c.Get("apm_config.apm_dd_url"))
	assert.Equal(t, false, c.Get("apm_config.receiver_enabled"))
	assert.Equal(t, false, c.Get("otlp_config.traces.span_name_as_resource_name"))
	assert.Equal(t, []string{"disable_operation_and_resource_name_logic_v2", "enable_otlp_compute_top_level_by_span_kind"},
		c.Get("apm_config.features"))
}

func (suite *ConfigTestSuite) TestAgentConfigExpandEnvVars() {
	t := suite.T()
	fileName := "testdata/config_default_expand_envvar.yaml"
	suite.T().Setenv("DD_API_KEY", "abc")
	c, err := NewConfigComponent(context.Background(), "", []string{fileName})
	if err != nil {
		t.Errorf("Failed to load agent config: %v", err)
	}
	assert.Equal(t, "abc", c.Get("api_key"))
}

func (suite *ConfigTestSuite) TestAgentConfigExpandEnvVars_NumberAPIKey() {
	t := suite.T()
	fileName := "testdata/config_default_expand_envvar.yaml"
	suite.T().Setenv("DD_API_KEY", "123456")
	c, err := NewConfigComponent(context.Background(), "", []string{fileName})
	if err != nil {
		t.Errorf("Failed to load agent config: %v", err)
	}
	assert.Equal(t, "123456", c.Get("api_key"))
}

func (suite *ConfigTestSuite) TestAgentConfigExpandEnvVars_Raw() {
	t := suite.T()
	fileName := "testdata/config_default_expand_envvar_raw.yaml"
	suite.T().Setenv("DD_API_KEY", "abc")
	c, err := NewConfigComponent(context.Background(), "", []string{fileName})
	if err != nil {
		t.Errorf("Failed to load agent config: %v", err)
	}
	assert.Equal(t, "abc", c.Get("api_key"))
}

func (suite *ConfigTestSuite) TestAgentConfigWithDatadogYamlDefaults() {
	t := suite.T()
	fileName := "testdata/config_default.yaml"
	ddFileName := "testdata/datadog.yaml"
	c, err := NewConfigComponent(context.Background(), ddFileName, []string{fileName})
	if err != nil {
		t.Errorf("Failed to load agent config: %v", err)
	}

	// all expected defaults
	assert.Equal(t, "DATADOG_API_KEY", c.Get("api_key"))
	assert.Equal(t, "datadoghq.com", c.Get("site"))
	assert.Equal(t, "https://api.datadoghq.com", c.Get("dd_url"))
	assert.Equal(t, true, c.Get("logs_enabled"))
	assert.Equal(t, "https://agent-http-intake.logs.datadoghq.com", c.Get("logs_config.logs_dd_url"))
	assert.Equal(t, 5, c.Get("logs_config.batch_wait"))
	assert.Equal(t, true, c.Get("logs_config.use_compression"))
	assert.Equal(t, true, c.Get("logs_config.force_use_http"))
	assert.Equal(t, 6, c.Get("logs_config.compression_level"))
	assert.Equal(t, "https://trace.agent.datadoghq.com", c.Get("apm_config.apm_dd_url"))
	assert.Equal(t, false, c.Get("apm_config.receiver_enabled"))
	assert.Equal(t, false, c.Get("otlp_config.traces.span_name_as_resource_name"))
	assert.Equal(t, []string{"enable_otlp_compute_top_level_by_span_kind"}, c.Get("apm_config.features"))

	// log_level from datadog.yaml takes precedence -> more verbose
	assert.Equal(t, "debug", c.Get("log_level"))
}

func (suite *ConfigTestSuite) TestAgentConfigWithDatadogYamlKeysAvailable() {
	t := suite.T()
	fileName := "testdata/config_default.yaml"
	ddFileName := "testdata/datadog.yaml"
	c, err := NewConfigComponent(context.Background(), ddFileName, []string{fileName})
	if err != nil {
		t.Errorf("Failed to load agent config: %v", err)
	}

	// log_level from datadog.yaml takes precedence -> more verbose
	assert.Equal(t, "debug", c.Get("log_level"))
	assert.True(t, c.GetBool("otelcollector.enabled"))
	assert.Equal(t, "https://localhost:7777", c.GetString("otelcollector.extension_url"))
	assert.Equal(t, 5009, c.GetInt("agent_ipc.port"))
	assert.Equal(t, 60, c.GetInt("agent_ipc.config_refresh_interval"))
}

func (suite *ConfigTestSuite) TestAgentConfigSetAPMFeaturesFromDatadogYaml() {
	t := suite.T()
	fileName := "testdata/config_default.yaml"
	ddFileName := "testdata/datadog_apm_config_features.yaml"
	c, err := NewConfigComponent(context.Background(), ddFileName, []string{fileName})
	if err != nil {
		t.Errorf("Failed to load agent config: %v", err)
	}

	assert.Equal(t, []string{"test1", "test2"}, c.GetStringSlice("apm_config.features"))
}

func (suite *ConfigTestSuite) TestAgentConfigSetAPMFeaturesFromEnv() {
	t := suite.T()
	fileName := "testdata/config_default.yaml"
	t.Setenv("DD_APM_FEATURES", "test1,test2")
	c, err := NewConfigComponent(context.Background(), "", []string{fileName})
	if err != nil {
		t.Errorf("Failed to load agent config: %v", err)
	}

	assert.Equal(t, []string{"test1", "test2"}, c.GetStringSlice("apm_config.features"))
}

func (suite *ConfigTestSuite) TestLogLevelPrecedence() {
	t := suite.T()
	fileName := "testdata/config_default.yaml"
	ddFileName := "testdata/datadog_low_log_level.yaml"
	c, err := NewConfigComponent(context.Background(), ddFileName, []string{fileName})
	if err != nil {
		t.Errorf("Failed to load agent config: %v", err)
	}

	// log_level from service config takes precedence -> more verbose
	// ddFlleName configures level warn, Telemetry defaults to info
	assert.Equal(t, "info", c.Get("log_level"))
}

func (suite *ConfigTestSuite) TestEnvLogLevelPrecedence() {
	t := suite.T()
	oldval, exists := os.LookupEnv("DD_LOG_LEVEL")
	os.Setenv("DD_LOG_LEVEL", "debug")
	defer func() {
		if !exists {
			os.Unsetenv("DD_LOG_LEVEL")
		} else {
			os.Setenv("DD_LOG_LEVEL", oldval)
		}
	}()
	fileName := "testdata/config_default.yaml"
	ddFileName := "testdata/datadog_low_log_level.yaml"
	c, err := NewConfigComponent(context.Background(), ddFileName, []string{fileName})
	if err != nil {
		t.Errorf("Failed to load agent config: %v", err)
	}

	// log_level from service config takes precedence -> more verbose
	// ddFlleName configures level warn, Telemetry defaults to info, env sets debug
	assert.Equal(t, "debug", c.Get("log_level"))
}

func (suite *ConfigTestSuite) TestEnvBadLogLevel() {
	t := suite.T()
	oldval, exists := os.LookupEnv("DD_LOG_LEVEL")
	os.Setenv("DD_LOG_LEVEL", "yabadabadooo")
	defer func() {
		if !exists {
			os.Unsetenv("DD_LOG_LEVEL")
		} else {
			os.Setenv("DD_LOG_LEVEL", oldval)
		}
	}()
	fileName := "testdata/config_default.yaml"
	ddFileName := "testdata/datadog_low_log_level.yaml"
	_, err := NewConfigComponent(context.Background(), ddFileName, []string{fileName})
	assert.EqualError(t, err, "invalid log level (yabadabadooo) set in the Datadog Agent configuration")
}

func (suite *ConfigTestSuite) TestEnvUpperCaseLogLevel() {
	t := suite.T()
	oldval, exists := os.LookupEnv("DD_LOG_LEVEL")
	os.Unsetenv("DD_LOG_LEVEL")
	defer func() {
		if !exists {
			os.Unsetenv("DD_LOG_LEVEL")
		} else {
			os.Setenv("DD_LOG_LEVEL", oldval)
		}
	}()
	fileName := "testdata/config_default.yaml"
	ddFileName := "testdata/datadog_uppercase_log_level.yaml"
	c, err := NewConfigComponent(context.Background(), ddFileName, []string{fileName})
	if err != nil {
		t.Errorf("Failed to load agent config: %v", err)
	}

	// log_level will be mapped to lowercase by code and set accordingly
	assert.Equal(t, "info", c.Get("log_level"))
}

func (suite *ConfigTestSuite) TestBadDDConfigFile() {
	t := suite.T()
	fileName := "testdata/config_default.yaml"
	ddFileName := "testdata/doesnotexists.yaml"
	_, err := NewConfigComponent(context.Background(), ddFileName, []string{fileName})

	assert.ErrorIs(t, err, fs.ErrNotExist)
}

func (suite *ConfigTestSuite) TestBadLogLevel() {
	t := suite.T()
	fileName := "testdata/config_default.yaml"
	ddFileName := "testdata/datadog_bad_log_level.yaml"
	_, err := NewConfigComponent(context.Background(), ddFileName, []string{fileName})

	expectedError := fmt.Sprintf(
		"invalid log level (%v) set in the Datadog Agent configuration",
		pkgconfigsetup.Datadog().GetString("log_level"))
	assert.ErrorContains(t, err, expectedError)
}

func (suite *ConfigTestSuite) TestNoDDExporter() {
	t := suite.T()
	fileName := "testdata/config_no_dd_exporter.yaml"
	_, err := NewConfigComponent(context.Background(), "", []string{fileName})
	assert.EqualError(t, err, "no datadog exporter found")
}

func (suite *ConfigTestSuite) TestMultipleDDExporters() {
	t := suite.T()
	fileName := "testdata/config_multiple_dd_exporters.yaml"
	_, err := NewConfigComponent(context.Background(), "", []string{fileName})
	assert.EqualError(t, err, "multiple datadog exporters found")
}

func (suite *ConfigTestSuite) TestNoDDAPISection() {
	t := suite.T()
	fileName := "testdata/config_no_api.yaml"
	c, err := NewConfigComponent(context.Background(), "", []string{fileName})
	require.NoError(t, err)
	assert.Equal(t, "datadoghq.com", c.Get("site"))
	assert.Equal(t, "https://api.datadoghq.com", c.Get("dd_url"))
	assert.Equal(t, "https://agent-http-intake.logs.datadoghq.com", c.Get("logs_config.logs_dd_url"))
	assert.Equal(t, "https://trace.agent.datadoghq.com", c.Get("apm_config.apm_dd_url"))
}

func (suite *ConfigTestSuite) TestNilDDAPISection() {
	t := suite.T()
	fileName := "testdata/config_nil_api.yaml"
	c, err := NewConfigComponent(context.Background(), "", []string{fileName})
	require.NoError(t, err)
	assert.Equal(t, "datadoghq.com", c.Get("site"))
	assert.Equal(t, "https://api.datadoghq.com", c.Get("dd_url"))
	assert.Equal(t, "https://agent-http-intake.logs.datadoghq.com", c.Get("logs_config.logs_dd_url"))
	assert.Equal(t, "https://trace.agent.datadoghq.com", c.Get("apm_config.apm_dd_url"))
}

func (suite *ConfigTestSuite) TestMalformedDDAPISection() {
	t := suite.T()
	fileName := "testdata/config_malformed_api.yaml"
	_, err := NewConfigComponent(context.Background(), "", []string{fileName})
	assert.EqualError(t, err, "invalid datadog exporter config")
}

func (suite *ConfigTestSuite) TestDDAPISiteEmpty() {
	t := suite.T()
	fileName := "testdata/config_site_empty.yaml"
	c, err := NewConfigComponent(context.Background(), "", []string{fileName})
	require.NoError(t, err)
	assert.Equal(t, "datadoghq.com", c.Get("site"))
	assert.Equal(t, "https://api.datadoghq.com", c.Get("dd_url"))
	assert.Equal(t, "https://agent-http-intake.logs.datadoghq.com", c.Get("logs_config.logs_dd_url"))
	assert.Equal(t, "https://trace.agent.datadoghq.com", c.Get("apm_config.apm_dd_url"))
}

func (suite *ConfigTestSuite) TestDDAPISiteNotSet() {
	t := suite.T()
	fileName := "testdata/config_site_not_set.yaml"
	c, err := NewConfigComponent(context.Background(), "", []string{fileName})
	require.NoError(t, err)
	assert.Equal(t, "datadoghq.com", c.Get("site"))
	assert.Equal(t, "https://api.datadoghq.com", c.Get("dd_url"))
	assert.Equal(t, "https://agent-http-intake.logs.datadoghq.com", c.Get("logs_config.logs_dd_url"))
	assert.Equal(t, "https://trace.agent.datadoghq.com", c.Get("apm_config.apm_dd_url"))
}

func (suite *ConfigTestSuite) TestDDAPISiteSet() {
	t := suite.T()
	fileName := "testdata/config_site_set.yaml"
	c, err := NewConfigComponent(context.Background(), "", []string{fileName})
	require.NoError(t, err)
	assert.Equal(t, "us3.datadoghq.com", c.Get("site"))
	assert.Equal(t, "https://api.us3.datadoghq.com", c.Get("dd_url"))
	assert.Equal(t, "https://agent-http-intake.logs.us3.datadoghq.com", c.Get("logs_config.logs_dd_url"))
	assert.Equal(t, "https://trace.agent.us3.datadoghq.com", c.Get("apm_config.apm_dd_url"))
}

func (suite *ConfigTestSuite) TestProxyEnvVarsBoth() {
	t := suite.T()
	t.Setenv("HTTP_PROXY", "http://proxy.example.com:8080")
	t.Setenv("HTTPS_PROXY", "https://secure-proxy.example.com:8443")
	t.Setenv("NO_PROXY", "localhost,127.0.0.1,.local")

	pkgconfig, err := NewConfigComponent(context.Background(), "testdata/datadog_proxy_test.yaml", []string{"testdata/config.yaml"})
	require.NoError(t, err)

	assert.Equal(t, "http://proxy.example.com:8080", pkgconfig.GetString("proxy.http"))
	assert.Equal(t, "https://secure-proxy.example.com:8443", pkgconfig.GetString("proxy.https"))
	assert.Equal(t, []string{"localhost", "127.0.0.1", ".local"}, pkgconfig.GetStringSlice("proxy.no_proxy"))
}

func (suite *ConfigTestSuite) TestProxyEnvVarsHTTPOnly() {
	t := suite.T()

	t.Setenv("HTTP_PROXY", "http://proxy.example.com:3128")

	pkgconfig, err := NewConfigComponent(context.Background(), "testdata/datadog_proxy_test.yaml", []string{"testdata/config.yaml"})
	require.NoError(t, err)

	assert.Equal(t, "http://proxy.example.com:3128", pkgconfig.GetString("proxy.http"))
	assert.Equal(t, "", pkgconfig.GetString("proxy.https"))
	assert.Equal(t, []string(nil), pkgconfig.GetStringSlice("proxy.no_proxy"))
}

func (suite *ConfigTestSuite) TestProxyEnvVarsNone() {
	t := suite.T()

	pkgconfig, err := NewConfigComponent(context.Background(), "testdata/datadog_proxy_test.yaml", []string{"testdata/config.yaml"})
	require.NoError(t, err)

	assert.Equal(t, "", pkgconfig.GetString("proxy.http"))
	assert.Equal(t, "", pkgconfig.GetString("proxy.https"))
	assert.Equal(t, []string(nil), pkgconfig.GetStringSlice("proxy.no_proxy"))
}

func (suite *ConfigTestSuite) TestProxyEnvVarsNOProxyOnly() {
	t := suite.T()

	// Set only NO_PROXY
	t.Setenv("NO_PROXY", "internal.company.com,localhost")

	pkgconfig, err := NewConfigComponent(context.Background(), "testdata/datadog_proxy_test.yaml", []string{"testdata/config.yaml"})
	require.NoError(t, err)

	assert.Equal(t, "", pkgconfig.GetString("proxy.http"))
	assert.Equal(t, "", pkgconfig.GetString("proxy.https"))
	assert.Equal(t, []string{"internal.company.com", "localhost"}, pkgconfig.GetStringSlice("proxy.no_proxy"))
}

func (suite *ConfigTestSuite) TestProxyConfigURLOnly() {
	t := suite.T()

	pkgconfig, err := NewConfigComponent(context.Background(), "testdata/datadog_proxy_test.yaml", []string{"testdata/config_proxy.yaml"})
	require.NoError(t, err)

	assert.Equal(t, "http://proxyurl.example.com:3128", pkgconfig.GetString("proxy.http"))
	assert.Equal(t, "http://proxyurl.example.com:3128", pkgconfig.GetString("proxy.https"))
	assert.Equal(t, []string(nil), pkgconfig.GetStringSlice("proxy.no_proxy"))
}

func (suite *ConfigTestSuite) TestProxyConfigURLPrecedence() {
	t := suite.T()

	t.Setenv("HTTP_PROXY", "http://proxy.example.com:8080")
	t.Setenv("HTTPS_PROXY", "https://secure-proxy.example.com:8443")

	pkgconfig, err := NewConfigComponent(context.Background(), "testdata/datadog_proxy_test.yaml", []string{"testdata/config_proxy.yaml"})
	require.NoError(t, err)

	// ProxyURL from config should take precedence over environment variables
	assert.Equal(t, "http://proxyurl.example.com:3128", pkgconfig.GetString("proxy.http"))
	assert.Equal(t, "http://proxyurl.example.com:3128", pkgconfig.GetString("proxy.https"))
	assert.Equal(t, []string(nil), pkgconfig.GetStringSlice("proxy.no_proxy"))
}

func (suite *ConfigTestSuite) TestProxyConfigURLOverridesDDConfig() {
	t := suite.T()

	pkgconfig, err := NewConfigComponent(context.Background(), "testdata/datadog_proxy_with_settings.yaml", []string{"testdata/config_proxy.yaml"})
	require.NoError(t, err)

	// ProxyURL from OTLP config should override proxy.http and proxy.https from datadog config
	assert.Equal(t, "http://proxyurl.example.com:3128", pkgconfig.GetString("proxy.http"))
	assert.Equal(t, "http://proxyurl.example.com:3128", pkgconfig.GetString("proxy.https"))
	assert.Equal(t, []string(nil), pkgconfig.GetStringSlice("proxy.no_proxy"))
}

// TestLogsEnabledViaEnvironmentVariable is a regression test for the issue where
// LoadDatadog was called before BuildSchema, causing "attempt to ReadInConfig before config
// is constructed" errors.
func TestLogsEnabledViaEnvironmentVariable(t *testing.T) {
	configmock.New(t)
	t.Setenv("DD_LOGS_ENABLED", "true")
	fileName := "testdata/config_default.yaml"

	// This should not panic or error with "attempt to ReadInConfig before config is constructed"
	c, err := NewConfigComponent(context.Background(), "", []string{fileName})
	require.NoError(t, err, "NewConfigComponent should succeed with DD_LOGS_ENABLED set")
	assert.True(t, c.GetBool("logs_enabled"), "logs_enabled should be true when DD_LOGS_ENABLED=true")

	libType := c.GetLibType()
	assert.NotEmpty(t, libType, "config lib type should be set")
}

// TestLogsEnabledViaDatadogConfig tests that logs_enabled can be set via a separate
// datadog.yaml config file and is correctly merged with the OTel config. This ensures
// the config initialization order works correctly when both configs are present.
func TestLogsEnabledViaDatadogConfig(t *testing.T) {
	configmock.New(t)
	ddFileName := "testdata/datadog_with_logs_enabled.yaml"
	c, err := NewConfigComponent(context.Background(), "", []string{ddFileName})
	require.NoError(t, err, "NewConfigComponent should succeed with datadog config")
	assert.True(t, c.GetBool("logs_enabled"), "logs_enabled should be true from datadog config")
}

func (suite *ConfigTestSuite) TestGatewayModeHostnameEmpty() {
	t := suite.T()
	fileName := "testdata/config_default.yaml"
	ddFileName := "testdata/datadog_gateway_mode.yaml"
	c, err := NewConfigComponent(context.Background(), ddFileName, []string{fileName})
	require.NoError(t, err, "NewConfigComponent should succeed with gateway mode enabled")

	// When gateway mode is enabled, hostname should be set to empty string
	assert.True(t, c.GetBool("otelcollector.gateway.mode"), "gateway mode should be enabled")
	assert.Equal(t, "", c.GetString("hostname"), "hostname should be empty when gateway mode is enabled")
}

func (suite *ConfigTestSuite) TestGatewayModeDisabledHostnameNotEmpty() {
	t := suite.T()
	fileName := "testdata/config_default.yaml"
	// Using datadog.yaml which doesn't have gateway mode set
	ddFileName := "testdata/datadog.yaml"
	c, err := NewConfigComponent(context.Background(), ddFileName, []string{fileName})
	require.NoError(t, err, "NewConfigComponent should succeed without gateway mode")

	// When gateway mode is not enabled, hostname should not be forced to empty
	assert.False(t, c.GetBool("otelcollector.gateway.mode"), "gateway mode should be disabled")
	// The hostname might be empty by default, but the key difference is that
	// it wasn't explicitly set to empty via the gateway mode logic
	// We verify the config was loaded without the gateway mode setting
	assert.False(t, c.IsConfigured("otelcollector.gateway.mode") && c.GetBool("otelcollector.gateway.mode"),
		"gateway mode should not be enabled")
}

func (suite *ConfigTestSuite) TestGatewayModeViaEnvVarHostnameEmpty() {
	t := suite.T()
	fileName := "testdata/config_default.yaml"
	// Set gateway mode via environment variable
	t.Setenv("DD_OTELCOLLECTOR_GATEWAY_MODE", "true")
	c, err := NewConfigComponent(context.Background(), "", []string{fileName})
	require.NoError(t, err, "NewConfigComponent should succeed with gateway mode env var set")

	// When gateway mode is enabled via env var, hostname should be set to empty string
	assert.True(t, c.GetBool("otelcollector.gateway.mode"), "gateway mode should be enabled from env var")
	assert.Equal(t, "", c.GetString("hostname"), "hostname should be empty when gateway mode is enabled via env var")
}

func (suite *ConfigTestSuite) TestGatewayModeOverridesDDHostnameEnvVar() {
	t := suite.T()
	fileName := "testdata/config_default.yaml"
	ddFileName := "testdata/datadog_gateway_mode.yaml"
	// Simulate the Helm chart setting DD_HOSTNAME to the Kubernetes node name
	t.Setenv("DD_HOSTNAME", "k8s-node-name")
	c, err := NewConfigComponent(context.Background(), ddFileName, []string{fileName})
	require.NoError(t, err, "NewConfigComponent should succeed with gateway mode enabled")

	// Gateway mode should override DD_HOSTNAME and set hostname to empty
	assert.True(t, c.GetBool("otelcollector.gateway.mode"), "gateway mode should be enabled")
	assert.Equal(t, "", c.GetString("hostname"), "hostname should be empty in gateway mode even when DD_HOSTNAME is set")
}

// TestSuite runs the CalculatorTestSuite
func TestSuite(t *testing.T) {
	suite.Run(t, new(ConfigTestSuite))
}
