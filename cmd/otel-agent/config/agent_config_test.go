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
	"path/filepath"
	"runtime"
	"testing"

	"go.opentelemetry.io/collector/confmap"
	"go.opentelemetry.io/collector/featuregate"

	configmock "github.com/DataDog/datadog-agent/pkg/config/mock"
	"github.com/DataDog/datadog-agent/pkg/util/defaultpaths"

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
	// LoadProxyFromEnv treats a present-but-empty var as set, so these must be
	// unset rather than set to "" - otherwise DD_PROXY_* short-circuits the
	// HTTP_PROXY/HTTPS_PROXY/NO_PROXY fallback even when a test sets those.
	// The lowercase variants are cleared too since LoadProxyFromEnv falls
	// back to them when the uppercase ones aren't set.
	os.Unsetenv("HTTP_PROXY")
	os.Unsetenv("http_proxy")
	os.Unsetenv("HTTPS_PROXY")
	os.Unsetenv("https_proxy")
	os.Unsetenv("NO_PROXY")
	os.Unsetenv("no_proxy")
	os.Unsetenv("DD_PROXY_HTTP")
	os.Unsetenv("DD_PROXY_HTTPS")
	os.Unsetenv("DD_PROXY_NO_PROXY")
}

func TestNoURIsProvided(t *testing.T) {
	_, err := NewConfigComponent(context.Background(), "", []string{}, nil)
	assert.Error(t, err, "no URIs provided for configs")
}

// TestDDOTSeriesV3EnabledForDefaultEndpoint verifies DDOT opts its default Datadog series
// endpoint in to the v3 metrics intake. The default endpoint is api.<site>, which
// datadog_only's IsDatadogURL does not recognize, so without an explicit per-endpoint
// override series would silently stay on v2. The override must be keyed by the exact dd_url
// the forwarder resolver reports as its config name.
func TestDDOTSeriesV3EnabledForDefaultEndpoint(t *testing.T) {
	configmock.New(t)
	// CI runners (Fabric Egress Gateway) set HTTP(S)_PROXY; DDOT keeps proxied Agents on
	// v2, so clear proxy env to exercise the no-proxy enable path. DD_PROXY_* shadow HTTP(S)_PROXY.
	t.Setenv("DD_PROXY_HTTP", "")
	t.Setenv("DD_PROXY_HTTPS", "")
	c, err := NewConfigComponent(context.Background(), "", []string{"testdata/config_default.yaml"}, nil)
	require.NoError(t, err)

	ddURL := c.GetString("dd_url")
	require.Equal(t, "https://api.datadoghq.com", ddURL, "DDOT default metrics endpoint")

	endpoints := c.GetStringMapString("use_v3_api.series.endpoints")
	assert.Equal(t, "true", endpoints[ddURL],
		"DDOT must opt the default api.<site> endpoint in to v3, keyed by dd_url")

	// The global default stays datadog_only, so custom (non-Datadog) endpoints are not
	// forced onto v3.
	assert.Equal(t, "datadog_only", c.GetString("use_v3_api.series.enabled"))
}

// TestDDOTSeriesV3NotForcedForCustomEndpoint verifies a non-default (custom / proxy) metrics
// endpoint is NOT force-opted into v3: it falls through to the global datadog_only default,
// preserving the safeguard that keeps non-Datadog destinations on v2.
func TestDDOTSeriesV3NotForcedForCustomEndpoint(t *testing.T) {
	configmock.New(t)
	c, err := NewConfigComponent(context.Background(), "", []string{"testdata/config_custom_metrics_endpoint.yaml"}, nil)
	require.NoError(t, err)

	ddURL := c.GetString("dd_url")
	require.Equal(t, "https://custom-proxy.example.test", ddURL)

	endpoints := c.GetStringMapString("use_v3_api.series.endpoints")
	_, present := endpoints[ddURL]
	assert.False(t, present, "custom endpoint must not be force-opted into v3")
	assert.Equal(t, "datadog_only", c.GetString("use_v3_api.series.enabled"))
}

// TestDDOTSeriesV3NotForcedForNonDatadogSite verifies a non-Datadog site (private or proxy)
// is NOT force-opted into v3, even though its default derived endpoint has the same
// https://api.<site> shape as a real Datadog default. IsDatadogURL only recognizes Datadog
// domains, so the override is withheld and the destination stays on the datadog_only default
// (v2) — guarding against shipping an unsupported v3 payload to a non-Datadog intake.
func TestDDOTSeriesV3NotForcedForNonDatadogSite(t *testing.T) {
	configmock.New(t)
	c, err := NewConfigComponent(context.Background(), "", []string{"testdata/config_non_datadog_site.yaml"}, nil)
	require.NoError(t, err)

	ddURL := c.GetString("dd_url")
	require.Equal(t, "https://api.mycompany.internal", ddURL,
		"exporter still derives api.<site> for a non-Datadog site")

	endpoints := c.GetStringMapString("use_v3_api.series.endpoints")
	_, present := endpoints[ddURL]
	assert.False(t, present, "a non-Datadog site must not be force-opted into v3")
	assert.Equal(t, "datadog_only", c.GetString("use_v3_api.series.enabled"))
}

// TestDDOTSeriesV3NotForcedBehindProxy verifies a proxied Agent is NOT force-opted into v3,
// even on a default Datadog endpoint. A forwarding proxy may inflate/recompress payloads and
// reject the v3 format (and custom proxy/pipeline endpoints may not support v3 yet), so per
// the v3 migration RFC a proxied deployment stays on v2 under datadog_only.
func TestDDOTSeriesV3NotForcedBehindProxy(t *testing.T) {
	configmock.New(t)
	t.Setenv("DD_PROXY_HTTPS", "http://proxy.corp.internal:3128")
	c, err := NewConfigComponent(context.Background(), "", []string{"testdata/config_default.yaml"}, nil)
	require.NoError(t, err)

	require.Equal(t, "https://api.datadoghq.com", c.GetString("dd_url"))
	require.NotEmpty(t, c.GetString("proxy.https"), "proxy env var must be loaded")

	endpoints := c.GetStringMapString("use_v3_api.series.endpoints")
	_, present := endpoints[c.GetString("dd_url")]
	assert.False(t, present, "a proxied Agent must not be force-opted into v3")
	assert.Equal(t, "datadog_only", c.GetString("use_v3_api.series.enabled"))
}

