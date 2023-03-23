// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package config

import (
	"bytes"
	"fmt"
	"log"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/DataDog/datadog-agent/pkg/autodiscovery/common/types"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func setupConf() Config {
	conf := NewConfig("datadog", "DD", strings.NewReplacer(".", "_"))
	InitConfig(conf)
	return conf
}

func setupConfFromYAML(yamlConfig string) Config {
	conf := setupConf()
	conf.SetConfigType("yaml")
	e := conf.ReadConfig(bytes.NewBuffer([]byte(yamlConfig)))
	if e != nil {
		log.Println(e)
	}
	return conf
}

func unsetEnvForTest(env string) (reset func()) {
	oldValue, ok := os.LookupEnv(env)
	os.Unsetenv(env)

	return func() {
		if !ok {
			os.Unsetenv(env)
		} else {
			os.Setenv(env, oldValue)
		}
	}
}

func TestDefaults(t *testing.T) {
	config := setupConf()

	// Testing viper's handling of defaults
	assert.False(t, config.IsSet("site"))
	assert.False(t, config.IsSet("dd_url"))
	assert.Equal(t, "", config.GetString("site"))
	assert.Equal(t, "", config.GetString("dd_url"))
	assert.Equal(t, []string{"aws", "gcp", "azure", "alibaba", "oracle", "ibm"}, config.GetStringSlice("cloud_provider_metadata"))

	// Testing process-agent defaults
	assert.Equal(t, map[string]interface{}{
		"enabled":        true,
		"hint_frequency": 60,
		"interval":       4 * time.Hour,
	}, config.GetStringMap("process_config.process_discovery"))
}

func TestDefaultSite(t *testing.T) {
	datadogYaml := `
api_key: fakeapikey
`
	testConfig := setupConfFromYAML(datadogYaml)

	multipleEndpoints, err := getMultipleEndpointsWithConfig(testConfig)
	externalAgentURL := GetMainEndpointWithConfig(testConfig, "https://external-agent.", "external_config.external_agent_dd_url")

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
	testConfig := setupConfFromYAML(datadogYaml)

	multipleEndpoints, err := getMultipleEndpointsWithConfig(testConfig)
	externalAgentURL := GetMainEndpointWithConfig(testConfig, "https://external-agent.", "external_config.external_agent_dd_url")

	expectedMultipleEndpoints := map[string][]string{
		"https://app.datadoghq.eu": {
			"fakeapikey",
		},
	}

	assert.Nil(t, err)
	assert.EqualValues(t, expectedMultipleEndpoints, multipleEndpoints)
	assert.Equal(t, "https://external-agent.datadoghq.eu", externalAgentURL)
}

func TestUnexpectedUnicode(t *testing.T) {
	keyYaml := "api_\u202akey: fakeapikey\n"
	valueYaml := "api_key: fa\u202akeapikey\n"

	testConfig := setupConfFromYAML(keyYaml)

	warnings := findUnexpectedUnicode(testConfig)
	require.Len(t, warnings, 1)

	assert.Contains(t, warnings[0], "Configuration key string")
	assert.Contains(t, warnings[0], "U+202A")

	testConfig = setupConfFromYAML(valueYaml)

	warnings = findUnexpectedUnicode(testConfig)

	require.Len(t, warnings, 1)
	assert.Contains(t, warnings[0], "For key 'api_key'")
	assert.Contains(t, warnings[0], "U+202A")
}

func TestUnexpectedNestedUnicode(t *testing.T) {
	yaml := "runtime_security_config:\n  activity_dump:\n    remote_storage:\n      endpoints:\n        logs_dd_url: \"http://\u202adatadawg.com\""
	testConfig := setupConfFromYAML(yaml)

	warnings := findUnexpectedUnicode(testConfig)
	require.Len(t, warnings, 1)

	assert.Contains(t, warnings[0], "U+202A")
	assert.Contains(t, warnings[0], "For key 'runtime_security_config.activity_dump.remote_storage.endpoints.logs_dd_url'")
}

func TestUnexpectedWhitespace(t *testing.T) {
	tests := []struct {
		yaml                string
		expectedWarningText string
		expectedPosition    string
	}{
		{
			yaml:                "root_element:\n  nestedKey: \"hiddenI\u200bnvalidWhitespaceEmbedded\n\"",
			expectedWarningText: "U+200B",
			expectedPosition:    fmt.Sprintf("position %d", 7),
		},
		{
			yaml:                "root_element:\n  nestedKey: \u202fhiddenInvalidWhitespaceToLeft\n",
			expectedWarningText: "U+202F",
			expectedPosition:    fmt.Sprintf("position %d", 0),
		},
		{
			yaml:                "root_element:\n  nestedKey: [validValue, \u202fhiddenInvalidWhitespaceToLeft]\n",
			expectedWarningText: "U+202F",
			expectedPosition:    fmt.Sprintf("position %d", 0),
		},
	}
	for _, tc := range tests {
		testConfig := setupConfFromYAML(tc.yaml)
		warnings := findUnexpectedUnicode(testConfig)
		require.Len(t, warnings, 1)

		assert.Contains(t, warnings[0], tc.expectedPosition)
		assert.Contains(t, warnings[0], tc.expectedPosition)
	}
}

