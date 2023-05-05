// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package utils

import (
	"testing"

	"github.com/DataDog/datadog-agent/pkg/config"
	"github.com/stretchr/testify/assert"
)

// TestSecretBackendWithMultipleEndpoints tests an edge case of `viper.AllSettings()` when a config
// key includes the key delimiter. Affects the config package when both secrets and multiple
// endpoints are configured.
// Refer to https://github.com/DataDog/viper/pull/2 for more details.
func TestSecretBackendWithMultipleEndpoints(t *testing.T) {
	conf := config.SetupConf()
	conf.SetConfigFile("./tests/datadog_secrets.yaml")
	// load the configuration
	_, err := config.LoadDatadogCustom(conf, "datadog_secrets.yaml", true)
	assert.NoError(t, err)

	expectedKeysPerDomain := map[string][]string{
		"https://app.datadoghq.com": {"someapikey", "someotherapikey"},
	}
	keysPerDomain, err := GetMultipleEndpoints(conf)
	assert.NoError(t, err)
	assert.Equal(t, expectedKeysPerDomain, keysPerDomain)
}

func TestGetMultipleEndpointsDefault(t *testing.T) {
	datadogYaml := `
api_key: fakeapikey

additional_endpoints:
  "https://app.datadoghq.com":
  - fakeapikey2
  - fakeapikey3
  "https://foo.datadoghq.com":
  - someapikey
`

	testConfig := config.SetupConfFromYAML(datadogYaml)

	multipleEndpoints, err := GetMultipleEndpoints(testConfig)

	expectedMultipleEndpoints := map[string][]string{
		"https://foo.datadoghq.com": {
			"someapikey",
		},
		"https://app.datadoghq.com": {
			"fakeapikey",
			"fakeapikey2",
			"fakeapikey3",
		},
	}

	assert.Nil(t, err)
	assert.EqualValues(t, expectedMultipleEndpoints, multipleEndpoints)
}

func TestGetMultipleEndpointsDDURL(t *testing.T) {
	datadogYaml := `
dd_url: "https://app.datadoghq.com"
api_key: fakeapikey

additional_endpoints:
  "https://app.datadoghq.com":
  - fakeapikey2
  - fakeapikey3
  "https://foo.datadoghq.com":
  - someapikey
`

	testConfig := config.SetupConfFromYAML(datadogYaml)

	multipleEndpoints, err := GetMultipleEndpoints(testConfig)

	expectedMultipleEndpoints := map[string][]string{
		"https://foo.datadoghq.com": {
			"someapikey",
		},
		"https://app.datadoghq.com": {
			"fakeapikey",
			"fakeapikey2",
			"fakeapikey3",
		},
	}

	assert.Nil(t, err)
	assert.EqualValues(t, expectedMultipleEndpoints, multipleEndpoints)
}

func TestGetMultipleEndpointsEnvVar(t *testing.T) {
	t.Setenv("DD_API_KEY", "fakeapikey")
	t.Setenv("DD_ADDITIONAL_ENDPOINTS", "{\"https://foo.datadoghq.com\": [\"someapikey\"]}")

	testConfig := config.SetupConf()

	multipleEndpoints, err := GetMultipleEndpoints(testConfig)

	expectedMultipleEndpoints := map[string][]string{
		"https://foo.datadoghq.com": {
			"someapikey",
		},
		"https://app.datadoghq.com": {
			"fakeapikey",
		},
	}

	assert.Nil(t, err)
	assert.EqualValues(t, expectedMultipleEndpoints, multipleEndpoints)
}

func TestGetMultipleEndpointsSite(t *testing.T) {
	datadogYaml := `
site: datadoghq.eu
api_key: fakeapikey

additional_endpoints:
  "https://app.datadoghq.com":
  - fakeapikey2
  - fakeapikey3
  "https://foo.datadoghq.com":
  - someapikey
`

	testConfig := config.SetupConfFromYAML(datadogYaml)

	multipleEndpoints, err := GetMultipleEndpoints(testConfig)

	expectedMultipleEndpoints := map[string][]string{
		"https://app.datadoghq.eu": {
			"fakeapikey",
		},
		"https://foo.datadoghq.com": {
			"someapikey",
		},
		"https://app.datadoghq.com": {
			"fakeapikey2",
			"fakeapikey3",
		},
	}

	assert.Nil(t, err)
	assert.EqualValues(t, expectedMultipleEndpoints, multipleEndpoints)
}

func TestGetMultipleEndpointsWithNoAdditionalEndpoints(t *testing.T) {
	datadogYaml := `
dd_url: "https://app.datadoghq.com"
api_key: fakeapikey
`

	testConfig := config.SetupConfFromYAML(datadogYaml)

	multipleEndpoints, err := GetMultipleEndpoints(testConfig)

	expectedMultipleEndpoints := map[string][]string{
		"https://app.datadoghq.com": {
			"fakeapikey",
		},
	}

	assert.Nil(t, err)
	assert.EqualValues(t, expectedMultipleEndpoints, multipleEndpoints)
}