// TestDDOTSeriesV3RespectsExplicitOptOut verifies DD_USE_V3_API_SERIES_ENABLED=false keeps
// the default api.<site> endpoint on v2: the per-endpoint opt-in is only injected on the
// datadog_only default, so an explicit operator kill-switch is honored (a per-endpoint entry
// would otherwise outrank use_v3_api.series.enabled).
func TestDDOTSeriesV3RespectsExplicitOptOut(t *testing.T) {
	configmock.New(t)
	t.Setenv("DD_USE_V3_API_SERIES_ENABLED", "false")
	c, err := NewConfigComponent(context.Background(), "", []string{"testdata/config_default.yaml"}, nil)
	require.NoError(t, err)

	require.Equal(t, "https://api.datadoghq.com", c.GetString("dd_url"))
	require.Equal(t, "false", c.GetString("use_v3_api.series.enabled"))

	endpoints := c.GetStringMapString("use_v3_api.series.endpoints")
	_, present := endpoints[c.GetString("dd_url")]
	assert.False(t, present, "explicit enabled=false must not be overridden by the per-endpoint opt-in")
}

// TestDDOTSketchesV3BetaShadowDisabled verifies DDOT opts out of the v3beta sketches
// shadow, which defaults to a non-zero sample rate in the core Agent. SourceAgentRuntime
// outranks SourceEnvVar, so a colocated core Agent's DD_ env vars cannot re-enable it.
func TestDDOTSketchesV3BetaShadowDisabled(t *testing.T) {
	configmock.New(t)
	t.Setenv("DD_SERIALIZER_EXPERIMENTAL_USE_V3_API_SKETCHES_SHADOW_SAMPLE_RATE", "1")
	c, err := NewConfigComponent(context.Background(), "", []string{"testdata/config_default.yaml"})
	require.NoError(t, err)

	assert.Zero(t, c.GetFloat64("serializer_experimental_use_v3_api.sketches.shadow_sample_rate"))
}

func (suite *ConfigTestSuite) TestAgentConfig() {
	t := suite.T()
	fileName := "testdata/config.yaml"
	c, err := NewConfigComponent(context.Background(), "", []string{fileName}, nil)
	if err != nil {
		t.Errorf("Failed to load agent config: %v", err)
	}
	assert.Equal(t, "DATADOG_API_KEY", c.Get("api_key"))
	assert.Equal(t, "datadoghq.eu", c.Get("site"))
	assert.Equal(t, "debug", c.Get("log_level"))
	assert.Equal(t, "test.metrics.com", c.Get("dd_url"))
	assert.Equal(t, true, c.Get("logs_enabled"))
	assert.Equal(t, "test.logs.com", c.Get("logs_config.logs_dd_url"))
	assert.Equal(t, float64(10), c.Get("logs_config.batch_wait"))
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
	c, err := NewConfigComponent(context.Background(), "", []string{fileName}, nil)
	if err != nil {
		t.Errorf("Failed to load agent config: %v", err)
	}
	assert.Equal(t, "DATADOG_API_KEY", c.Get("api_key"))
	assert.Equal(t, "datadoghq.com", c.Get("site"))
	assert.Equal(t, "https://api.datadoghq.com", c.Get("dd_url"))
	assert.Equal(t, true, c.Get("logs_enabled"))
	assert.Equal(t, "https://agent-http-intake.logs.datadoghq.com", c.Get("logs_config.logs_dd_url"))
	assert.Equal(t, float64(5), c.Get("logs_config.batch_wait"))
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
	c, err := NewConfigComponent(context.Background(), "", []string{fileName}, nil)
	if err != nil {
		t.Errorf("Failed to load agent config: %v", err)
	}
	assert.Equal(t, "DATADOG_API_KEY", c.Get("api_key"))
	assert.Equal(t, "datadoghq.com", c.Get("site"))
	assert.Equal(t, "https://api.datadoghq.com", c.Get("dd_url"))
	assert.Equal(t, true, c.Get("logs_enabled"))
	assert.Equal(t, "https://agent-http-intake.logs.datadoghq.com", c.Get("logs_config.logs_dd_url"))
	assert.Equal(t, float64(5), c.Get("logs_config.batch_wait"))
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
	c, err := NewConfigComponent(context.Background(), "", []string{fileName}, nil)
	if err != nil {
		t.Errorf("Failed to load agent config: %v", err)
	}
	assert.Equal(t, "abc", c.Get("api_key"))
}

func (suite *ConfigTestSuite) TestAgentConfigExpandEnvVars_NumberAPIKey() {
	t := suite.T()
	fileName := "testdata/config_default_expand_envvar.yaml"
	suite.T().Setenv("DD_API_KEY", "123456")
	c, err := NewConfigComponent(context.Background(), "", []string{fileName}, nil)
	if err != nil {
		t.Errorf("Failed to load agent config: %v", err)
	}
	assert.Equal(t, "123456", c.Get("api_key"))
}

func (suite *ConfigTestSuite) TestAgentConfigExpandEnvVars_Raw() {
	t := suite.T()
	fileName := "testdata/config_default_expand_envvar_raw.yaml"
	suite.T().Setenv("DD_API_KEY", "abc")
	c, err := NewConfigComponent(context.Background(), "", []string{fileName}, nil)
	if err != nil {
		t.Errorf("Failed to load agent config: %v", err)
	}
	assert.Equal(t, "abc", c.Get("api_key"))
}

func (suite *ConfigTestSuite) TestAgentConfigWithDatadogYamlDefaults() {
	t := suite.T()
	fileName := "testdata/config_default.yaml"
	ddFileName := "testdata/datadog.yaml"
	c, err := NewConfigComponent(context.Background(), ddFileName, []string{fileName}, nil)
	if err != nil {
		t.Errorf("Failed to load agent config: %v", err)
	}

	// all expected defaults
	assert.Equal(t, "DATADOG_API_KEY", c.Get("api_key"))
	assert.Equal(t, "datadoghq.com", c.Get("site"))
	assert.Equal(t, "https://api.datadoghq.com", c.Get("dd_url"))
	assert.Equal(t, true, c.Get("logs_enabled"))
	assert.Equal(t, "https://agent-http-intake.logs.datadoghq.com", c.Get("logs_config.logs_dd_url"))
	assert.Equal(t, float64(5), c.Get("logs_config.batch_wait"))
	assert.Equal(t, true, c.Get("logs_config.use_compression"))
	assert.Equal(t, true, c.Get("logs_config.force_use_http"))
	assert.Equal(t, 6, c.Get("logs_config.compression_level"))
	assert.Equal(t, "https://trace.agent.datadoghq.com", c.Get("apm_config.apm_dd_url"))
	assert.Equal(t, false, c.Get("apm_config.receiver_enabled"))
	assert.Equal(t, false, c.Get("otlp_config.traces.span_name_as_resource_name"))
	assert.Equal(t, []string{"enable_otlp_compute_top_level_by_span_kind"}, c.Get("apm_config.features"))
	// DDOT uses zstd (v3-compatible), so it inherits the global datadog_only v3 default.
	assert.Equal(t, "datadog_only", c.GetString("use_v3_api.series.enabled"))

	// log_level from datadog.yaml takes precedence -> more verbose
	assert.Equal(t, "debug", c.Get("log_level"))
}