func TestUnknownKeysWarning(t *testing.T) {
	yamlBase := `
site: datadoghq.eu
`
	confBase := setupConfFromYAML(yamlBase)
	assert.Len(t, findUnknownKeys(confBase), 0)

	yamlWithUnknownKeys := `
site: datadoghq.eu
unknown_key.unknown_subkey: true
`
	confWithUnknownKeys := setupConfFromYAML(yamlWithUnknownKeys)
	assert.Len(t, findUnknownKeys(confWithUnknownKeys), 1)

	confWithUnknownKeys.SetKnown("unknown_key.*")
	assert.Len(t, findUnknownKeys(confWithUnknownKeys), 0)
}

func TestUnknownVarsWarning(t *testing.T) {
	test := func(v string, unknown bool, additional []string) func(*testing.T) {
		return func(t *testing.T) {
			env := []string{fmt.Sprintf("%s=foo", v)}
			var exp []string
			if unknown {
				exp = append(exp, v)
			}
			assert.Equal(t, exp, findUnknownEnvVars(Mock(t), env, additional))
		}
	}
	t.Run("DD_API_KEY", test("DD_API_KEY", false, nil))
	t.Run("DD_SITE", test("DD_SITE", false, nil))
	t.Run("DD_UNKNOWN", test("DD_UNKNOWN", true, nil))
	t.Run("UNKNOWN", test("UNKNOWN", false, nil)) // no DD_ prefix
	t.Run("DD_PROXY_NO_PROXY", test("DD_PROXY_NO_PROXY", false, nil))
	t.Run("DD_PROXY_HTTP", test("DD_PROXY_HTTP", false, nil))
	t.Run("DD_PROXY_HTTPS", test("DD_PROXY_HTTPS", false, nil))
	t.Run("DD_INSIDE_CI", test("DD_INSIDE_CI", false, nil))
	t.Run("DD_SYSTEM_PROBE_EXTRA", test("DD_SYSTEM_PROBE_EXTRA", false, []string{"DD_SYSTEM_PROBE_EXTRA"}))
}

func TestSiteEnvVar(t *testing.T) {
	t.Setenv("DD_API_KEY", "fakeapikey")
	t.Setenv("DD_SITE", "datadoghq.eu")
	testConfig := setupConfFromYAML("")

	multipleEndpoints, err := getMultipleEndpointsWithConfig(testConfig)
	externalAgentURL := GetMainEndpointWithConfig(testConfig, "https://external-agent.", "external_config.external_agent_dd_url")

	expectedMultipleEndpoints := map[string][]string{
		"https://app.datadoghq.eu": {
			"fakeapikey",
		},
	}

	assert.Nil(t, err)
	assert.EqualValues(t, expectedMultipleEndpoints, multipleEndpoints)
	assert.Equal(t, "https://external-agent.datadoghq.eu", externalAgentURL)
}

func TestDefaultTraceManagedServicesEnvVarValue(t *testing.T) {
	testConfig := setupConfFromYAML("")
	assert.Equal(t, true, testConfig.Get("serverless.trace_managed_services"))
}

func TestExplicitFalseTraceManagedServicesEnvVar(t *testing.T) {
	t.Setenv("DD_TRACE_MANAGED_SERVICES", "false")
	testConfig := setupConfFromYAML("")
	assert.Equal(t, false, testConfig.Get("serverless.trace_managed_services"))
}

func TestDDHostnameFileEnvVar(t *testing.T) {
	t.Setenv("DD_API_KEY", "fakeapikey")
	t.Setenv("DD_HOSTNAME_FILE", "somefile")
	testConfig := setupConfFromYAML("")

	assert.Equal(t, "somefile", testConfig.Get("hostname_file"))
}