func TestGetMultipleEndpointseIgnoresDomainWithoutApiKey(t *testing.T) {
	datadogYaml := `
dd_url: "https://app.datadoghq.com"
api_key: fakeapikey

additional_endpoints:
  "https://app.datadoghq.com":
  - fakeapikey2
  "https://foo.datadoghq.com":
  - someapikey
  "https://bar.datadoghq.com":
  - ""
`

	testConfig := config.SetupConfFromYAML(datadogYaml)

	multipleEndpoints, err := GetMultipleEndpoints(testConfig)

	expectedMultipleEndpoints := map[string][]string{
		"https://app.datadoghq.com": {
			"fakeapikey",
			"fakeapikey2",
		},
		"https://foo.datadoghq.com": {
			"someapikey",
		},
	}

	assert.Nil(t, err)
	assert.EqualValues(t, expectedMultipleEndpoints, multipleEndpoints)
}

func TestGetMultipleEndpointsApiKeyDeduping(t *testing.T) {
	datadogYaml := `
dd_url: "https://app.datadoghq.com"
api_key: fakeapikey

additional_endpoints:
  "https://app.datadoghq.com":
  - fakeapikey2
  - fakeapikey
  "https://foo.datadoghq.com":
  - someapikey
  - someotherapikey
  - someapikey
`

	testConfig := config.SetupConfFromYAML(datadogYaml)

	multipleEndpoints, err := GetMultipleEndpoints(testConfig)

	expectedMultipleEndpoints := map[string][]string{
		"https://app.datadoghq.com": {
			"fakeapikey",
			"fakeapikey2",
		},
		"https://foo.datadoghq.com": {
			"someapikey",
			"someotherapikey",
		},
	}

	assert.Nil(t, err)
	assert.EqualValues(t, expectedMultipleEndpoints, multipleEndpoints)
}

func TestSiteEnvVar(t *testing.T) {
	t.Setenv("DD_API_KEY", "fakeapikey")
	t.Setenv("DD_SITE", "datadoghq.eu")
	testConfig := config.SetupConfFromYAML("")

	multipleEndpoints, err := GetMultipleEndpoints(testConfig)
	externalAgentURL := GetMainEndpoint(testConfig, "https://external-agent.", "external_config.external_agent_dd_url")

	expectedMultipleEndpoints := map[string][]string{
		"https://app.datadoghq.eu": {
			"fakeapikey",
		},
	}

	assert.Nil(t, err)
	assert.EqualValues(t, expectedMultipleEndpoints, multipleEndpoints)
	assert.Equal(t, "https://external-agent.datadoghq.eu", externalAgentURL)
}

func TestDefaultSite(t *testing.T) {
	datadogYaml := `
api_key: fakeapikey
`
	testConfig := config.SetupConfFromYAML(datadogYaml)

	multipleEndpoints, err := GetMultipleEndpoints(testConfig)
	externalAgentURL := GetMainEndpoint(testConfig, "https://external-agent.", "external_config.external_agent_dd_url")

	expectedMultipleEndpoints := map[string][]string{
		"https://app.datadoghq.com": {
			"fakeapikey",
		},
	}

	assert.Nil(t, err)
	assert.EqualValues(t, expectedMultipleEndpoints, multipleEndpoints)
	assert.Equal(t, "https://external-agent.datadoghq.com", externalAgentURL)
}

func TestSite(t *testing.T) {
	datadogYaml := `
site: datadoghq.eu
api_key: fakeapikey
`
	testConfig := config.SetupConfFromYAML(datadogYaml)

	multipleEndpoints, err := GetMultipleEndpoints(testConfig)
	externalAgentURL := GetMainEndpoint(testConfig, "https://external-agent.", "external_config.external_agent_dd_url")

	expectedMultipleEndpoints := map[string][]string{
		"https://app.datadoghq.eu": {
			"fakeapikey",
		},
	}

	assert.Nil(t, err)
	assert.EqualValues(t, expectedMultipleEndpoints, multipleEndpoints)
	assert.Equal(t, "https://external-agent.datadoghq.eu", externalAgentURL)
}