func (suite *ConfigTestSuite) TestAgentConfigWithDatadogYamlKeysAvailable() {
	t := suite.T()
	fileName := "testdata/config_default.yaml"
	ddFileName := "testdata/datadog.yaml"
	c, err := NewConfigComponent(context.Background(), ddFileName, []string{fileName}, nil)
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

// TestStandaloneModeIgnoresCoreAgentIPCEnvVars reproduces OTAGENT-1149: a
// deployment tool that colocates otel-agent with a core Datadog Agent (e.g.
// the Datadog Operator's DaemonSet) injects that agent's env vars —
// DD_REMOTE_CONFIGURATION_ENABLED=true and DD_AGENT_IPC_CONFIG_REFRESH_INTERVAL
// — into the otel-agent container too. Standalone mode must win regardless,
// since there is no core agent IPC endpoint for the RC client or configsync
// to talk to.
func (suite *ConfigTestSuite) TestStandaloneModeIgnoresCoreAgentIPCEnvVars() {
	t := suite.T()
	t.Setenv("DD_OTEL_STANDALONE", "true")
	t.Setenv("DD_REMOTE_CONFIGURATION_ENABLED", "true")
	t.Setenv("DD_AGENT_IPC_CONFIG_REFRESH_INTERVAL", "60")

	c, err := NewConfigComponent(context.Background(), "", []string{"testdata/config.yaml"}, nil)
	require.NoError(t, err)

	assert.Equal(t, -1, c.GetInt("cmd_port"))
	assert.False(t, c.GetBool("remote_configuration.enabled"))
	assert.Equal(t, 0, c.GetInt("agent_ipc.config_refresh_interval"))
}

func (suite *ConfigTestSuite) TestAgentConfigSetAPMFeaturesFromDatadogYaml() {
	t := suite.T()
	fileName := "testdata/config_default.yaml"
	ddFileName := "testdata/datadog_apm_config_features.yaml"
	c, err := NewConfigComponent(context.Background(), ddFileName, []string{fileName}, nil)
	if err != nil {
		t.Errorf("Failed to load agent config: %v", err)
	}

	assert.Equal(t, []string{"test1", "test2"}, c.GetStringSlice("apm_config.features"))
}

func (suite *ConfigTestSuite) TestAgentConfigSetAPMFeaturesFromEnv() {
	t := suite.T()
	fileName := "testdata/config_default.yaml"
	t.Setenv("DD_APM_FEATURES", "test1,test2")
	c, err := NewConfigComponent(context.Background(), "", []string{fileName}, nil)
	if err != nil {
		t.Errorf("Failed to load agent config: %v", err)
	}

	assert.Equal(t, []string{"test1", "test2"}, c.GetStringSlice("apm_config.features"))
}

func (suite *ConfigTestSuite) TestLogLevelPrecedence() {
	t := suite.T()
	fileName := "testdata/config_default.yaml"
	ddFileName := "testdata/datadog_low_log_level.yaml"
	c, err := NewConfigComponent(context.Background(), ddFileName, []string{fileName}, nil)
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
	c, err := NewConfigComponent(context.Background(), ddFileName, []string{fileName}, nil)
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
	_, err := NewConfigComponent(context.Background(), ddFileName, []string{fileName}, nil)
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
	c, err := NewConfigComponent(context.Background(), ddFileName, []string{fileName}, nil)
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
	_, err := NewConfigComponent(context.Background(), ddFileName, []string{fileName}, nil)

	assert.ErrorIs(t, err, fs.ErrNotExist)
}

func (suite *ConfigTestSuite) TestBadLogLevel() {
	t := suite.T()
	fileName := "testdata/config_default.yaml"
	ddFileName := "testdata/datadog_bad_log_level.yaml"
	_, err := NewConfigComponent(context.Background(), ddFileName, []string{fileName}, nil)

	assert.EqualError(t, err, "invalid log level (yabadabadoo) set in the Datadog Agent configuration")
}

func (suite *ConfigTestSuite) TestNoDDExporter() {
	t := suite.T()
	fileName := "testdata/config_no_dd_exporter.yaml"
	_, err := NewConfigComponent(context.Background(), "", []string{fileName}, nil)
	assert.EqualError(t, err, "no datadog exporter found")
}

func (suite *ConfigTestSuite) TestMultipleDDExporters() {
	t := suite.T()
	fileName := "testdata/config_multiple_dd_exporters.yaml"
	_, err := NewConfigComponent(context.Background(), "", []string{fileName}, nil)
	assert.EqualError(t, err, "multiple datadog exporters found")
}

func (suite *ConfigTestSuite) TestNoDDAPISection() {
	t := suite.T()
	fileName := "testdata/config_no_api.yaml"
	c, err := NewConfigComponent(context.Background(), "", []string{fileName}, nil)
	require.NoError(t, err)
	assert.Equal(t, "datadoghq.com", c.Get("site"))
	assert.Equal(t, "https://api.datadoghq.com", c.Get("dd_url"))
	assert.Equal(t, "https://agent-http-intake.logs.datadoghq.com", c.Get("logs_config.logs_dd_url"))
	assert.Equal(t, "https://trace.agent.datadoghq.com", c.Get("apm_config.apm_dd_url"))
}

func (suite *ConfigTestSuite) TestNilDDAPISection() {
	t := suite.T()
	fileName := "testdata/config_nil_api.yaml"
	c, err := NewConfigComponent(context.Background(), "", []string{fileName}, nil)
	require.NoError(t, err)
	assert.Equal(t, "datadoghq.com", c.Get("site"))
	assert.Equal(t, "https://api.datadoghq.com", c.Get("dd_url"))
	assert.Equal(t, "https://agent-http-intake.logs.datadoghq.com", c.Get("logs_config.logs_dd_url"))
	assert.Equal(t, "https://trace.agent.datadoghq.com", c.Get("apm_config.apm_dd_url"))
}

func (suite *ConfigTestSuite) TestMalformedDDAPISection() {
	t := suite.T()
	fileName := "testdata/config_malformed_api.yaml"
	_, err := NewConfigComponent(context.Background(), "", []string{fileName}, nil)
	assert.EqualError(t, err, "invalid datadog exporter config")
}

func (suite *ConfigTestSuite) TestDDAPISiteEmpty() {
	t := suite.T()
	fileName := "testdata/config_site_empty.yaml"
	c, err := NewConfigComponent(context.Background(), "", []string{fileName}, nil)
	require.NoError(t, err)
	assert.Equal(t, "datadoghq.com", c.Get("site"))
	assert.Equal(t, "https://api.datadoghq.com", c.Get("dd_url"))
	assert.Equal(t, "https://agent-http-intake.logs.datadoghq.com", c.Get("logs_config.logs_dd_url"))
	assert.Equal(t, "https://trace.agent.datadoghq.com", c.Get("apm_config.apm_dd_url"))
}

func (suite *ConfigTestSuite) TestDDAPISiteNotSet() {
	t := suite.T()
	fileName := "testdata/config_site_not_set.yaml"
	c, err := NewConfigComponent(context.Background(), "", []string{fileName}, nil)
	require.NoError(t, err)
	assert.Equal(t, "datadoghq.com", c.Get("site"))
	assert.Equal(t, "https://api.datadoghq.com", c.Get("dd_url"))
	assert.Equal(t, "https://agent-http-intake.logs.datadoghq.com", c.Get("logs_config.logs_dd_url"))
	assert.Equal(t, "https://trace.agent.datadoghq.com", c.Get("apm_config.apm_dd_url"))
}

func (suite *ConfigTestSuite) TestDDAPISiteSet() {
	t := suite.T()
	fileName := "testdata/config_site_set.yaml"
	c, err := NewConfigComponent(context.Background(), "", []string{fileName}, nil)
	require.NoError(t, err)
	assert.Equal(t, "us3.datadoghq.com", c.Get("site"))
	assert.Equal(t, "https://api.us3.datadoghq.com", c.Get("dd_url"))
	assert.Equal(t, "https://agent-http-intake.logs.us3.datadoghq.com", c.Get("logs_config.logs_dd_url"))
	assert.Equal(t, "https://trace.agent.us3.datadoghq.com", c.Get("apm_config.apm_dd_url"))
}

func (suite *ConfigTestSuite) TestProxyDDEnvVarsWithoutCoreConfig() {
	t := suite.T()
	t.Setenv("DD_PROXY_HTTP", "http://dd-proxy.example.com:8080")
	t.Setenv("DD_PROXY_HTTPS", "https://dd-proxy.example.com:8443")
	t.Setenv("DD_PROXY_NO_PROXY", "localhost,127.0.0.1")

	pkgconfig, err := NewConfigComponent(context.Background(), "", []string{"testdata/config.yaml"}, nil)
	require.NoError(t, err)

	assert.Equal(t, "http://dd-proxy.example.com:8080", pkgconfig.GetString("proxy.http"))
	assert.Equal(t, "https://dd-proxy.example.com:8443", pkgconfig.GetString("proxy.https"))
	// 169.254.169.254 and 100.100.100.200 are added by default (cloud metadata endpoints)
	assert.ElementsMatch(t, []string{"localhost", "127.0.0.1", "169.254.169.254", "100.100.100.200"}, pkgconfig.GetStringSlice("proxy.no_proxy"))
}

func (suite *ConfigTestSuite) TestProxyHTTPEnvVarsWithoutCoreConfig() {
	t := suite.T()
	t.Setenv("HTTP_PROXY", "http://proxy.example.com:8080")
	t.Setenv("HTTPS_PROXY", "https://proxy.example.com:8443")
	t.Setenv("NO_PROXY", "localhost,127.0.0.1")

	pkgconfig, err := NewConfigComponent(context.Background(), "", []string{"testdata/config.yaml"}, nil)
	require.NoError(t, err)

	assert.Equal(t, "http://proxy.example.com:8080", pkgconfig.GetString("proxy.http"))
	assert.Equal(t, "https://proxy.example.com:8443", pkgconfig.GetString("proxy.https"))
	// 169.254.169.254 and 100.100.100.200 are added by default (cloud metadata endpoints)
	assert.ElementsMatch(t, []string{"localhost", "127.0.0.1", "169.254.169.254", "100.100.100.200"}, pkgconfig.GetStringSlice("proxy.no_proxy"))
}

func (suite *ConfigTestSuite) TestProxyDDEnvVarsTakePrecedenceOverHTTPEnvVars() {
	t := suite.T()
	t.Setenv("DD_PROXY_HTTP", "http://dd-proxy.example.com:8080")
	t.Setenv("DD_PROXY_HTTPS", "https://dd-proxy.example.com:8443")
	t.Setenv("HTTP_PROXY", "http://other-proxy.example.com:8080")
	t.Setenv("HTTPS_PROXY", "https://other-proxy.example.com:8443")

	pkgconfig, err := NewConfigComponent(context.Background(), "", []string{"testdata/config.yaml"}, nil)
	require.NoError(t, err)

	assert.Equal(t, "http://dd-proxy.example.com:8080", pkgconfig.GetString("proxy.http"))
	assert.Equal(t, "https://dd-proxy.example.com:8443", pkgconfig.GetString("proxy.https"))
}

func (suite *ConfigTestSuite) TestProxyConfigURLTakesPrecedenceOverDDEnvVars() {
	t := suite.T()
	t.Setenv("DD_PROXY_HTTP", "http://dd-proxy.example.com:8080")
	t.Setenv("DD_PROXY_HTTPS", "https://dd-proxy.example.com:8443")

	pkgconfig, err := NewConfigComponent(context.Background(), "", []string{"testdata/config_proxy.yaml"}, nil)
	require.NoError(t, err)

	// proxy_url from OTel exporter config should take precedence over DD_PROXY_* env vars
	assert.Equal(t, "http://proxyurl.example.com:3128", pkgconfig.GetString("proxy.http"))
	assert.Equal(t, "http://proxyurl.example.com:3128", pkgconfig.GetString("proxy.https"))
}

func (suite *ConfigTestSuite) TestProxyEnvVarsBoth() {
	t := suite.T()
	t.Setenv("HTTP_PROXY", "http://proxy.example.com:8080")
	t.Setenv("HTTPS_PROXY", "https://secure-proxy.example.com:8443")
	t.Setenv("NO_PROXY", "localhost,127.0.0.1,.local")

	pkgconfig, err := NewConfigComponent(context.Background(), "testdata/datadog_proxy_test.yaml", []string{"testdata/config.yaml"}, nil)
	require.NoError(t, err)

	assert.Equal(t, "http://proxy.example.com:8080", pkgconfig.GetString("proxy.http"))
	assert.Equal(t, "https://secure-proxy.example.com:8443", pkgconfig.GetString("proxy.https"))
	assert.Equal(t, []string{"localhost", "127.0.0.1", ".local"}, pkgconfig.GetStringSlice("proxy.no_proxy"))
}

func (suite *ConfigTestSuite) TestProxyEnvVarsHTTPOnly() {
	t := suite.T()

	t.Setenv("HTTP_PROXY", "http://proxy.example.com:3128")

	pkgconfig, err := NewConfigComponent(context.Background(), "testdata/datadog_proxy_test.yaml", []string{"testdata/config.yaml"}, nil)
	require.NoError(t, err)

	assert.Equal(t, "http://proxy.example.com:3128", pkgconfig.GetString("proxy.http"))
	assert.Equal(t, "", pkgconfig.GetString("proxy.https"))
	assert.Equal(t, []string(nil), pkgconfig.GetStringSlice("proxy.no_proxy"))
}

func (suite *ConfigTestSuite) TestProxyEnvVarsNone() {
	t := suite.T()

	pkgconfig, err := NewConfigComponent(context.Background(), "testdata/datadog_proxy_test.yaml", []string{"testdata/config.yaml"}, nil)
	require.NoError(t, err)

	assert.Equal(t, "", pkgconfig.GetString("proxy.http"))
	assert.Equal(t, "", pkgconfig.GetString("proxy.https"))
	assert.Equal(t, []string{}, pkgconfig.GetStringSlice("proxy.no_proxy"))
}

func (suite *ConfigTestSuite) TestProxyEnvVarsNOProxyOnly() {
	t := suite.T()

	// Set only NO_PROXY
	t.Setenv("NO_PROXY", "internal.company.com,localhost")

	pkgconfig, err := NewConfigComponent(context.Background(), "testdata/datadog_proxy_test.yaml", []string{"testdata/config.yaml"}, nil)
	require.NoError(t, err)

	assert.Equal(t, "", pkgconfig.GetString("proxy.http"))
	assert.Equal(t, "", pkgconfig.GetString("proxy.https"))
	assert.Equal(t, []string{"internal.company.com", "localhost"}, pkgconfig.GetStringSlice("proxy.no_proxy"))
}

func (suite *ConfigTestSuite) TestProxyConfigURLOnly() {
	t := suite.T()

	pkgconfig, err := NewConfigComponent(context.Background(), "testdata/datadog_proxy_test.yaml", []string{"testdata/config_proxy.yaml"}, nil)
	require.NoError(t, err)

	assert.Equal(t, "http://proxyurl.example.com:3128", pkgconfig.GetString("proxy.http"))
	assert.Equal(t, "http://proxyurl.example.com:3128", pkgconfig.GetString("proxy.https"))
	assert.Equal(t, []string(nil), pkgconfig.GetStringSlice("proxy.no_proxy"))
}

func (suite *ConfigTestSuite) TestProxyConfigURLPrecedence() {
	t := suite.T()

	t.Setenv("HTTP_PROXY", "http://proxy.example.com:8080")
	t.Setenv("HTTPS_PROXY", "https://secure-proxy.example.com:8443")

	pkgconfig, err := NewConfigComponent(context.Background(), "testdata/datadog_proxy_test.yaml", []string{"testdata/config_proxy.yaml"}, nil)
	require.NoError(t, err)

	// ProxyURL from config should take precedence over environment variables
	assert.Equal(t, "http://proxyurl.example.com:3128", pkgconfig.GetString("proxy.http"))
	assert.Equal(t, "http://proxyurl.example.com:3128", pkgconfig.GetString("proxy.https"))
	assert.Equal(t, []string(nil), pkgconfig.GetStringSlice("proxy.no_proxy"))
}

func (suite *ConfigTestSuite) TestProxyConfigURLOverridesDDConfig() {
	t := suite.T()

	pkgconfig, err := NewConfigComponent(context.Background(), "testdata/datadog_proxy_with_settings.yaml", []string{"testdata/config_proxy.yaml"}, nil)
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
	c, err := NewConfigComponent(context.Background(), "", []string{fileName}, nil)
	require.NoError(t, err, "NewConfigComponent should succeed with DD_LOGS_ENABLED set")
	assert.True(t, c.GetBool("logs_enabled"), "logs_enabled should be true when DD_LOGS_ENABLED=true")
}

// TestLogsEnabledViaDatadogConfig tests that logs_enabled can be set via a separate
// datadog.yaml config file and is correctly merged with the OTel config. This ensures
// the config initialization order works correctly when both configs are present.
func TestLogsEnabledViaDatadogConfig(t *testing.T) {
	configmock.New(t)
	ddFileName := "testdata/datadog_with_logs_enabled.yaml"
	c, err := NewConfigComponent(context.Background(), "", []string{ddFileName}, nil)
	require.NoError(t, err, "NewConfigComponent should succeed with datadog config")
	assert.True(t, c.GetBool("logs_enabled"), "logs_enabled should be true from datadog config")
}

// TestDogtelExtensionConfig_FullStandaloneConfig verifies that all dogtelextension
// standalone config fields are applied to the DD agent config.
func (suite *ConfigTestSuite) TestDogtelExtensionConfig_FullStandaloneConfig() {
	t := suite.T()
	t.Setenv("DD_OTEL_STANDALONE", "true")
	c, err := NewConfigComponent(context.Background(), "", []string{"testdata/config_standalone.yaml"}, nil)
	require.NoError(t, err)

	assert.Equal(t, true, c.GetBool("enable_metadata_collection"))
	assert.Equal(t, "my-standalone-host", c.Get("hostname"))
	assert.Equal(t, "/usr/local/bin/secret-provider", c.Get("secret_backend_command"))
	assert.Equal(t, []string{"--timeout", "30"}, c.GetStringSlice("secret_backend_arguments"))
	assert.Equal(t, 60, c.GetInt("secret_backend_timeout"))
	assert.Equal(t, 8192, c.GetInt("secret_backend_output_max_size"))
	assert.Equal(t, "10.0.0.1", c.Get("kubernetes_kubelet_host"))
	assert.Equal(t, false, c.GetBool("kubelet_tls_verify"))
	assert.Equal(t, 10255, c.GetInt("kubernetes_http_kubelet_port"))
	assert.Equal(t, 10250, c.GetInt("kubernetes_https_kubelet_port"))
}

// TestDogtelExtensionConfig_PartialConfig verifies that only the dogtelextension
// fields that are explicitly set override the corresponding DD agent config keys.
func (suite *ConfigTestSuite) TestDogtelExtensionConfig_PartialConfig() {
	t := suite.T()
	t.Setenv("DD_OTEL_STANDALONE", "true")
	c, err := NewConfigComponent(context.Background(), "", []string{"testdata/config_standalone_partial.yaml"}, nil)
	require.NoError(t, err)

	assert.Equal(t, "192.168.1.100", c.Get("kubernetes_kubelet_host"))
	assert.Equal(t, false, c.GetBool("kubelet_tls_verify"))
	// Fields not set in dogtelextension must not override DD agent defaults.
	assert.Equal(t, "", c.GetString("hostname"))
	assert.Equal(t, "", c.GetString("secret_backend_command"))
}

// TestDogtelExtensionConfig_MetadataDisabled verifies that setting
// enable_metadata_collection: false propagates to the DD agent config.
func (suite *ConfigTestSuite) TestDogtelExtensionConfig_MetadataDisabled() {
	t := suite.T()
	t.Setenv("DD_OTEL_STANDALONE", "true")
	c, err := NewConfigComponent(context.Background(), "", []string{"testdata/config_standalone_no_metadata.yaml"}, nil)
	require.NoError(t, err)

	assert.Equal(t, false, c.GetBool("enable_metadata_collection"))
}

// TestDogtelExtensionConfig_MetadataInterval verifies that metadata_interval is
// applied to the metadata_providers host entry so the host metadata collector
// uses the configured interval.
func (suite *ConfigTestSuite) TestDogtelExtensionConfig_MetadataInterval() {
	t := suite.T()
	t.Setenv("DD_OTEL_STANDALONE", "true")
	c, err := NewConfigComponent(context.Background(), "", []string{"testdata/config_standalone.yaml"}, nil)
	require.NoError(t, err)

	providers := c.Get("metadata_providers")
	require.NotNil(t, providers)
	providerList, ok := providers.([]map[string]interface{})
	require.True(t, ok)
	require.Len(t, providerList, 1)
	assert.Equal(t, "host", providerList[0]["name"])
	assert.Equal(t, 600, providerList[0]["interval"])
}

// TestDogtelExtensionConfig_MetadataIntervalMerge verifies that setting
// metadata_interval in the dogtel extension merges into the existing
// metadata_providers list rather than replacing it wholesale. Providers other
// than "host" must be preserved, and an existing "host" entry must have its
// interval updated in place.
func (suite *ConfigTestSuite) TestDogtelExtensionConfig_MetadataIntervalMerge() {
	t := suite.T()
	t.Setenv("DD_OTEL_STANDALONE", "true")
	// datadog_with_metadata_providers.yaml pre-seeds two providers:
	//   {name: resources, interval: 300} and {name: host, interval: 60}
	// The dogtel extension in config_standalone.yaml sets metadata_interval: 600.
	// The host entry's interval must be updated to 600; the resources entry must survive.
	c, err := NewConfigComponent(context.Background(), "testdata/datadog_with_metadata_providers.yaml", []string{"testdata/config_standalone.yaml"}, nil)
	require.NoError(t, err)

	providers := c.Get("metadata_providers")
	require.NotNil(t, providers)
	providerList, ok := providers.([]map[string]interface{})
	require.True(t, ok, "metadata_providers should be []map[string]interface{}")

	byName := map[string]map[string]interface{}{}
	for _, p := range providerList {
		if name, ok := p["name"].(string); ok {
			byName[name] = p
		}
	}

	require.Contains(t, byName, "host", "host provider must be present")
	assert.Equal(t, 600, byName["host"]["interval"], "host interval must be updated to metadata_interval value")

	require.Contains(t, byName, "resources", "resources provider must be preserved")
	assert.Equal(t, 300, byName["resources"]["interval"], "resources interval must remain unchanged")
}

// TestDogtelExtensionConfig_NoDogtelExtension verifies that a config without
// the dogtelextension section is still processed correctly (no error, no overrides).
// This test does NOT set DD_OTEL_STANDALONE, verifying that the block is skipped
// entirely in connected mode regardless of whether a dogtelextension is present.
func (suite *ConfigTestSuite) TestDogtelExtensionConfig_NoDogtelExtension() {
	t := suite.T()
	c, err := NewConfigComponent(context.Background(), "", []string{"testdata/config_default.yaml"}, nil)
	require.NoError(t, err)

	// No dogtelextension + not standalone → hostname not set.
	assert.Equal(t, "", c.GetString("hostname"))
	assert.Equal(t, "", c.GetString("secret_backend_command"))
	assert.Equal(t, "", c.GetString("kubernetes_kubelet_host"))
}

// TestDogtelExtensionConfig_StandaloneNoDDExporter verifies that dogtelextension
// config (hostname, metadata interval, kubelet settings, secret backend) and
// ENC[] secret resolution are still applied in standalone mode even when the
// OTel Collector config has no datadog exporter — NewConfigComponent returns
// ErrNoDDExporter, but that must not skip config that doesn't come from the
// exporter section.
func (suite *ConfigTestSuite) TestDogtelExtensionConfig_StandaloneNoDDExporter() {
	t := suite.T()
	t.Setenv("DD_OTEL_STANDALONE", "true")
	c, err := NewConfigComponent(context.Background(), "", []string{"testdata/config_standalone_no_dd_exporter.yaml"}, nil)
	require.ErrorIs(t, err, ErrNoDDExporter)
	require.NotNil(t, c)

	assert.Equal(t, true, c.GetBool("enable_metadata_collection"))
	assert.Equal(t, "my-standalone-host", c.Get("hostname"))
	assert.Equal(t, "/usr/local/bin/secret-provider", c.Get("secret_backend_command"))
	assert.Equal(t, []string{"--timeout", "30"}, c.GetStringSlice("secret_backend_arguments"))
	assert.Equal(t, 60, c.GetInt("secret_backend_timeout"))
	assert.Equal(t, 8192, c.GetInt("secret_backend_output_max_size"))
	assert.Equal(t, "10.0.0.1", c.Get("kubernetes_kubelet_host"))
	assert.Equal(t, false, c.GetBool("kubelet_tls_verify"))
	assert.Equal(t, 10255, c.GetInt("kubernetes_http_kubelet_port"))
	assert.Equal(t, 10250, c.GetInt("kubernetes_https_kubelet_port"))

	providers := c.Get("metadata_providers")
	require.NotNil(t, providers)
	providerList, ok := providers.([]map[string]interface{})
	require.True(t, ok)
	require.Len(t, providerList, 1)
	assert.Equal(t, "host", providerList[0]["name"])
	assert.Equal(t, 600, providerList[0]["interval"])
}

// TestDogtelExtensionConfig_ConnectedModeIgnored verifies that dogtelextension
// config is NOT applied when otel_standalone is false (connected mode).
func (suite *ConfigTestSuite) TestDogtelExtensionConfig_ConnectedModeIgnored() {
	t := suite.T()
	// otel_standalone is false by default — dogtelextension fields must be ignored.
	c, err := NewConfigComponent(context.Background(), "", []string{"testdata/config_standalone.yaml"}, nil)
	require.NoError(t, err)

	assert.Equal(t, "", c.GetString("hostname"))
	assert.Equal(t, "", c.GetString("secret_backend_command"))
	assert.Equal(t, "", c.GetString("kubernetes_kubelet_host"))
}

// TestGetDogtelExtensionConfig_NilExtensionSection verifies that getDogtelExtensionConfig
// returns nil without error when the extensions section is absent.
func TestGetDogtelExtensionConfig_NilExtensionSection(t *testing.T) {
	cfg := confmap.NewFromStringMap(map[string]any{
		"exporters": map[string]any{},
	})
	extcfg, err := getDogtelExtensionConfig(cfg)
	require.NoError(t, err)
	assert.Nil(t, extcfg)
}

// TestGetDogtelExtensionConfig_EmptyDogtelSection verifies that an empty dogtel
// extension section returns a zero-value struct with all pointer fields nil.
func TestGetDogtelExtensionConfig_EmptyDogtelSection(t *testing.T) {
	cfg := confmap.NewFromStringMap(map[string]any{
		"extensions": map[string]any{
			"dogtel": nil,
		},
	})
	extcfg, err := getDogtelExtensionConfig(cfg)
	require.NoError(t, err)
	require.NotNil(t, extcfg)
	assert.Equal(t, "", extcfg.Hostname)
	assert.Nil(t, extcfg.KubeletTLSVerify)
	assert.Nil(t, extcfg.EnableMetadataCollection)
	assert.Equal(t, 0, extcfg.MetadataInterval)
}

// TestGetDogtelExtensionConfig_EnableMetadataCollectionFalse verifies that
// enable_metadata_collection: false is correctly parsed as a *bool pointing to false,
// not left as nil.
func TestGetDogtelExtensionConfig_EnableMetadataCollectionFalse(t *testing.T) {
	falseVal := false
	cfg := confmap.NewFromStringMap(map[string]any{
		"extensions": map[string]any{
			"dogtel": map[string]any{
				"enable_metadata_collection": falseVal,
			},
		},
	})
	extcfg, err := getDogtelExtensionConfig(cfg)
	require.NoError(t, err)
	require.NotNil(t, extcfg)
	require.NotNil(t, extcfg.EnableMetadataCollection)
	assert.False(t, *extcfg.EnableMetadataCollection)
}

// TestGetDogtelExtensionConfig_KubeletTLSVerify verifies that kubelet_tls_verify
// can be explicitly set to false (distinguishable from the unset/nil state).
func TestGetDogtelExtensionConfig_KubeletTLSVerify(t *testing.T) {
	falseVal := false
	cfg := confmap.NewFromStringMap(map[string]any{
		"extensions": map[string]any{
			"dogtel": map[string]any{
				"kubelet_tls_verify": falseVal,
			},
		},
	})
	extcfg, err := getDogtelExtensionConfig(cfg)
	require.NoError(t, err)
	require.NotNil(t, extcfg)
	require.NotNil(t, extcfg.KubeletTLSVerify)
	assert.False(t, *extcfg.KubeletTLSVerify)
}

// TestGetDogtelExtensionConfig_InvalidExtensions verifies that a malformed
// extensions section returns an error.
func TestGetDogtelExtensionConfig_InvalidExtensions(t *testing.T) {
	cfg := confmap.NewFromStringMap(map[string]any{
		"extensions": "not-a-map",
	})
	_, err := getDogtelExtensionConfig(cfg)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "invalid extensions config")
}