func TestDDURLEnvVar(t *testing.T) {
	t.Setenv("DD_API_KEY", "fakeapikey")
	t.Setenv("DD_URL", "https://app.datadoghq.eu")
	t.Setenv("DD_EXTERNAL_CONFIG_EXTERNAL_AGENT_DD_URL", "https://custom.external-agent.datadoghq.com")
	testConfig := setupConfFromYAML("")
	testConfig.BindEnv("external_config.external_agent_dd_url")

	multipleEndpoints, err := getMultipleEndpointsWithConfig(testConfig)
	externalAgentURL := GetMainEndpointWithConfig(testConfig, "https://external-agent.", "external_config.external_agent_dd_url")

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
	testConfig := setupConfFromYAML("")
	testConfig.BindEnv("external_config.external_agent_dd_url")

	multipleEndpoints, err := getMultipleEndpointsWithConfig(testConfig)
	externalAgentURL := GetMainEndpointWithConfig(testConfig, "https://external-agent.", "external_config.external_agent_dd_url")

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
	testConfig := setupConfFromYAML("")
	testConfig.BindEnv("external_config.external_agent_dd_url")

	multipleEndpoints, err := getMultipleEndpointsWithConfig(testConfig)
	externalAgentURL := GetMainEndpointWithConfig(testConfig, "https://external-agent.", "external_config.external_agent_dd_url")

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
	testConfig := setupConfFromYAML(datadogYaml)

	multipleEndpoints, err := getMultipleEndpointsWithConfig(testConfig)
	externalAgentURL := GetMainEndpointWithConfig(testConfig, "https://external-agent.", "external_config.external_agent_dd_url")

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
	testConfig := setupConfFromYAML(datadogYaml)

	multipleEndpoints, err := getMultipleEndpointsWithConfig(testConfig)
	externalAgentURL := GetMainEndpointWithConfig(testConfig, "https://external-agent.", "external_config.external_agent_dd_url")

	expectedMultipleEndpoints := map[string][]string{
		"https://app.datadoghq.eu": {
			"fakeapikey",
		},
	}

	assert.Nil(t, err)
	assert.EqualValues(t, expectedMultipleEndpoints, multipleEndpoints)
	assert.Equal(t, "https://custom.external-agent.datadoghq.eu", externalAgentURL)
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

	testConfig := setupConfFromYAML(datadogYaml)

	multipleEndpoints, err := getMultipleEndpointsWithConfig(testConfig)

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

	testConfig := setupConf()

	multipleEndpoints, err := getMultipleEndpointsWithConfig(testConfig)

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

	testConfig := setupConfFromYAML(datadogYaml)

	multipleEndpoints, err := getMultipleEndpointsWithConfig(testConfig)

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

	testConfig := setupConfFromYAML(datadogYaml)

	multipleEndpoints, err := getMultipleEndpointsWithConfig(testConfig)

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

func TestGetMultipleEndpointsWithNoAdditionalEndpoints(t *testing.T) {
	datadogYaml := `
dd_url: "https://app.datadoghq.com"
api_key: fakeapikey
`

	testConfig := setupConfFromYAML(datadogYaml)

	multipleEndpoints, err := getMultipleEndpointsWithConfig(testConfig)

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

	testConfig := setupConfFromYAML(datadogYaml)

	multipleEndpoints, err := getMultipleEndpointsWithConfig(testConfig)

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

	testConfig := setupConfFromYAML(datadogYaml)

	multipleEndpoints, err := getMultipleEndpointsWithConfig(testConfig)

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
	}

	for _, testCase := range versionURLTests {
		appURL, err := AddAgentVersionToDomain(testCase.url, "app")
		require.Nil(t, err)
		flareURL, err := AddAgentVersionToDomain(testCase.url, "flare")
		require.Nil(t, err)

		if testCase.shouldAppendVersion {
			assert.Equal(t, "https://"+appVersionPrefix+testCase.expectedURL, appURL)
			assert.Equal(t, "https://"+flareVersionPrefix+testCase.expectedURL, flareURL)
		} else {
			assert.Equal(t, "https://"+testCase.expectedURL, appURL)
			assert.Equal(t, "https://"+testCase.expectedURL, flareURL)
		}
	}
}

func TestIsCloudProviderEnabled(t *testing.T) {
	holdValue := Datadog.Get("cloud_provider_metadata")
	defer Datadog.Set("cloud_provider_metadata", holdValue)

	Datadog.Set("cloud_provider_metadata", []string{"aws", "gcp", "azure", "alibaba", "tencent"})
	assert.True(t, IsCloudProviderEnabled("AWS"))
	assert.True(t, IsCloudProviderEnabled("GCP"))
	assert.True(t, IsCloudProviderEnabled("Alibaba"))
	assert.True(t, IsCloudProviderEnabled("Azure"))
	assert.True(t, IsCloudProviderEnabled("Tencent"))

	Datadog.Set("cloud_provider_metadata", []string{"aws"})
	assert.True(t, IsCloudProviderEnabled("AWS"))
	assert.False(t, IsCloudProviderEnabled("GCP"))
	assert.False(t, IsCloudProviderEnabled("Alibaba"))
	assert.False(t, IsCloudProviderEnabled("Azure"))
	assert.False(t, IsCloudProviderEnabled("Tencent"))

	Datadog.Set("cloud_provider_metadata", []string{"tencent"})
	assert.False(t, IsCloudProviderEnabled("AWS"))
	assert.False(t, IsCloudProviderEnabled("GCP"))
	assert.False(t, IsCloudProviderEnabled("Alibaba"))
	assert.False(t, IsCloudProviderEnabled("Azure"))
	assert.True(t, IsCloudProviderEnabled("Tencent"))

	Datadog.Set("cloud_provider_metadata", []string{})
	assert.False(t, IsCloudProviderEnabled("AWS"))
	assert.False(t, IsCloudProviderEnabled("GCP"))
	assert.False(t, IsCloudProviderEnabled("Alibaba"))
	assert.False(t, IsCloudProviderEnabled("Azure"))
	assert.False(t, IsCloudProviderEnabled("Tencent"))
}

