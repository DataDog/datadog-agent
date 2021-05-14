// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package config

import (
	"bytes"
	"log"
	"os"
	"strings"
	"testing"

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

func setEnvForTest(env, value string) (reset func()) {
	oldValue, ok := os.LookupEnv(env)
	os.Setenv(env, value)

	return func() {
		if !ok {
			os.Unsetenv(env)
		} else {
			os.Setenv(env, oldValue)
		}
	}
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
	assert.Equal(t, []string{"aws", "gcp", "azure", "alibaba"}, config.GetStringSlice("cloud_provider_metadata"))
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

func TestSiteEnvVar(t *testing.T) {
	resetAPIKey := setEnvForTest("DD_API_KEY", "fakeapikey")
	resetSite := setEnvForTest("DD_SITE", "datadoghq.eu")
	defer resetAPIKey()
	defer resetSite()
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

func TestDDURLEnvVar(t *testing.T) {
	resetAPIKey := setEnvForTest("DD_API_KEY", "fakeapikey")
	resetURL := setEnvForTest("DD_DD_URL", "https://app.datadoghq.eu")
	resetExternalURL := setEnvForTest("DD_EXTERNAL_CONFIG_EXTERNAL_AGENT_DD_URL", "https://custom.external-agent.datadoghq.com")
	defer resetAPIKey()
	defer resetURL()
	defer resetExternalURL()
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
	resetAPIKey := setEnvForTest("DD_API_KEY", "fakeapikey")
	resetAdditionalEndpoints := setEnvForTest("DD_ADDITIONAL_ENDPOINTS", "{\"https://foo.datadoghq.com\": [\"someapikey\"]}")
	defer resetAPIKey()
	defer resetAdditionalEndpoints()

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
	// US
	newURL, err := AddAgentVersionToDomain("https://app.datadoghq.com", "app")
	require.Nil(t, err)
	assert.Equal(t, "https://"+getDomainPrefix("app")+".datadoghq.com", newURL)

	newURL, err = AddAgentVersionToDomain("https://app.datadoghq.com", "flare")
	require.Nil(t, err)
	assert.Equal(t, "https://"+getDomainPrefix("flare")+".datadoghq.com", newURL)

	// EU
	newURL, err = AddAgentVersionToDomain("https://app.datadoghq.eu", "app")
	require.Nil(t, err)
	assert.Equal(t, "https://"+getDomainPrefix("app")+".datadoghq.eu", newURL)

	newURL, err = AddAgentVersionToDomain("https://app.datadoghq.eu", "flare")
	require.Nil(t, err)
	assert.Equal(t, "https://"+getDomainPrefix("flare")+".datadoghq.eu", newURL)

	// Additional site
	newURL, err = AddAgentVersionToDomain("https://app.us2.datadoghq.com", "app")
	require.Nil(t, err)
	assert.Equal(t, "https://"+getDomainPrefix("app")+".us2.datadoghq.com", newURL)

	newURL, err = AddAgentVersionToDomain("https://app.us2.datadoghq.com", "flare")
	require.Nil(t, err)
	assert.Equal(t, "https://"+getDomainPrefix("flare")+".us2.datadoghq.com", newURL)

	// Custom DD URL: leave unchanged
	newURL, err = AddAgentVersionToDomain("https://custom.datadoghq.com", "app")
	require.Nil(t, err)
	assert.Equal(t, "https://custom.datadoghq.com", newURL)

	newURL, err = AddAgentVersionToDomain("https://custom.datadoghq.com", "flare")
	require.Nil(t, err)
	assert.Equal(t, "https://custom.datadoghq.com", newURL)

	// Custom DD URL with 'agent' subdomain: leave unchanged
	newURL, err = AddAgentVersionToDomain("https://custom.agent.datadoghq.com", "app")
	require.Nil(t, err)
	assert.Equal(t, "https://custom.agent.datadoghq.com", newURL)

	newURL, err = AddAgentVersionToDomain("https://custom.agent.datadoghq.com", "flare")
	require.Nil(t, err)
	assert.Equal(t, "https://custom.agent.datadoghq.com", newURL)

	// Custom DD URL: unclear if anyone is actually using such a URL, but for now leave unchanged
	newURL, err = AddAgentVersionToDomain("https://app.custom.datadoghq.com", "app")
	require.Nil(t, err)
	assert.Equal(t, "https://app.custom.datadoghq.com", newURL)

	newURL, err = AddAgentVersionToDomain("https://app.custom.datadoghq.com", "flare")
	require.Nil(t, err)
	assert.Equal(t, "https://app.custom.datadoghq.com", newURL)

	// Custom top-level domain: unclear if anyone is actually using this, but for now leave unchanged
	newURL, err = AddAgentVersionToDomain("https://app.datadoghq.internal", "app")
	require.Nil(t, err)
	assert.Equal(t, "https://app.datadoghq.internal", newURL)

	newURL, err = AddAgentVersionToDomain("https://app.datadoghq.internal", "flare")
	require.Nil(t, err)
	assert.Equal(t, "https://app.datadoghq.internal", newURL)

	// DD URL set to proxy, leave unchanged
	newURL, err = AddAgentVersionToDomain("https://app.myproxy.com", "app")
	require.Nil(t, err)
	assert.Equal(t, "https://app.myproxy.com", newURL)
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
	resetEnv := setEnvForTest("DD_FOO_BAR_NESTED", "baz")
	defer resetEnv()

	assert.Equal(t, "baz", config.GetString("foo.bar.nested"))
}

func TestLoadProxyFromStdEnvNoValue(t *testing.T) {
	config := setupConf()

	resetEnv := unsetEnvForTest("NO_PROXY") // CircleCI sets NO_PROXY, so unset it for this test
	defer resetEnv()

	loadProxyFromEnv(config)
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

	loadProxyFromEnv(config)
	proxies := GetProxies()
	assert.Equal(t, p, proxies)
}

func TestLoadProxyStdEnvOnly(t *testing.T) {
	config := setupConf()

	// Don't include cloud metadata URL's in no_proxy
	config.Set("use_proxy_for_cloud_metadata", true)

	// uppercase
	resetHTTPProxyUpper := setEnvForTest("HTTP_PROXY", "http_url")
	resetHTTPSProxyUpper := setEnvForTest("HTTPS_PROXY", "https_url")
	resetNoProxyUpper := setEnvForTest("NO_PROXY", "a,b,c") // comma-separated list
	defer resetHTTPProxyUpper()
	defer resetHTTPSProxyUpper()
	defer resetNoProxyUpper()

	loadProxyFromEnv(config)

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
	resetHTTPProxyLower := setEnvForTest("http_proxy", "http_url2")
	resetHTTPSProxyLower := setEnvForTest("https_proxy", "https_url2")
	resetNoProxyLower := setEnvForTest("no_proxy", "1,2,3") // comma-separated list
	defer resetHTTPProxyLower()
	defer resetHTTPSProxyLower()
	defer resetNoProxyLower()

	loadProxyFromEnv(config)
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

	resetHTTPProxy := setEnvForTest("DD_PROXY_HTTP", "http_url")
	resetHTTPSProxy := setEnvForTest("DD_PROXY_HTTPS", "https_url")
	resetNoProxy := setEnvForTest("DD_PROXY_NO_PROXY", "a b c") // space-separated list
	defer resetHTTPProxy()
	defer resetHTTPSProxy()
	defer resetNoProxy()

	loadProxyFromEnv(config)

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

	resetDdHTTPProxy := setEnvForTest("DD_PROXY_HTTP", "dd_http_url")
	resetDdHTTPSProxy := setEnvForTest("DD_PROXY_HTTPS", "dd_https_url")
	resetDdNoProxy := setEnvForTest("DD_PROXY_NO_PROXY", "a b c")
	resetHTTPProxy := setEnvForTest("HTTP_PROXY", "env_http_url")
	resetHTTPSProxy := setEnvForTest("HTTPS_PROXY", "env_https_url")
	resetNoProxy := setEnvForTest("NO_PROXY", "d,e,f")
	defer resetDdHTTPProxy()
	defer resetDdHTTPSProxy()
	defer resetDdNoProxy()
	defer resetHTTPProxy()
	defer resetHTTPSProxy()
	defer resetNoProxy()

	loadProxyFromEnv(config)

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

	resetHTTPProxy := setEnvForTest("HTTP_PROXY", "http_env")
	resetNoProxy := unsetEnvForTest("NO_PROXY") // CircleCI sets NO_PROXY, so unset it for this test
	config.Set("proxy.no_proxy", []string{"d", "e", "f"})
	config.Set("proxy.http", "http_conf")
	defer resetHTTPProxy()
	defer resetNoProxy()

	loadProxyFromEnv(config)
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

	resetHTTPProxy := setEnvForTest("DD_PROXY_HTTP", "http_env")
	resetNoProxy := unsetEnvForTest("NO_PROXY") // CircleCI sets NO_PROXY, so unset it for this test
	config.Set("proxy.no_proxy", []string{"d", "e", "f"})
	config.Set("proxy.http", "http_conf")
	defer resetHTTPProxy()
	defer resetNoProxy()

	loadProxyFromEnv(config)
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

	resetDdHTTPProxy := setEnvForTest("DD_PROXY_HTTP", "")
	resetDdNoProxy := setEnvForTest("DD_PROXY_NO_PROXY", "a b c")
	resetHTTPProxy := setEnvForTest("HTTP_PROXY", "env_http_url")
	resetHTTPSProxy := setEnvForTest("HTTPS_PROXY", "")
	resetNoProxy := setEnvForTest("NO_PROXY", "")
	config.Set("proxy.https", "https_conf")
	defer resetDdHTTPProxy()
	defer resetDdNoProxy()
	defer resetHTTPProxy()
	defer resetHTTPSProxy()
	defer resetNoProxy()

	loadProxyFromEnv(config)

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

	resetHTTPProxy := setEnvForTest("DD_PROXY_HTTP", "http_url")
	resetHTTPSProxy := setEnvForTest("DD_PROXY_HTTPS", "https_url")
	resetNoProxy := unsetEnvForTest("NO_PROXY") // CircleCI sets NO_PROXY, so unset it for this test
	defer resetHTTPProxy()
	defer resetHTTPSProxy()
	defer resetNoProxy()

	loadProxyFromEnv(config)

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
	_, err := load(conf, "datadog_secrets.yaml", true)
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
	err := os.Setenv(env, `[{"name":"another_profile","prefix":"abcd","mappings":[{"match":"airflow\\.dag_processing\\.last_runtime\\.(.*)","match_type":"regex","name":"foo","tags":{"a":"$1","b":"$2"}}]},{"name":"some_other_profile","prefix":"some_other_profile.","mappings":[{"match":"some_other_profile.*","name":"some_other_profile.abc","tags":{"a":"$1"}}]}]`)
	assert.Nil(t, err)
	defer os.Unsetenv(env)
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

func TestPrometheusScrapeChecksEnv(t *testing.T) {
	env := "DD_PROMETHEUS_SCRAPE_CHECKS"
	err := os.Setenv(env, `[{"configurations":[{"timeout":5,"send_distribution_buckets":true}],"autodiscovery":{"kubernetes_container_names":["my-app"],"kubernetes_annotations":{"include":{"custom_label":"true"}}}}]`)
	assert.Nil(t, err)
	defer os.Unsetenv(env)
	expected := []*types.PrometheusCheck{
		{
			Instances: []*types.OpenmetricsInstance{{Timeout: 5, DistributionBuckets: true}},
			AD:        &types.ADConfig{KubeContainerNames: []string{"my-app"}, KubeAnnotations: &types.InclExcl{Incl: map[string]string{"custom_label": "true"}}},
		},
	}
	checks := []*types.PrometheusCheck{}
	assert.NoError(t, Datadog.UnmarshalKey("prometheus_scrape.checks", &checks))
	assert.EqualValues(t, checks, expected)
}

func TestGetValidHostAliasesWithConfig(t *testing.T) {
	config := setupConfFromYAML(`host_aliases: ["foo", "-bar"]`)
	assert.EqualValues(t, getValidHostAliasesWithConfig(config), []string{"foo"})
}