// TestGetDogtelExtensionConfig_MultipleDogtelEntries verifies that having more than one
// "dogtel*" extension returns an error instead of silently picking one.
func TestGetDogtelExtensionConfig_MultipleDogtelEntries(t *testing.T) {
	cfg := confmap.NewFromStringMap(map[string]any{
		"extensions": map[string]any{
			"dogtel":        nil,
			"dogtel/second": nil,
		},
	})
	_, err := getDogtelExtensionConfig(cfg)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "multiple dogtel extensions found")
}

// TestGetDogtelExtensionConfig_SingleNamedDogtelEntry verifies that a single
// named "dogtel/<name>" entry (not literally "dogtel") is accepted without error.
func TestGetDogtelExtensionConfig_SingleNamedDogtelEntry(t *testing.T) {
	cfg := confmap.NewFromStringMap(map[string]any{
		"extensions": map[string]any{
			"dogtel/custom": map[string]any{
				"hostname": "myhost",
			},
		},
	})
	extcfg, err := getDogtelExtensionConfig(cfg)
	require.NoError(t, err)
	require.NotNil(t, extcfg)
	assert.Equal(t, "myhost", extcfg.Hostname)
}

// TestSecretsResolutionViaEnvVar verifies that ENC[] handles in DD_HOSTNAME (and
// other DD_* env vars) are resolved by the real secrets backend in standalone mode.
func (suite *ConfigTestSuite) TestSecretsResolutionViaEnvVar() {
	if runtime.GOOS == "windows" {
		suite.T().Skip("shell secret backend script not applicable on Windows")
	}
	t := suite.T()

	// Write a minimal secret backend script: ignore stdin, return a fixed value.
	scriptDir := t.TempDir()
	scriptPath := filepath.Join(scriptDir, "secret_backend.sh")
	script := "#!/bin/sh\necho '{\"hostname_secret\": {\"value\": \"resolved-hostname-from-secret\"}}'\n"
	require.NoError(t, os.WriteFile(scriptPath, []byte(script), 0700)) //nolint:gosec

	t.Setenv("DD_OTEL_STANDALONE", "true")
	t.Setenv("DD_HOSTNAME", "ENC[hostname_secret]")
	t.Setenv("DD_SECRET_BACKEND_COMMAND", scriptPath)

	c, err := NewConfigComponent(context.Background(), "", []string{"testdata/config_default.yaml"}, nil)
	require.NoError(t, err)

	assert.Equal(t, "resolved-hostname-from-secret", c.GetString("hostname"),
		"ENC[hostname_secret] should be resolved to the script's output value; "+
			"raw value would indicate secrets resolution was skipped in NewConfigComponent")
}