func TestEnvNestedConfig(t *testing.T) {
	config := setupConf()
	config.BindEnv("foo.bar.nested")
	t.Setenv("DD_FOO_BAR_NESTED", "baz")

	assert.Equal(t, "baz", config.GetString("foo.bar.nested"))
}

func TestLoadProxyFromStdEnvNoValue(t *testing.T) {
	config := setupConf()

	resetEnv := unsetEnvForTest("NO_PROXY") // CircleCI sets NO_PROXY, so unset it for this test
	defer resetEnv()

	LoadProxyFromEnv(config)
	assert.Nil(t, config.Get("proxy"))

	proxies := GetProxies()
	require.Nil(t, proxies)
}

func TestLoadProxyConfOnly(t *testing.T) {
	config := setupConf()

	// check value loaded before aren't overwrite when no env variables are set
	p := &Proxy{HTTP: "test", HTTPS: "test2", NoProxy: []string{"a", "b", "c"}}
	config.Set("proxy", p)
	// Don't include cloud metadata URL's in no_proxy
	config.Set("use_proxy_for_cloud_metadata", true)

	resetEnv := unsetEnvForTest("NO_PROXY") // CircleCI sets NO_PROXY, so unset it for this test
	defer resetEnv()

	LoadProxyFromEnv(config)
	proxies := GetProxies()
	assert.Equal(t, p, proxies)
}

func TestLoadProxyStdEnvOnly(t *testing.T) {
	config := setupConf()

	// Don't include cloud metadata URL's in no_proxy
	config.Set("use_proxy_for_cloud_metadata", true)

	// uppercase
	t.Setenv("HTTP_PROXY", "http_url")
	t.Setenv("HTTPS_PROXY", "https_url")
	t.Setenv("NO_PROXY", "a,b,c") // comma-separated list

	LoadProxyFromEnv(config)

	proxies := GetProxies()
	assert.Equal(t,
		&Proxy{
			HTTP:    "http_url",
			HTTPS:   "https_url",
			NoProxy: []string{"a", "b", "c"}},
		proxies)

	os.Unsetenv("NO_PROXY")
	os.Unsetenv("HTTPS_PROXY")
	os.Unsetenv("HTTP_PROXY")
	config.Set("proxy", nil)

	// lowercase
	t.Setenv("http_proxy", "http_url2")
	t.Setenv("https_proxy", "https_url2")
	t.Setenv("no_proxy", "1,2,3") // comma-separated list

	LoadProxyFromEnv(config)
	proxies = GetProxies()
	assert.Equal(t,
		&Proxy{
			HTTP:    "http_url2",
			HTTPS:   "https_url2",
			NoProxy: []string{"1", "2", "3"}},
		proxies)
}

func TestLoadProxyDDSpecificEnvOnly(t *testing.T) {
	config := setupConf()
	// Don't include cloud metadata URL's in no_proxy
	config.Set("use_proxy_for_cloud_metadata", true)

	t.Setenv("DD_PROXY_HTTP", "http_url")
	t.Setenv("DD_PROXY_HTTPS", "https_url")
	t.Setenv("DD_PROXY_NO_PROXY", "a b c") // space-separated list

	LoadProxyFromEnv(config)

	proxies := GetProxies()
	assert.Equal(t,
		&Proxy{
			HTTP:    "http_url",
			HTTPS:   "https_url",
			NoProxy: []string{"a", "b", "c"}},
		proxies)
}

func TestLoadProxyDDSpecificEnvPrecedenceOverStdEnv(t *testing.T) {
	config := setupConf()
	// Don't include cloud metadata URL's in no_proxy
	config.Set("use_proxy_for_cloud_metadata", true)

	t.Setenv("DD_PROXY_HTTP", "dd_http_url")
	t.Setenv("DD_PROXY_HTTPS", "dd_https_url")
	t.Setenv("DD_PROXY_NO_PROXY", "a b c")
	t.Setenv("HTTP_PROXY", "env_http_url")
	t.Setenv("HTTPS_PROXY", "env_https_url")
	t.Setenv("NO_PROXY", "d,e,f")

	LoadProxyFromEnv(config)

	proxies := GetProxies()
	assert.Equal(t,
		&Proxy{
			HTTP:    "dd_http_url",
			HTTPS:   "dd_https_url",
			NoProxy: []string{"a", "b", "c"}},
		proxies)
}

func TestLoadProxyStdEnvAndConf(t *testing.T) {
	config := setupConf()
	// Don't include cloud metadata URL's in no_proxy
	config.Set("use_proxy_for_cloud_metadata", true)

	t.Setenv("HTTP_PROXY", "http_env")
	resetNoProxy := unsetEnvForTest("NO_PROXY") // CircleCI sets NO_PROXY, so unset it for this test
	config.Set("proxy.no_proxy", []string{"d", "e", "f"})
	config.Set("proxy.http", "http_conf")
	defer resetNoProxy()

	LoadProxyFromEnv(config)
	proxies := GetProxies()
	assert.Equal(t,
		&Proxy{
			HTTP:    "http_env",
			HTTPS:   "",
			NoProxy: []string{"d", "e", "f"}},
		proxies)
}

