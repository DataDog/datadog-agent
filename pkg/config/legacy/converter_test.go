// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2018 Datadog, Inc.

package legacy

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/DataDog/datadog-agent/pkg/config"
)

func TestIsAffirmative(t *testing.T) {
	value, err := isAffirmative("yes")
	assert.Nil(t, err)
	assert.True(t, value)

	value, err = isAffirmative("True")
	assert.Nil(t, err)
	assert.True(t, value)

	value, err = isAffirmative("1")
	assert.Nil(t, err)
	assert.True(t, value)

	_, err = isAffirmative("")
	assert.NotNil(t, err)

	value, err = isAffirmative("ok")
	assert.Nil(t, err)
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

	value, err := buildProxySettings(agentConfig)
	assert.Nil(t, err)
	assert.Empty(t, value)

	// malformed url
	agentConfig["proxy_host"] = "http://notanurl{}"
	_, err = buildProxySettings(agentConfig)
	assert.NotNil(t, err)

	agentConfig["proxy_host"] = "foobar.baz"

	value, err = buildProxySettings(agentConfig)
	assert.Nil(t, err)
	assert.Equal(t, proxyOnlyHost, value)

	agentConfig["proxy_port"] = "8080"

	value, err = buildProxySettings(agentConfig)
	assert.Nil(t, err)
	assert.Equal(t, proxyNoUser, value)

	// the password alone should not be considered without an user
	agentConfig["proxy_password"] = "mypass"
	value, err = buildProxySettings(agentConfig)
	assert.Nil(t, err)
	assert.Equal(t, proxyOnlyPass, value)

	// the user alone is ok
	agentConfig["proxy_password"] = ""
	agentConfig["proxy_user"] = "myuser"
	value, err = buildProxySettings(agentConfig)
	assert.Nil(t, err)
	assert.Equal(t, proxyOnlyUser, value)

	agentConfig["proxy_password"] = "mypass"
	agentConfig["proxy_user"] = "myuser"
	value, err = buildProxySettings(agentConfig)
	assert.Nil(t, err)
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
	assert.Nil(t, err)
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
	assert.Nil(t, err)
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
	agentConfig := make(Config)
	FromAgentConfig(agentConfig)

	assert.Equal(t, true, config.Datadog.GetBool("hostname_fqdn"))
}

func TestExtractURLAPIKeys(t *testing.T) {
	configConverter := config.NewConfigConverter()
	defer func(*config.LegacyConfigConverter) {
		configConverter.Set("dd_url", "")
		configConverter.Set("api_key", "")
		configConverter.Set("additional_endpoints", nil)
	}(configConverter)
	agentConfig := make(Config)

	// empty
	agentConfig["dd_url"] = ""
	agentConfig["api_key"] = ""
	err := extractURLAPIKeys(agentConfig, configConverter)
	assert.Nil(t, err)
	assert.Equal(t, "", config.Datadog.Get("dd_url"))
	assert.Equal(t, "", config.Datadog.Get("api_key"))
	assert.Nil(t, config.Datadog.Get("additional_endpoints"))

	// one url and one key
	agentConfig["dd_url"] = "https://datadoghq.com"
	agentConfig["api_key"] = "123456789"
	err = extractURLAPIKeys(agentConfig, configConverter)
	assert.Nil(t, err)
	assert.Equal(t, "https://datadoghq.com", config.Datadog.Get("dd_url"))
	assert.Equal(t, "123456789", config.Datadog.Get("api_key"))
	assert.Nil(t, config.Datadog.Get("additional_endpoints"))

	// multiple dd_url and api_key
	agentConfig["dd_url"] = "https://datadoghq.com,https://datadoghq.com,https://datadoghq.com,https://staging.com"
	agentConfig["api_key"] = "123456789,abcdef,secret_key,secret_key2"
	err = extractURLAPIKeys(agentConfig, configConverter)
	assert.Nil(t, err)
	assert.Equal(t, "https://datadoghq.com", config.Datadog.Get("dd_url"))
	assert.Equal(t, "123456789", config.Datadog.Get("api_key"))

	endpoints := config.Datadog.Get("additional_endpoints").(map[string][]string)
	assert.Equal(t, 2, len(endpoints))
	assert.Equal(t, []string{"abcdef", "secret_key"}, endpoints["https://datadoghq.com"])
	assert.Equal(t, []string{"secret_key2"}, endpoints["https://staging.com"])

	// config error
	agentConfig["dd_url"] = "https://datadoghq.com,https://datadoghq.com,hhttps://datadoghq.com,ttps://staging.com"
	agentConfig["api_key"] = "123456789,abcdef,secret_key"
	err = extractURLAPIKeys(agentConfig, configConverter)
	assert.NotNil(t, err)
}