// TestSecretsNotResolvedInConnectedMode verifies that ENC[] handles are NOT
// resolved locally when otel_standalone is false (connected mode). In connected
// mode the core agent owns secret resolution and the otel-agent must not attempt
// to run a local backend — doing so would fail for backends that are only
// accessible to the core agent and abort otel-agent startup.
func (suite *ConfigTestSuite) TestSecretsNotResolvedInConnectedMode() {
	if runtime.GOOS == "windows" {
		suite.T().Skip("shell secret backend script not applicable on Windows")
	}
	t := suite.T()

	// Provide a working script so the test would resolve if the gate opened.
	scriptDir := t.TempDir()
	scriptPath := filepath.Join(scriptDir, "secret_backend.sh")
	script := "#!/bin/sh\necho '{\"hostname_secret\": {\"value\": \"resolved-hostname-from-secret\"}}'\n"
	require.NoError(t, os.WriteFile(scriptPath, []byte(script), 0700)) //nolint:gosec

	// Connected mode (DD_OTEL_STANDALONE not set / false).
	t.Setenv("DD_HOSTNAME", "ENC[hostname_secret]")
	t.Setenv("DD_SECRET_BACKEND_COMMAND", scriptPath)

	c, err := NewConfigComponent(context.Background(), "", []string{"testdata/config_default.yaml"}, nil)
	require.NoError(t, err, "NewConfigComponent must not error in connected mode even when a secret backend is configured")

	assert.Equal(t, "ENC[hostname_secret]", c.GetString("hostname"),
		"ENC[] handle must remain unresolved in connected mode; "+
			"the core agent resolves secrets and the otel-agent must not run a local backend")
}