func TestLoadProxyDDSpecificEnvAndConf(t *testing.T) {
	config := setupConf()
	// Don't include cloud metadata URL's in no_proxy
	config.Set("use_proxy_for_cloud_metadata", true)

	t.Setenv("DD_PROXY_HTTP", "http_env")
	resetNoProxy := unsetEnvForTest("NO_PROXY") // CircleCI sets NO_PROXY, so unset it for this test
	config.Set("proxy.no_proxy", []string{"d", "e", "f"})
	config.Set("proxy.http", "http_conf")
	defer resetNoProxy()

	LoadProxyFromEnv(config)
	proxies := GetProxies()
	assert.Equal(t,
		&Proxy{
			HTTP:    "http_env",
			HTTPS:   "",
			NoProxy: []string{"d", "e", "f"}},
		proxies)
}

func TestLoadProxyEmptyValuePrecedence(t *testing.T) {
	config := setupConf()
	// Don't include cloud metadata URL's in no_proxy
	config.Set("use_proxy_for_cloud_metadata", true)

	t.Setenv("DD_PROXY_HTTP", "")
	t.Setenv("DD_PROXY_NO_PROXY", "a b c")
	t.Setenv("HTTP_PROXY", "env_http_url")
	t.Setenv("HTTPS_PROXY", "")
	t.Setenv("NO_PROXY", "")
	config.Set("proxy.https", "https_conf")

	LoadProxyFromEnv(config)

	proxies := GetProxies()
	assert.Equal(t,
		&Proxy{
			HTTP:    "",
			HTTPS:   "",
			NoProxy: []string{"a", "b", "c"}},
		proxies)
}

func TestLoadProxyWithoutNoProxy(t *testing.T) {
	config := setupConf()

	// Don't include cloud metadata URL's in no_proxy
	config.Set("use_proxy_for_cloud_metadata", true)

	t.Setenv("DD_PROXY_HTTP", "http_url")
	t.Setenv("DD_PROXY_HTTPS", "https_url")
	resetNoProxy := unsetEnvForTest("NO_PROXY") // CircleCI sets NO_PROXY, so unset it for this test
	defer resetNoProxy()

	LoadProxyFromEnv(config)

	proxies := GetProxies()
	assert.Equal(t,
		&Proxy{
			HTTP:  "http_url",
			HTTPS: "https_url",
		},
		proxies)

	assert.Equal(t, []interface{}{}, config.Get("proxy.no_proxy"))
}

func TestSanitizeAPIKeyConfig(t *testing.T) {
	config := setupConf()

	config.Set("api_key", "foo")
	SanitizeAPIKeyConfig(config, "api_key")
	assert.Equal(t, "foo", config.GetString("api_key"))

	config.Set("api_key", "foo\n")
	SanitizeAPIKeyConfig(config, "api_key")
	assert.Equal(t, "foo", config.GetString("api_key"))

	config.Set("api_key", "foo\n\n")
	SanitizeAPIKeyConfig(config, "api_key")
	assert.Equal(t, "foo", config.GetString("api_key"))

	config.Set("api_key", " \n  foo   \n")
	SanitizeAPIKeyConfig(config, "api_key")
	assert.Equal(t, "foo", config.GetString("api_key"))
}

// TestSecretBackendWithMultipleEndpoints tests an edge case of `viper.AllSettings()` when a config
// key includes the key delimiter. Affects the config package when both secrets and multiple
// endpoints are configured.
// Refer to https://github.com/DataDog/viper/pull/2 for more details.
func TestSecretBackendWithMultipleEndpoints(t *testing.T) {
	conf := setupConf()
	conf.SetConfigFile("./tests/datadog_secrets.yaml")
	// load the configuration
	_, err := LoadDatadogCustom(conf, "datadog_secrets.yaml", true)
	assert.NoError(t, err)

	expectedKeysPerDomain := map[string][]string{
		"https://app.datadoghq.com": {"someapikey", "someotherapikey"},
	}
	keysPerDomain, err := getMultipleEndpointsWithConfig(conf)
	assert.NoError(t, err)
	assert.Equal(t, expectedKeysPerDomain, keysPerDomain)
}

func TestNumWorkers(t *testing.T) {
	config := setupConf()

	config.Set("python_version", "2")
	config.Set("tracemalloc_debug", true)
	config.Set("check_runners", 4)

	setNumWorkers(config)
	workers := config.GetInt("check_runners")
	assert.Equal(t, workers, config.GetInt("check_runners"))

	config.Set("tracemalloc_debug", false)
	setNumWorkers(config)
	workers = config.GetInt("check_runners")
	assert.Equal(t, workers, config.GetInt("check_runners"))

	config.Set("python_version", "3")
	setNumWorkers(config)
	workers = config.GetInt("check_runners")
	assert.Equal(t, workers, config.GetInt("check_runners"))

	config.Set("tracemalloc_debug", true)
	setNumWorkers(config)
	workers = config.GetInt("check_runners")
	assert.Equal(t, workers, 1)
}

