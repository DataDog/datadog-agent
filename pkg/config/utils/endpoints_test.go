// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package utils

import (
	"fmt"
	"testing"

	"github.com/DataDog/datadog-agent/pkg/config/mock"
	pkgconfigmodel "github.com/DataDog/datadog-agent/pkg/config/model"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestSecretBackendWithMultipleEndpoints tests an edge case of `viper.AllSettings()` when a config
// key includes the key delimiter. Affects the config package when both secrets and multiple
// endpoints are configured.
// Refer to https://github.com/DataDog/viper/pull/2 for more details.
func TestSecretBackendWithMultipleEndpoints(t *testing.T) {
	conf := mock.NewFromFile(t, "./tests/datadog_secrets.yaml")

	expectedKeysPerDomain := map[string][]APIKeys{
		"https://app.datadoghq.com.": {
			NewAPIKeys("api_key", "someapikey"),
			NewAPIKeys("additional_endpoints", "someotherapikey"),
		},
	}
	keysPerDomain, err := GetMultipleEndpoints(conf)
	assert.NoError(t, err)
	assert.Equal(t, expectedKeysPerDomain, keysPerDomain)
}

func TestGetMultipleEndpointsDefault(t *testing.T) {
	datadogYaml := `
api_key: fakeapikey

additional_endpoints:
  "https://app.datadoghq.com.":
  - fakeapikey2
  - fakeapikey3
  "https://foo.datadoghq.com.":
  - someapikey
`

	testConfig := mock.NewFromYAML(t, datadogYaml)

	multipleEndpoints, err := GetMultipleEndpoints(testConfig)

	expectedMultipleEndpoints := map[string][]APIKeys{
		"https://foo.datadoghq.com.": {
			NewAPIKeys("additional_endpoints", "someapikey"),
		},
		"https://app.datadoghq.com.": {
			NewAPIKeys("api_key", "fakeapikey"),
			NewAPIKeys("additional_endpoints", "fakeapikey2", "fakeapikey3"),
		},
	}

	assert.NoError(t, err)
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

	testConfig := mock.NewFromYAML(t, datadogYaml)

	multipleEndpoints, err := GetMultipleEndpoints(testConfig)

	expectedMultipleEndpoints := map[string][]APIKeys{
		"https://foo.datadoghq.com": {
			NewAPIKeys("additional_endpoints", "someapikey"),
		},
		"https://app.datadoghq.com": {
			NewAPIKeys("api_key", "fakeapikey"),
			NewAPIKeys("additional_endpoints", "fakeapikey2", "fakeapikey3"),
		},
	}

	assert.NoError(t, err)
	assert.EqualValues(t, expectedMultipleEndpoints, multipleEndpoints)
}

func TestGetMultipleEndpointsEnvVar(t *testing.T) {
	t.Setenv("DD_API_KEY", "fakeapikey")
	t.Setenv("DD_ADDITIONAL_ENDPOINTS", "{\"https://foo.datadoghq.com.\": [\"someapikey\"]}")

	testConfig := mock.New(t)

	multipleEndpoints, err := GetMultipleEndpoints(testConfig)

	expectedMultipleEndpoints := map[string][]APIKeys{
		"https://foo.datadoghq.com.": {
			NewAPIKeys("additional_endpoints", "someapikey"),
		},
		"https://app.datadoghq.com.": {
			NewAPIKeys("api_key", "fakeapikey"),
		},
	}

	assert.NoError(t, err)
	assert.EqualValues(t, expectedMultipleEndpoints, multipleEndpoints)
}

func TestGetMultipleEndpointsSite(t *testing.T) {
	datadogYaml := `
site: datadoghq.eu
api_key: fakeapikey

additional_endpoints:
  "https://app.datadoghq.com.":
  - fakeapikey2
  - fakeapikey3
  "https://foo.datadoghq.com.":
  - someapikey
`

	testConfig := mock.NewFromYAML(t, datadogYaml)

	multipleEndpoints, err := GetMultipleEndpoints(testConfig)

	expectedMultipleEndpoints := map[string][]APIKeys{
		"https://app.datadoghq.eu.": {
			NewAPIKeys("api_key", "fakeapikey"),
		},
		"https://foo.datadoghq.com.": {
			NewAPIKeys("additional_endpoints", "someapikey"),
		},
		"https://app.datadoghq.com.": {
			NewAPIKeys("additional_endpoints", "fakeapikey2", "fakeapikey3"),
		},
	}

	assert.NoError(t, err)
	assert.EqualValues(t, expectedMultipleEndpoints, multipleEndpoints)
}

func TestGetMultipleEndpointsWithNoAdditionalEndpoints(t *testing.T) {
	datadogYaml := `
dd_url: "https://app.datadoghq.com"
api_key: fakeapikey
`

	testConfig := mock.NewFromYAML(t, datadogYaml)

	multipleEndpoints, err := GetMultipleEndpoints(testConfig)

	expectedMultipleEndpoints := map[string][]APIKeys{
		"https://app.datadoghq.com": {
			NewAPIKeys("api_key", "fakeapikey"),
		},
	}

	assert.NoError(t, err)
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

	testConfig := mock.NewFromYAML(t, datadogYaml)

	multipleEndpoints, err := GetMultipleEndpoints(testConfig)

	expectedMultipleEndpoints := map[string][]APIKeys{
		"https://app.datadoghq.com": {
			NewAPIKeys("api_key", "fakeapikey"),
			NewAPIKeys("additional_endpoints", "fakeapikey2"),
		},
		"https://foo.datadoghq.com": {
			NewAPIKeys("additional_endpoints", "someapikey"),
		},
	}

	assert.NoError(t, err)
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

	testConfig := mock.NewFromYAML(t, datadogYaml)

	multipleEndpoints, err := GetMultipleEndpoints(testConfig)

	expectedMultipleEndpoints := map[string][]APIKeys{
		"https://app.datadoghq.com": {
			NewAPIKeys("api_key", "fakeapikey"),
			NewAPIKeys("additional_endpoints", "fakeapikey2", "fakeapikey"),
		},
		"https://foo.datadoghq.com": {
			NewAPIKeys("additional_endpoints", "someapikey", "someotherapikey", "someapikey"),
		},
	}

	assert.NoError(t, err)

	assert.EqualValues(t, expectedMultipleEndpoints, multipleEndpoints)
}

func TestSiteEnvVar(t *testing.T) {
	testCases := []struct {
		convertSiteFQDNEnabled bool
		siteURL                string
		expectedSiteURL        string
		prefix                 string
		expectedURLWithPrefix  string
	}{
		{true, "datadoghq.eu", "https://app.datadoghq.eu.", "https://external-agent.", "https://external-agent.datadoghq.eu."},
		{false, "datadoghq.eu", "https://app.datadoghq.eu", "https://external-agent.", "https://external-agent.datadoghq.eu"},
	}

	for _, tc := range testCases {
		t.Run(fmt.Sprintf("convertSiteFQDNEnabled=%t", tc.convertSiteFQDNEnabled), func(t *testing.T) {
			t.Setenv("DD_API_KEY", "fakeapikey")
			t.Setenv("DD_SITE", tc.siteURL)

			testConfig := mock.New(t)
			testConfig.Set("convert_dd_site_fqdn.enabled", tc.convertSiteFQDNEnabled, pkgconfigmodel.SourceAgentRuntime)

			multipleEndpoints, err := GetMultipleEndpoints(testConfig)
			externalAgentURL := GetMainEndpoint(testConfig, tc.prefix, "external_config.external_agent_dd_url")

			expectedMultipleEndpoints := map[string][]APIKeys{
				tc.expectedSiteURL: {
					NewAPIKeys("api_key", "fakeapikey"),
				},
			}

			assert.NoError(t, err)
			assert.EqualValues(t, expectedMultipleEndpoints, multipleEndpoints)
			assert.Equal(t, tc.expectedURLWithPrefix, externalAgentURL)
		})
	}

}

func TestDefaultSite(t *testing.T) {
	datadogYaml := `
api_key: fakeapikey
`
	testConfig := mock.NewFromYAML(t, datadogYaml)

	multipleEndpoints, err := GetMultipleEndpoints(testConfig)
	externalAgentURL := GetMainEndpoint(testConfig, "https://external-agent.", "external_config.external_agent_dd_url")

	expectedMultipleEndpoints := map[string][]APIKeys{
		"https://app.datadoghq.com.": {
			NewAPIKeys("api_key", "fakeapikey"),
		},
	}

	assert.NoError(t, err)
	assert.EqualValues(t, expectedMultipleEndpoints, multipleEndpoints)
	assert.Equal(t, "https://external-agent.datadoghq.com.", externalAgentURL)
}

func TestSite(t *testing.T) {
	testCases := []struct {
		yamlConfig     string
		externalPrefix string
		externalConfig string
		expectedSite   string
		expectedURL    string
	}{
		{
			yamlConfig: `
site: datadoghq.eu
api_key: fakeapikey
convert_dd_site_fqdn.enabled: true
`,
			externalPrefix: "https://external-agent.",
			externalConfig: "external_config.external_agent_dd_url",
			expectedSite:   "https://app.datadoghq.eu.",
			expectedURL:    "https://external-agent.datadoghq.eu.",
		},
		{
			yamlConfig: `
site: datadoghq.eu
api_key: fakeapikey
convert_dd_site_fqdn.enabled: false
`,
			externalPrefix: "https://external-agent.",
			externalConfig: "external_config.external_agent_dd_url",
			expectedSite:   "https://app.datadoghq.eu",
			expectedURL:    "https://external-agent.datadoghq.eu",
		},
	}
	for _, tc := range testCases {
		t.Run(tc.expectedSite, func(t *testing.T) {
			testConfig := mock.NewFromYAML(t, tc.yamlConfig)

			multipleEndpoints, err := GetMultipleEndpoints(testConfig)
			externalAgentURL := GetMainEndpoint(testConfig, tc.externalPrefix, tc.externalConfig)

			expectedMultipleEndpoints := map[string][]APIKeys{
				tc.expectedSite: {
					NewAPIKeys("api_key", "fakeapikey"),
				},
			}

			assert.NoError(t, err)
			assert.EqualValues(t, expectedMultipleEndpoints, multipleEndpoints)
			assert.Equal(t, tc.expectedURL, externalAgentURL)
		})
	}
}

func TestDDURLEnvVar(t *testing.T) {
	t.Setenv("DD_API_KEY", "fakeapikey")
	t.Setenv("DD_URL", "https://app.datadoghq.eu")
	t.Setenv("DD_EXTERNAL_CONFIG_EXTERNAL_AGENT_DD_URL", "https://custom.external-agent.datadoghq.com")
	testConfig := mock.New(t)
	testConfig.BindEnv("external_config.external_agent_dd_url") //nolint:forbidigo // TODO: replace by 'SetDefaultAndBindEnv'
	testConfig.BuildSchema()

	multipleEndpoints, err := GetMultipleEndpoints(testConfig)
	externalAgentURL := GetMainEndpoint(testConfig, "https://external-agent.", "external_config.external_agent_dd_url")

	expectedMultipleEndpoints := map[string][]APIKeys{
		"https://app.datadoghq.eu": {
			NewAPIKeys("api_key", "fakeapikey"),
		},
	}

	assert.NoError(t, err)
	assert.EqualValues(t, expectedMultipleEndpoints, multipleEndpoints)
	assert.Equal(t, "https://custom.external-agent.datadoghq.com", externalAgentURL)
}

func TestDDDDURLEnvVar(t *testing.T) {
	t.Setenv("DD_API_KEY", "fakeapikey")
	t.Setenv("DD_DD_URL", "https://app.datadoghq.eu")
	t.Setenv("DD_EXTERNAL_CONFIG_EXTERNAL_AGENT_DD_URL", "https://custom.external-agent.datadoghq.com")
	testConfig := mock.New(t)
	testConfig.BindEnv("external_config.external_agent_dd_url") //nolint:forbidigo // TODO: replace by 'SetDefaultAndBindEnv'
	testConfig.BuildSchema()

	multipleEndpoints, err := GetMultipleEndpoints(testConfig)
	externalAgentURL := GetMainEndpoint(testConfig, "https://external-agent.", "external_config.external_agent_dd_url")

	expectedMultipleEndpoints := map[string][]APIKeys{
		"https://app.datadoghq.eu": {
			NewAPIKeys("api_key", "fakeapikey"),
		},
	}

	assert.NoError(t, err)
	assert.EqualValues(t, expectedMultipleEndpoints, multipleEndpoints)
	assert.Equal(t, "https://custom.external-agent.datadoghq.com", externalAgentURL)
}

func TestDDURLAndDDDDURLEnvVar(t *testing.T) {
	t.Setenv("DD_API_KEY", "fakeapikey")

	// If DD_DD_URL and DD_URL are set, the value of DD_DD_URL is used
	t.Setenv("DD_DD_URL", "https://app.datadoghq.dd_dd_url.eu")
	t.Setenv("DD_URL", "https://app.datadoghq.dd_url.eu")

	t.Setenv("DD_EXTERNAL_CONFIG_EXTERNAL_AGENT_DD_URL", "https://custom.external-agent.datadoghq.com")
	testConfig := mock.New(t)
	testConfig.BindEnv("external_config.external_agent_dd_url") //nolint:forbidigo // TODO: replace by 'SetDefaultAndBindEnv'
	testConfig.BuildSchema()

	multipleEndpoints, err := GetMultipleEndpoints(testConfig)
	externalAgentURL := GetMainEndpoint(testConfig, "https://external-agent.", "external_config.external_agent_dd_url")

	expectedMultipleEndpoints := map[string][]APIKeys{
		"https://app.datadoghq.dd_dd_url.eu": {
			NewAPIKeys("api_key", "fakeapikey"),
		},
	}

	assert.NoError(t, err)
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
	testConfig := mock.NewFromYAML(t, datadogYaml)

	multipleEndpoints, err := GetMultipleEndpoints(testConfig)
	externalAgentURL := GetMainEndpoint(testConfig, "https://external-agent.", "external_config.external_agent_dd_url")

	expectedMultipleEndpoints := map[string][]APIKeys{
		"https://app.datadoghq.com": {
			NewAPIKeys("api_key", "fakeapikey"),
		},
	}

	assert.NoError(t, err)
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
	testConfig := mock.NewFromYAML(t, datadogYaml)

	multipleEndpoints, err := GetMultipleEndpoints(testConfig)
	externalAgentURL := GetMainEndpoint(testConfig, "https://external-agent.", "external_config.external_agent_dd_url")

	expectedMultipleEndpoints := map[string][]APIKeys{
		"https://app.datadoghq.eu": {
			NewAPIKeys("api_key", "fakeapikey"),
		},
	}

	assert.NoError(t, err)
	assert.EqualValues(t, expectedMultipleEndpoints, multipleEndpoints)
	assert.Equal(t, "https://custom.external-agent.datadoghq.eu", externalAgentURL)
}

func TestAddAgentVersionToDomain(t *testing.T) {
	appVersionPrefix := getDomainPrefix("app")
	flareVersionPrefix := getDomainPrefix("flare")

	versionURLTests := []struct {
		url                 string
		expectedURL         string
		shouldAppendVersion bool
	}{
		{ // US
			"https://app.datadoghq.com",
			".datadoghq.com",
			true,
		},
		{ // EU
			"https://app.datadoghq.eu",
			".datadoghq.eu",
			true,
		},
		{ // Gov
			"https://app.ddog-gov.com",
			".ddog-gov.com",
			true,
		},
		{ // Additional site
			"https://app.us2.datadoghq.com",
			".us2.datadoghq.com",
			true,
		},
		{ // arbitrary site
			"https://app.xx9.datadoghq.com",
			".xx9.datadoghq.com",
			true,
		},
		{ // Custom DD URL: leave unchanged
			"https://custom.datadoghq.com",
			"custom.datadoghq.com",
			false,
		},
		{ // Custom DD URL with 'agent' subdomain: leave unchanged
			"https://custom.agent.datadoghq.com",
			"custom.agent.datadoghq.com",
			false,
		},
		{ // Custom DD URL: unclear if anyone is actually using such a URL, but for now leave unchanged
			"https://app.custom.datadoghq.com",
			"app.custom.datadoghq.com",
			false,
		},
		{ // Custom top-level domain: unclear if anyone is actually using this, but for now leave unchanged
			"https://app.datadoghq.internal",
			"app.datadoghq.internal",
			false,
		},
		{ // DD URL set to proxy, leave unchanged
			"https://app.myproxy.com",
			"app.myproxy.com",
			false,
		},
		{ // MRF
			"https://app.mrf.datadoghq.com",
			".mrf.datadoghq.com",
			true,
		},
		{ // Trailing dot
			"https://app.datadoghq.com.",
			".datadoghq.com.",
			true,
		},
	}

	for _, testCase := range versionURLTests {
		appURL, err := AddAgentVersionToDomain(testCase.url, "app")
		require.NoError(t, err)
		flareURL, err := AddAgentVersionToDomain(testCase.url, "flare")
		require.NoError(t, err)

		if testCase.shouldAppendVersion {
			assert.Equal(t, "https://"+appVersionPrefix+testCase.expectedURL, appURL)
			assert.Equal(t, "https://"+flareVersionPrefix+testCase.expectedURL, flareURL)
		} else {
			assert.Equal(t, "https://"+testCase.expectedURL, appURL)
			assert.Equal(t, "https://"+testCase.expectedURL, flareURL)
		}
	}
}
