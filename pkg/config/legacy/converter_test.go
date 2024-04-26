// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package legacy

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/pkg/config"
)

func TestIsAffirmative(t *testing.T) {
	value, err := isAffirmative("yes")
	assert.NoError(t, err)
	assert.True(t, value)

	value, err = isAffirmative("True")
	assert.NoError(t, err)
	assert.True(t, value)

	value, err = isAffirmative("1")
	assert.NoError(t, err)
	assert.True(t, value)

	_, err = isAffirmative("")
	assert.NotNil(t, err)

	value, err = isAffirmative("ok")
	assert.NoError(t, err)
	assert.False(t, value)
}

func TestBuildProxySettings(t *testing.T) {
	agentConfig := make(Config)

	proxyOnlyHost := map[string]string{
		"http":  "http://foobar.baz",
		"https": "http://foobar.baz",
	}
	proxyNoUser := map[string]string{
		"http":  "http://foobar.baz:8080",
		"https": "http://foobar.baz:8080",
	}
	proxyOnlyPass := map[string]string{
		"http":  "http://foobar.baz:8080",
		"https": "http://foobar.baz:8080",
	}
	proxyOnlyUser := map[string]string{
		"http":  "http://myuser@foobar.baz:8080",
		"https": "http://myuser@foobar.baz:8080",
	}
	proxyWithUser := map[string]string{
		"http":  "http://myuser:mypass@foobar.baz:8080",
		"https": "http://myuser:mypass@foobar.baz:8080",
	}

	value, err := BuildProxySettings(agentConfig)
	assert.NoError(t, err)
	assert.Empty(t, value)

	// malformed url
	agentConfig["proxy_host"] = "http://notanurl{}"
	_, err = BuildProxySettings(agentConfig)
	assert.NotNil(t, err)

	agentConfig["proxy_host"] = "foobar.baz"

	value, err = BuildProxySettings(agentConfig)
	assert.NoError(t, err)
	assert.Equal(t, proxyOnlyHost, value)

	agentConfig["proxy_port"] = "8080"

	value, err = BuildProxySettings(agentConfig)
	assert.NoError(t, err)
	assert.Equal(t, proxyNoUser, value)

	// the password alone should not be considered without an user
	agentConfig["proxy_password"] = "mypass"
	value, err = BuildProxySettings(agentConfig)
	assert.NoError(t, err)
	assert.Equal(t, proxyOnlyPass, value)

	// the user alone is ok
	agentConfig["proxy_password"] = ""
	agentConfig["proxy_user"] = "myuser"
	value, err = BuildProxySettings(agentConfig)
	assert.NoError(t, err)
	assert.Equal(t, proxyOnlyUser, value)

	agentConfig["proxy_password"] = "mypass"
	agentConfig["proxy_user"] = "myuser"
	value, err = BuildProxySettings(agentConfig)
	assert.NoError(t, err)
	assert.Equal(t, proxyWithUser, value)
}

func TestBuildSyslogURI(t *testing.T) {
	agentConfig := make(Config)

	assert.Empty(t, buildSyslogURI(agentConfig))

	agentConfig["syslog_host"] = "127.0.0.1"
	agentConfig["syslog_port"] = "1234"
	assert.Equal(t, "127.0.0.1:1234", buildSyslogURI(agentConfig))
}

func TestBuildConfigProviders(t *testing.T) {
	agentConfig := make(Config)

	// unknown config provider
	agentConfig["sd_config_backend"] = "foo"
	_, err := buildConfigProviders(agentConfig)
	assert.NotNil(t, err)

	// etcd
	agentConfig["sd_config_backend"] = "etcd"
	agentConfig["sd_backend_host"] = "127.0.0.1"
	agentConfig["sd_backend_port"] = "1234"
	agentConfig["sd_backend_username"] = "user"
	agentConfig["sd_backend_password"] = "pass"
	providers, err := buildConfigProviders(agentConfig)
	assert.NoError(t, err)
	assert.Len(t, providers, 1)
	p := providers[0]
	assert.Equal(t, "etcd", p.Name)
	assert.Equal(t, "127.0.0.1:1234", p.TemplateURL)
	assert.Equal(t, "user", p.Username)
	assert.Equal(t, "pass", p.Password)
	assert.True(t, p.Polling)
	assert.Empty(t, p.Token)

	// consul has specific settings
	agentConfig = make(Config)
	agentConfig["sd_config_backend"] = "consul"
	agentConfig["consul_token"] = "123456"
	providers, err = buildConfigProviders(agentConfig)
	assert.NoError(t, err)
	assert.Len(t, providers, 1)
	p = providers[0]
	assert.Equal(t, "consul", p.Name)
	assert.Equal(t, "123456", p.Token)
}