func TestDDURLEnvVar(t *testing.T) {
	t.Setenv("DD_API_KEY", "fakeapikey")
	t.Setenv("DD_URL", "https://app.datadoghq.eu")
	t.Setenv("DD_EXTERNAL_CONFIG_EXTERNAL_AGENT_DD_URL", "https://custom.external-agent.datadoghq.com")
	testConfig := config.SetupConfFromYAML("")
	testConfig.BindEnv("external_config.external_agent_dd_url")

	multipleEndpoints, err := GetMultipleEndpoints(testConfig)
	externalAgentURL := GetMainEndpoint(testConfig, "https://external-agent.", "external_config.external_agent_dd_url")

	expectedMultipleEndpoints := map[string][]string{
		"https://app.datadoghq.eu": {
			"fakeapikey",
		},
	}

	assert.Nil(t, err)
	assert.EqualValues(t, expectedMultipleEndpoints, multipleEndpoints)
	assert.Equal(t, "https://custom.external-agent.datadoghq.com", externalAgentURL)
}

func TestDDDDURLEnvVar(t *testing.T) {
	t.Setenv("DD_API_KEY", "fakeapikey")
	t.Setenv("DD_DD_URL", "https://app.datadoghq.eu")
	t.Setenv("DD_EXTERNAL_CONFIG_EXTERNAL_AGENT_DD_URL", "https://custom.external-agent.datadoghq.com")
	testConfig := config.SetupConfFromYAML("")
	testConfig.BindEnv("external_config.external_agent_dd_url")

	multipleEndpoints, err := GetMultipleEndpoints(testConfig)
	externalAgentURL := GetMainEndpoint(testConfig, "https://external-agent.", "external_config.external_agent_dd_url")

	expectedMultipleEndpoints := map[string][]string{
		"https://app.datadoghq.eu": {
			"fakeapikey",
		},
	}

	assert.Nil(t, err)
	assert.EqualValues(t, expectedMultipleEndpoints, multipleEndpoints)
	assert.Equal(t, "https://custom.external-agent.datadoghq.com", externalAgentURL)
}

func TestDDURLAndDDDDURLEnvVar(t *testing.T) {
	t.Setenv("DD_API_KEY", "fakeapikey")

	// If DD_DD_URL and DD_URL are set, the value of DD_DD_URL is used
	t.Setenv("DD_DD_URL", "https://app.datadoghq.dd_dd_url.eu")
	t.Setenv("DD_URL", "https://app.datadoghq.dd_url.eu")

	t.Setenv("DD_EXTERNAL_CONFIG_EXTERNAL_AGENT_DD_URL", "https://custom.external-agent.datadoghq.com")
	testConfig := config.SetupConfFromYAML("")
	testConfig.BindEnv("external_config.external_agent_dd_url")

	multipleEndpoints, err := GetMultipleEndpoints(testConfig)
	externalAgentURL := GetMainEndpoint(testConfig, "https://external-agent.", "external_config.external_agent_dd_url")

	expectedMultipleEndpoints := map[string][]string{
		"https://app.datadoghq.dd_dd_url.eu": {
			"fakeapikey",
		},
	}

	assert.Nil(t, err)
	assert.EqualValues(t, expectedMultipleEndpoints, multipleEndpoints)
	assert.Equal(t, "https://custom.external-agent.datadoghq.com", externalAgentURL)
}

func TestDDURLOverridesSite(t *testing.T) {
	datadogYaml := `
site: datadoghq.eu
dd_url: "https://app.datadoghq.com"
api_key: fakeapikey

external_config:
  external_agent_dd_url: "https://external-agent.datadoghq.com"
`
	testConfig := config.SetupConfFromYAML(datadogYaml)

	multipleEndpoints, err := GetMultipleEndpoints(testConfig)
	externalAgentURL := GetMainEndpoint(testConfig, "https://external-agent.", "external_config.external_agent_dd_url")

	expectedMultipleEndpoints := map[string][]string{
		"https://app.datadoghq.com": {
			"fakeapikey",
		},
	}

	assert.Nil(t, err)
	assert.EqualValues(t, expectedMultipleEndpoints, multipleEndpoints)
	assert.Equal(t, "https://external-agent.datadoghq.com", externalAgentURL)
}

func TestDDURLNoSite(t *testing.T) {
	datadogYaml := `
dd_url: "https://app.datadoghq.eu"
api_key: fakeapikey

external_config:
  external_agent_dd_url: "https://custom.external-agent.datadoghq.eu"
`
	testConfig := config.SetupConfFromYAML(datadogYaml)

	multipleEndpoints, err := GetMultipleEndpoints(testConfig)
	externalAgentURL := GetMainEndpoint(testConfig, "https://external-agent.", "external_config.external_agent_dd_url")

	expectedMultipleEndpoints := map[string][]string{
		"https://app.datadoghq.eu": {
			"fakeapikey",
		},
	}

	assert.Nil(t, err)
	assert.EqualValues(t, expectedMultipleEndpoints, multipleEndpoints)
	assert.Equal(t, "https://custom.external-agent.datadoghq.eu", externalAgentURL)
}