// TestSecretBackendTypeGate verifies that secret_backend_type alone (without
// secret_backend_command) opens the resolution gate in NewConfigComponent.
// When no ENC[] handles are present in the config, the resolver exits early
// before invoking any subprocess, so this test runs without the embedded
// secret-generic-connector binary.
func (suite *ConfigTestSuite) TestSecretBackendTypeGate() {
	t := suite.T()
	t.Setenv("DD_OTEL_STANDALONE", "true")
	t.Setenv("DD_SECRET_BACKEND_TYPE", "file.json")

	c, err := NewConfigComponent(context.Background(), "", []string{"testdata/config_default.yaml"}, nil)
	require.NoError(t, err)

	assert.Equal(t, "file.json", c.GetString("secret_backend_type"),
		"secret_backend_type should be propagated to the agent config")
}

// TestSecretsResolutionViaBackendType verifies end-to-end ENC[] resolution when
// secret_backend_type is configured instead of secret_backend_command. The test
// uses the file.json native backend (secret-generic-connector) and is skipped if
// the embedded binary has not been built.
//
// DD_SECRET_BACKEND_CONFIG cannot be supplied as an env var because viper does not
// expand map types from a single env string; instead, secret_backend_config is
// written to a temporary datadog.yaml and passed as the ddCfg argument.
func (suite *ConfigTestSuite) TestSecretsResolutionViaBackendType() {
	if runtime.GOOS == "windows" {
		suite.T().Skip("file.json backend test not applicable on Windows")
	}
	t := suite.T()

	// Skip when the embedded secret-generic-connector binary has not been built.
	binPath := filepath.Join(defaultpaths.GetEmbeddedBinPath(), "secret-generic-connector")
	if _, statErr := os.Stat(binPath); statErr != nil {
		t.Skipf("secret-generic-connector not found at %s; skipping native backend test", binPath)
	}

	// Create a flat JSON secrets file: handle → resolved value.
	secretsDir := t.TempDir()
	secretsFile := filepath.Join(secretsDir, "secrets.json")
	require.NoError(t, os.WriteFile(secretsFile,
		[]byte(`{"hostname_secret": "resolved-hostname-from-secret"}`), 0644))

	t.Setenv("DD_OTEL_STANDALONE", "true")
	t.Setenv("DD_HOSTNAME", "ENC[hostname_secret]")
	t.Setenv("DD_SECRET_BACKEND_TYPE", "file.json")

	// Write a minimal datadog.yaml that supplies secret_backend_config.file_path.
	ddCfgDir := t.TempDir()
	ddCfgPath := filepath.Join(ddCfgDir, "datadog.yaml")
	ddCfg := fmt.Sprintf("secret_backend_config:\n  file_path: %q\n", secretsFile)
	require.NoError(t, os.WriteFile(ddCfgPath, []byte(ddCfg), 0644))

	c, err := NewConfigComponent(context.Background(), ddCfgPath, []string{"testdata/config_default.yaml"}, nil)
	require.NoError(t, err)

	assert.Equal(t, "resolved-hostname-from-secret", c.GetString("hostname"),
		"ENC[hostname_secret] should be resolved via the file.json native backend; "+
			"raw value would indicate the secret_backend_type gate was skipped")
}