// TestOverrides validates that the config overrides system works well.
func TestApplyOverrides(t *testing.T) {
	assert := assert.New(t)

	datadogYaml := `
dd_url: "https://app.datadoghq.eu"
api_key: fakeapikey

external_config:
  external_agent_dd_url: "https://custom.external-agent.datadoghq.eu"
`
	AddOverrides(map[string]interface{}{
		"api_key": "overrided",
	})

	config := setupConfFromYAML(datadogYaml)
	applyOverrideFuncs(config)

	assert.Equal(config.GetString("api_key"), "overrided", "the api key should have been overrided")
	assert.Equal(config.GetString("dd_url"), "https://app.datadoghq.eu", "this shouldn't be overrided")

	AddOverrides(map[string]interface{}{
		"dd_url": "http://localhost",
	})
	applyOverrideFuncs(config)

	assert.Equal(config.GetString("api_key"), "overrided", "the api key should have been overrided")
	assert.Equal(config.GetString("dd_url"), "http://localhost", "this dd_url should have been overrided")
}

func TestDogstatsdMappingProfilesOk(t *testing.T) {
	datadogYaml := `
dogstatsd_mapper_profiles:
  - name: "airflow"
    prefix: "airflow."
    mappings:
      - match: 'airflow\.job\.duration_sec\.(.*)'
        name: "airflow.job.duration"
        match_type: "regex"
        tags:
          job_type: "$1"
          job_name: "$2"
      - match: "airflow.job.size.*.*"
        name: "airflow.job.size"
        tags:
          foo: "$1"
          bar: "$2"
  - name: "profile2"
    prefix: "profile2."
    mappings:
      - match: "profile2.hello.*"
        name: "profile2.hello"
        tags:
          foo: "$1"
`
	testConfig := setupConfFromYAML(datadogYaml)

	profiles, err := getDogstatsdMappingProfilesConfig(testConfig)

	expectedProfiles := []MappingProfile{
		{
			Name:   "airflow",
			Prefix: "airflow.",
			Mappings: []MetricMapping{
				{
					Match:     "airflow\\.job\\.duration_sec\\.(.*)",
					MatchType: "regex",
					Name:      "airflow.job.duration",
					Tags:      map[string]string{"job_type": "$1", "job_name": "$2"},
				},
				{
					Match: "airflow.job.size.*.*",
					Name:  "airflow.job.size",
					Tags:  map[string]string{"foo": "$1", "bar": "$2"},
				},
			},
		},
		{
			Name:   "profile2",
			Prefix: "profile2.",
			Mappings: []MetricMapping{
				{
					Match: "profile2.hello.*",
					Name:  "profile2.hello",
					Tags:  map[string]string{"foo": "$1"},
				},
			},
		},
	}

	assert.Nil(t, err)
	assert.EqualValues(t, expectedProfiles, profiles)
}

func TestDogstatsdMappingProfilesEmpty(t *testing.T) {
	datadogYaml := `
dogstatsd_mapper_profiles:
`
	testConfig := setupConfFromYAML(datadogYaml)

	profiles, err := getDogstatsdMappingProfilesConfig(testConfig)

	var expectedProfiles []MappingProfile

	assert.Nil(t, err)
	assert.EqualValues(t, expectedProfiles, profiles)
}

func TestDogstatsdMappingProfilesError(t *testing.T) {
	datadogYaml := `
dogstatsd_mapper_profiles:
  - abc
`
	testConfig := setupConfFromYAML(datadogYaml)
	profiles, err := getDogstatsdMappingProfilesConfig(testConfig)

	expectedErrorMsg := "Could not parse dogstatsd_mapper_profiles"
	assert.NotNil(t, err)
	assert.Contains(t, err.Error(), expectedErrorMsg)
	assert.Empty(t, profiles)
}

func TestDogstatsdMappingProfilesEnv(t *testing.T) {
	env := "DD_DOGSTATSD_MAPPER_PROFILES"
	t.Setenv(env, `[{"name":"another_profile","prefix":"abcd","mappings":[{"match":"airflow\\.dag_processing\\.last_runtime\\.(.*)","match_type":"regex","name":"foo","tags":{"a":"$1","b":"$2"}}]},{"name":"some_other_profile","prefix":"some_other_profile.","mappings":[{"match":"some_other_profile.*","name":"some_other_profile.abc","tags":{"a":"$1"}}]}]`)
	expected := []MappingProfile{
		{Name: "another_profile", Prefix: "abcd", Mappings: []MetricMapping{
			{Match: "airflow\\.dag_processing\\.last_runtime\\.(.*)", MatchType: "regex", Name: "foo", Tags: map[string]string{"a": "$1", "b": "$2"}},
		}},
		{Name: "some_other_profile", Prefix: "some_other_profile.", Mappings: []MetricMapping{
			{Match: "some_other_profile.*", Name: "some_other_profile.abc", Tags: map[string]string{"a": "$1"}},
		}},
	}
	mappings, _ := GetDogstatsdMappingProfiles()
	assert.Equal(t, mappings, expected)
}