func TestBuildHistogramAggregates(t *testing.T) {
	agentConfig := make(Config)

	// empty list
	agentConfig["histogram_aggregates"] = ""
	valueEmpty := buildHistogramAggregates(agentConfig)
	assert.Nil(t, valueEmpty)

	// list with invalid values
	agentConfig["histogram_aggregates"] = "test1, test2, test3"
	valueInvalids := buildHistogramAggregates(agentConfig)
	assert.Empty(t, valueInvalids)

	// list with valid and invalid values
	agentConfig["histogram_aggregates"] = "max, test1, count, min, test2"
	expectedBoth := []string{"max", "count", "min"}
	valueBoth := buildHistogramAggregates(agentConfig)
	assert.Equal(t, expectedBoth, valueBoth)

	// list with valid values
	agentConfig["histogram_aggregates"] = "max, min, count, sum"
	expectedValid := []string{"max", "min", "count", "sum"}
	valueValid := buildHistogramAggregates(agentConfig)
	assert.Equal(t, expectedValid, valueValid)
}

func TestBuildHistogramPercentiles(t *testing.T) {
	agentConfig := make(Config)

	// empty list
	agentConfig["histogram_percentiles"] = ""
	empty := buildHistogramPercentiles(agentConfig)
	assert.Nil(t, empty)

	// list with invalid values
	agentConfig["histogram_percentiles"] = "1, 2, -1, 0"
	actualInvalids := buildHistogramPercentiles(agentConfig)
	assert.Empty(t, actualInvalids)

	// list with valid values
	agentConfig["histogram_percentiles"] = "0.95, 0.511, 0.01"
	expectedValids := []string{"0.95", "0.51", "0.01"}
	actualValids := buildHistogramPercentiles(agentConfig)
	assert.Equal(t, expectedValids, actualValids)

	// list with both values
	agentConfig["histogram_percentiles"] = "0.25, 0, 0.677, 1"
	expectedBoth := []string{"0.25", "0.68"}
	actualBoth := buildHistogramPercentiles(agentConfig)
	assert.Equal(t, expectedBoth, actualBoth)
}

func TestDefaultValues(t *testing.T) {
	configConverter := config.NewConfigConverter()
	agentConfig := make(Config)
	FromAgentConfig(agentConfig, configConverter)
	assert.Equal(t, true, config.Datadog.GetBool("hostname_fqdn"))
}

func TestTraceIgnoreResources(t *testing.T) {
	require := require.New(t)
	configConverter := config.NewConfigConverter()

	cases := []struct {
		config   string
		expected []string
	}{
		{`r1`, []string{"r1"}},
		{`"r1","r2,"`, []string{"r1", "r2,"}},
		{`"r1"`, []string{"r1"}},
		{`r1,r2`, []string{"r1", "r2"}},
	}

	for _, c := range cases {
		cfg := make(Config)
		cfg["trace.ignore.resource"] = c.config
		err := FromAgentConfig(cfg, configConverter)
		require.NoError(err)
		require.Equal(c.expected, config.Datadog.GetStringSlice("apm_config.ignore_resources"))

	}
}