// TestSecretsResolutionViaDogtelExtension verifies that ENC[] handles in DD_HOSTNAME
// (and other DD_* env vars) are resolved when secret_backend_command is configured
// via extensions.dogtel in the OTel config rather than via DD_SECRET_BACKEND_COMMAND.
// This guards against the regression where the secret resolution block ran before the
// dogtelextension config was applied, leaving ENC[] values unresolved because
// secret_backend_command was still empty at resolution time.
func (suite *ConfigTestSuite) TestSecretsResolutionViaDogtelExtension() {
	if runtime.GOOS == "windows" {
		suite.T().Skip("shell secret backend script not applicable on Windows")
	}
	t := suite.T()

	scriptDir := t.TempDir()
	scriptPath := filepath.Join(scriptDir, "secret_backend.sh")
	script := "#!/bin/sh\necho '{\"hostname_secret\": {\"value\": \"resolved-hostname-from-secret\"}}'\n"
	require.NoError(t, os.WriteFile(scriptPath, []byte(script), 0700)) //nolint:gosec

	t.Setenv("DD_OTEL_STANDALONE", "true")
	t.Setenv("DD_HOSTNAME", "ENC[hostname_secret]")

	otelCfgDir := t.TempDir()
	otelCfgPath := filepath.Join(otelCfgDir, "otel-config.yaml")
	otelCfg := fmt.Sprintf(`extensions:
  dogtel:
    secret_backend_command: %q
receivers:
  otlp:
    protocols:
      grpc:
exporters:
  datadog:
    api:
      key: TESTKEY
service:
  extensions: [dogtel]
  pipelines:
    traces:
      receivers: [otlp]
      exporters: [datadog]
`, scriptPath)
	require.NoError(t, os.WriteFile(otelCfgPath, []byte(otelCfg), 0644))

	c, err := NewConfigComponent(context.Background(), "", []string{otelCfgPath}, nil)
	require.NoError(t, err)

	assert.Equal(t, scriptPath, c.GetString("secret_backend_command"),
		"secret_backend_command should be set from dogtelextension config")

	assert.Equal(t, "resolved-hostname-from-secret", c.GetString("hostname"),
		"ENC[hostname_secret] in DD_HOSTNAME should be resolved via the secret backend "+
			"configured in dogtelextension")
}

// TestSuite runs the CalculatorTestSuite
func TestSuite(t *testing.T) {
	suite.Run(t, new(ConfigTestSuite))
}