func TestGetValidHostAliasesWithConfig(t *testing.T) {
	config := setupConfFromYAML(`host_aliases: ["foo", "-bar"]`)
	assert.EqualValues(t, getValidHostAliasesWithConfig(config), []string{"foo"})
}

func TestNetworkDevicesNamespace(t *testing.T) {
	datadogYaml := `
network_devices:
`
	config := setupConfFromYAML(datadogYaml)
	assert.Equal(t, "default", config.GetString("network_devices.namespace"))

	datadogYaml = `
network_devices:
  namespace: dev
`
	config = setupConfFromYAML(datadogYaml)
	assert.Equal(t, "dev", config.GetString("network_devices.namespace"))
}

func TestPrometheusScrapeChecksTransformer(t *testing.T) {
	input := `[{"configurations":[{"timeout":5,"send_distribution_buckets":true}],"autodiscovery":{"kubernetes_container_names":["my-app"],"kubernetes_annotations":{"include":{"custom_label":"true"}}}}]`
	expected := []*types.PrometheusCheck{
		{
			Instances: []*types.OpenmetricsInstance{{Timeout: 5, DistributionBuckets: true}},
			AD:        &types.ADConfig{KubeContainerNames: []string{"my-app"}, KubeAnnotations: &types.InclExcl{Incl: map[string]string{"custom_label": "true"}}},
		},
	}

	assert.EqualValues(t, PrometheusScrapeChecksTransformer(input), expected)
}

func TestUsePodmanLogsAndDockerPathOverride(t *testing.T) {
	// If use_podman_logs is true and docker_path_override is set, the config should return an error
	datadogYaml := `
logs_config:
  use_podman_logs: true
  docker_path_override: "/custom/path"
`

	config := setupConfFromYAML(datadogYaml)
	err := checkConflictingOptions(config)

	assert.NotNil(t, err)
}

func TestSetupFipsEndpoints(t *testing.T) {
	datadogYaml := `
dd_url: https://somehost:1234

skip_ssl_validation: true

apm_config:
  apm_dd_url: https://somehost:1234
  profiling_dd_url: https://somehost:1234
  telemetry:
    dd_url: https://somehost:1234

process_config:
  process_dd_url:  https://somehost:1234

logs_config:
  use_http: false
  logs_no_ssl: false
  logs_dd_url: somehost:1234

database_monitoring:
  metrics:
    dd_url: somehost:1234
  activity:
    dd_url: somehost:1234
  samples:
    dd_url: somehost:1234

network_devices:
  metadata:
    dd_url: somehost:1234

orchestrator_explorer:
    orchestrator_dd_url: https://somehost:1234

runtime_security_config:
    endpoints:
        logs_dd_url: somehost:1234

proxy:
  http: http://localhost:1234
  https: https://localhost:1234
`
	expectedURL := "somehost:1234"
	expectedHTTPURL := "https://" + expectedURL
	testConfig := setupConfFromYAML(datadogYaml)
	LoadProxyFromEnv(testConfig)
	err := setupFipsEndpoints(testConfig)
	require.NoError(t, err)

	assertFipsProxyExpectedConfig(t, expectedHTTPURL, expectedURL, false, testConfig)
	assert.Equal(t, false, testConfig.GetBool("logs_config.use_http"))
	assert.Equal(t, false, testConfig.GetBool("logs_config.logs_no_ssl"))
	assert.Equal(t, false, testConfig.GetBool("runtime_security_config.endpoints.use_http"))
	assert.Equal(t, false, testConfig.GetBool("runtime_security_config.endpoints.logs_no_ssl"))
	assert.NotNil(t, GetProxies())
	// reseting proxies
	proxies = nil

	datadogYamlFips := datadogYaml + `
fips:
  enabled: true
  local_address: localhost
  port_range_start: 5000
  https: false
`

	expectedURL = "localhost:50"
	expectedHTTPURL = "http://" + expectedURL
	testConfig = setupConfFromYAML(datadogYamlFips)
	LoadProxyFromEnv(testConfig)
	err = setupFipsEndpoints(testConfig)
	require.NoError(t, err)

	assertFipsProxyExpectedConfig(t, expectedHTTPURL, expectedURL, true, testConfig)
	assert.Equal(t, true, testConfig.GetBool("logs_config.use_http"))
	assert.Equal(t, true, testConfig.GetBool("logs_config.logs_no_ssl"))
	assert.Equal(t, true, testConfig.GetBool("runtime_security_config.endpoints.use_http"))
	assert.Equal(t, true, testConfig.GetBool("runtime_security_config.endpoints.logs_no_ssl"))
	assert.Nil(t, GetProxies())

	datadogYamlFips = datadogYaml + `
fips:
  enabled: true
  local_address: localhost
  port_range_start: 5000
  https: true
  tls_verify: false
`

	expectedHTTPURL = "https://" + expectedURL
	testConfig = setupConfFromYAML(datadogYamlFips)
	testConfig.Set("skip_ssl_validation", false) // should be overridden by fips.tls_verify
	LoadProxyFromEnv(testConfig)
	err = setupFipsEndpoints(testConfig)
	require.NoError(t, err)

	assertFipsProxyExpectedConfig(t, expectedHTTPURL, expectedURL, true, testConfig)
	assert.Equal(t, true, testConfig.GetBool("logs_config.use_http"))
	assert.Equal(t, false, testConfig.GetBool("logs_config.logs_no_ssl"))
	assert.Equal(t, true, testConfig.GetBool("runtime_security_config.endpoints.use_http"))
	assert.Equal(t, false, testConfig.GetBool("runtime_security_config.endpoints.logs_no_ssl"))
	assert.Equal(t, true, testConfig.GetBool("skip_ssl_validation"))
	assert.Nil(t, GetProxies())

	testConfig.Set("skip_ssl_validation", true) // should be overridden by fips.tls_verify
	testConfig.Set("fips.tls_verify", true)
	LoadProxyFromEnv(testConfig)
	err = setupFipsEndpoints(testConfig)
	require.NoError(t, err)

	assert.Equal(t, false, testConfig.GetBool("skip_ssl_validation"))
	assert.Nil(t, GetProxies())
}

