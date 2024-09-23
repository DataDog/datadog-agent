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
	"strings"
	"testing"

	pkgconfigmodel "github.com/DataDog/datadog-agent/pkg/config/model"
	pkgconfigsetup "github.com/DataDog/datadog-agent/pkg/config/setup"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/suite"
)

type ConfigTestSuite struct {
	suite.Suite
}

func (suite *ConfigTestSuite) SetupTest() {
	datadog := pkgconfigmodel.NewConfig("datadog", "DD", strings.NewReplacer(".", "_"))
	pkgconfigsetup.SetDatadog(datadog)
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
	assert.Equal(t, nil, c.Get("apm_config.features"))
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
	assert.Equal(t, true, c.Get("otlp_config.traces.span_name_as_resource_name"))
	assert.Equal(t, []string{"enable_otlp_compute_top_level_by_span_kind"}, c.Get("apm_config.features"))
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
	assert.Equal(t, true, c.Get("otlp_config.traces.span_name_as_resource_name"))
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
	assert.Error(t, err)
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

// TestSuite runs the CalculatorTestSuite
func TestSuite(t *testing.T) {
	suite.Run(t, new(ConfigTestSuite))
}