func TestConverter(t *testing.T) {
	require := require.New(t)
	configConverter := config.NewConfigConverter()
	cfg, err := GetAgentConfig("./tests/datadog.conf")
	require.NoError(err)
	err = FromAgentConfig(cfg, configConverter)
	require.NoError(err)
	c := config.Datadog

	require.Equal([]string{
		"GET|POST /healthcheck",
		"GET /V1",
	}, c.GetStringSlice("apm_config.ignore_resources"))

	// string values
	for k, v := range map[string]string{
		"api_key":                      "",
		"apm_config.api_key":           "1234",          // trace.api.api_key
		"apm_config.apm_dd_url":        "http://ip.url", // trace.api.endpoint
		"apm_config.env":               "staging",       // trace.config.env
		"apm_config.log_level":         "warn",          // trace.config.log_level
		"apm_config.log_file":          "/path/to/file", // trace.config.log_file
		"apm_config.extra_aggregators": "a,b,c",         // trace.concentrator.extra_aggregators
		"proxy.http":                   "http://user:password@my-proxy.com:3128",
		"proxy.https":                  "http://user:password@my-proxy.com:3128",
		"hostname":                     "mymachine.mydomain",
		"bind_host":                    "localhost",
		"log_level":                    "INFO",
	} {
		require.True(c.IsSet(k), k)
		require.Equal(v, c.GetString(k), k)
	}

	// bool values
	for k, v := range map[string]bool{
		"hostname_fqdn":                    true,
		"apm_config.enabled":               false,
		"apm_config.apm_non_local_traffic": true,
		"dogstatsd_non_local_traffic":      true,
		"skip_ssl_validation":              false,
	} {
		require.True(c.IsSet(k), k)
		require.Equal(v, c.GetBool(k), k)
	}

	// int values
	for k, v := range map[string]int{
		"apm_config.bucket_size_seconds":           5,    // trace.concentrator.bucket_size_seconds
		"apm_config.receiver_port":                 8126, // trace.receiver.receiver_port
		"apm_config.connection_limit":              2000, // trace.receiver.connection_limit
		"apm_config.receiver_timeout":              4,    // trace.receiver.timeout
		"apm_config.max_connections":               40,   // trace.watchdog.max_connections
		"apm_config.watchdog_check_delay":          5,    // trace.watchdog.check_delay_seconds
		"apm_config.extra_sample_rate":             1,
		"dogstatsd_port":                           8125,
		"apm_config.stats_writer.connection_limit": 3,
		"apm_config.stats_writer.queue_size":       4,
		"apm_config.trace_writer.connection_limit": 5,
		"apm_config.trace_writer.queue_size":       6,
	} {
		require.True(c.IsSet(k), k)
		require.Equal(v, c.GetInt(k), k)
	}

	// float64 values
	for k, v := range map[string]float64{
		"apm_config.extra_sample_rate":     1.,     // trace.sampler.extra_sample_rate
		"apm_config.max_traces_per_second": 10.,    // trace.sampler.max_traces_per_second
		"apm_config.max_events_per_second": 10.4,   // trace.sampler.max_events_per_second
		"apm_config.max_memory":            1234.5, // trace.watchdog.max_memory
		"apm_config.max_cpu_percent":       85.4,   // trace.watchdog.max_cpu_percent
	} {
		require.True(c.IsSet(k), k)
		require.Equal(v, c.GetFloat64(k), k)
	}

	require.Equal(map[string]string{
		"service1": "1.1",
		"service2": "1.2",
	}, c.GetStringMapString("apm_config.analyzed_rate_by_service"))

	require.Equal(map[string]string{
		"service3|op3": "1.3",
		"service4|op4": "1.4",
	}, c.GetStringMapString("apm_config.analyzed_spans"))
}

func TestExtractURLAPIKeys(t *testing.T) {
	configConverter := config.NewConfigConverter()
	defer func() {
		configConverter.Set("dd_url", "")
		configConverter.Set("api_key", "")
		configConverter.Set("additional_endpoints", nil)
	}()
	agentConfig := make(Config)

	// empty
	agentConfig["dd_url"] = ""
	agentConfig["api_key"] = ""
	err := extractURLAPIKeys(agentConfig, configConverter)
	assert.NoError(t, err)
	assert.Equal(t, "", config.Datadog.GetString("dd_url"))
	assert.Equal(t, "", config.Datadog.GetString("api_key"))
	assert.Empty(t, config.Datadog.GetStringMapStringSlice("additional_endpoints"))

	// one url and one key
	agentConfig["dd_url"] = "https://datadoghq.com"
	agentConfig["api_key"] = "123456789"
	err = extractURLAPIKeys(agentConfig, configConverter)
	assert.NoError(t, err)
	assert.Equal(t, "https://datadoghq.com", config.Datadog.GetString("dd_url"))
	assert.Equal(t, "123456789", config.Datadog.GetString("api_key"))
	assert.Empty(t, config.Datadog.GetStringMapStringSlice("additional_endpoints"))

	// multiple dd_url and api_key
	agentConfig["dd_url"] = "https://datadoghq.com,https://datadoghq.com,https://datadoghq.com,https://staging.com"
	agentConfig["api_key"] = "123456789,abcdef,secret_key,secret_key2"
	err = extractURLAPIKeys(agentConfig, configConverter)
	assert.NoError(t, err)
	assert.Equal(t, "https://datadoghq.com", config.Datadog.GetString("dd_url"))
	assert.Equal(t, "123456789", config.Datadog.GetString("api_key"))

	endpoints := config.Datadog.GetStringMapStringSlice("additional_endpoints")
	assert.Equal(t, 2, len(endpoints))
	assert.Equal(t, []string{"abcdef", "secret_key"}, endpoints["https://datadoghq.com"])
	assert.Equal(t, []string{"secret_key2"}, endpoints["https://staging.com"])

	// config error
	agentConfig["dd_url"] = "https://datadoghq.com,https://datadoghq.com,hhttps://datadoghq.com,ttps://staging.com"
	agentConfig["api_key"] = "123456789,abcdef,secret_key"
	err = extractURLAPIKeys(agentConfig, configConverter)
	assert.NotNil(t, err)
}