func assertFipsProxyExpectedConfig(t *testing.T, expectedBaseHTTPURL, expectedBaseURL string, rng bool, c Config) {
	if rng {
		assert.Equal(t, expectedBaseHTTPURL+"01", c.GetString("dd_url"))
		assert.Equal(t, expectedBaseHTTPURL+"02", c.GetString("apm_config.apm_dd_url"))
		assert.Equal(t, expectedBaseHTTPURL+"03"+"/api/v2/profile", c.GetString("apm_config.profiling_dd_url"))
		assert.Equal(t, expectedBaseHTTPURL+"10", c.GetString("apm_config.telemetry.dd_url"))
		assert.Equal(t, expectedBaseHTTPURL+"04", c.GetString("process_config.process_dd_url"))
		assert.Equal(t, expectedBaseURL+"05", c.GetString("logs_config.logs_dd_url"))
		assert.Equal(t, expectedBaseURL+"06", c.GetString("database_monitoring.metrics.dd_url"))
		assert.Equal(t, expectedBaseURL+"06", c.GetString("database_monitoring.activity.dd_url"))
		assert.Equal(t, expectedBaseURL+"07", c.GetString("database_monitoring.samples.dd_url"))
		assert.Equal(t, expectedBaseURL+"08", c.GetString("network_devices.metadata.dd_url"))
		assert.Equal(t, expectedBaseHTTPURL+"12", c.GetString("orchestrator_explorer.orchestrator_dd_url"))
		assert.Equal(t, expectedBaseURL+"13", c.GetString("runtime_security_config.endpoints.logs_dd_url"))

	} else {
		assert.Equal(t, expectedBaseHTTPURL, c.GetString("dd_url"))
		assert.Equal(t, expectedBaseHTTPURL, c.GetString("apm_config.apm_dd_url"))
		assert.Equal(t, expectedBaseHTTPURL, c.GetString("apm_config.profiling_dd_url")) // Omitting "/api/v2/profile" as the config is not overwritten
		assert.Equal(t, expectedBaseHTTPURL, c.GetString("apm_config.telemetry.dd_url"))
		assert.Equal(t, expectedBaseHTTPURL, c.GetString("process_config.process_dd_url"))
		assert.Equal(t, expectedBaseURL, c.GetString("logs_config.logs_dd_url"))
		assert.Equal(t, expectedBaseURL, c.GetString("database_monitoring.metrics.dd_url"))
		assert.Equal(t, expectedBaseURL, c.GetString("database_monitoring.activity.dd_url"))
		assert.Equal(t, expectedBaseURL, c.GetString("database_monitoring.samples.dd_url"))
		assert.Equal(t, expectedBaseURL, c.GetString("network_devices.metadata.dd_url"))
		assert.Equal(t, expectedBaseHTTPURL, c.GetString("orchestrator_explorer.orchestrator_dd_url"))
		assert.Equal(t, expectedBaseURL, c.GetString("runtime_security_config.endpoints.logs_dd_url"))
	}
}

func TestSetupFipsEndpointsNonLocalAddress(t *testing.T) {
	datadogYaml := `
fips:
  enabled: true
  local_address: 1.2.3.4
  port_range_start: 5000
`

	testConfig := setupConfFromYAML(datadogYaml)
	err := setupFipsEndpoints(testConfig)
	require.Error(t, err)
}
