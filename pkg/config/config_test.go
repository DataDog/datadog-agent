// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-2020 Datadog, Inc.

package config

import (
	"bytes"
	"log"
	"os"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func setupConf() Config {
	conf := NewConfig("datadog", "DD", strings.NewReplacer(".", "_"))
	initConfig(conf)
	return conf
}

func setupConfFromYAML(yamlConfig string) Config {
	conf := NewConfig("datadog", "DD", strings.NewReplacer(".", "_"))
	initConfig(conf)
	conf.SetConfigType("yaml")
	e := conf.ReadConfig(bytes.NewBuffer([]byte(yamlConfig)))
	if e != nil {
		log.Println(e)
	}
	return conf
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
	os.Setenv("DD_API_KEY", "fakeapikey")
	os.Setenv("DD_SITE", "datadoghq.eu")
	defer os.Unsetenv("DD_API_KEY")
	defer os.Unsetenv("DD_SITE")
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
	os.Setenv("DD_API_KEY", "fakeapikey")
	os.Setenv("DD_DD_URL", "https://app.datadoghq.eu")
	os.Setenv("DD_EXTERNAL_CONFIG_EXTERNAL_AGENT_DD_URL", "https://custom.external-agent.datadoghq.com")
	defer os.Unsetenv("DD_API_KEY")
	defer os.Unsetenv("DD_DD_URL")
	defer os.Unsetenv("DD_EXTERNAL_CONFIG_EXTERNAL_AGENT_DD_URL")
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
	os.Setenv("DD_API_KEY", "fakeapikey")
	os.Setenv("DD_ADDITIONAL_ENDPOINTS", "{\"https://foo.datadoghq.com\": [\"someapikey\"]}")
	defer os.Unsetenv("DD_API_KEY")
	defer os.Unsetenv("DD_ADDITIONAL_ENDPOINTS")

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
	newURL, err := AddAgentVersionToDomain("https://app.datadoghq.com", "app")
	require.Nil(t, err)
	assert.Equal(t, "https://"+getDomainPrefix("app")+".datadoghq.com", newURL)

	newURL, err = AddAgentVersionToDomain("https://app.datadoghq.com", "flare")
	require.Nil(t, err)
	assert.Equal(t, "https://"+getDomainPrefix("flare")+".datadoghq.com", newURL)

	newURL, err = AddAgentVersionToDomain("https://app.datadoghq.eu", "app")
	require.Nil(t, err)
	assert.Equal(t, "https://"+getDomainPrefix("app")+".datadoghq.eu", newURL)

	newURL, err = AddAgentVersionToDomain("https://app.datadoghq.eu", "flare")
	require.Nil(t, err)
	assert.Equal(t, "https://"+getDomainPrefix("flare")+".datadoghq.eu", newURL)

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
	os.Setenv("DD_FOO_BAR_NESTED", "baz")

	assert.Equal(t, "baz", config.GetString("foo.bar.nested"))
	os.Unsetenv("DD_FOO_BAR_NESTED")
}

func TestLoadProxyFromStdEnvNoValue(t *testing.T) {
	config := setupConf()

	// circleCI set some proxy setting
	ciValue := os.Getenv("NO_PROXY")
	os.Unsetenv("NO_PROXY")
	defer os.Setenv("NO_PROXY", ciValue)

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

	// circleCI set some proxy setting
	ciValue := os.Getenv("NO_PROXY")
	os.Unsetenv("NO_PROXY")
	defer os.Setenv("NO_PROXY", ciValue)

	loadProxyFromEnv(config)
	proxies := GetProxies()
	assert.Equal(t, p, proxies)
}

func TestLoadProxyStdEnvOnly(t *testing.T) {
	config := setupConf()

	// uppercase
	os.Setenv("HTTP_PROXY", "http_url")
	os.Setenv("HTTPS_PROXY", "https_url")
	os.Setenv("NO_PROXY", "a,b,c") // comma-separated list

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
	os.Setenv("http_proxy", "http_url2")
	os.Setenv("https_proxy", "https_url2")
	os.Setenv("no_proxy", "1,2,3") // comma-separated list

	loadProxyFromEnv(config)
	proxies = GetProxies()
	assert.Equal(t,
		&Proxy{
			HTTP:    "http_url2",
			HTTPS:   "https_url2",
			NoProxy: []string{"1", "2", "3"}},
		proxies)

	os.Unsetenv("no_proxy")
	os.Unsetenv("https_proxy")
	os.Unsetenv("http_proxy")
}

func TestLoadProxyDDSpecificEnvOnly(t *testing.T) {
	config := setupConf()

	os.Setenv("DD_PROXY_HTTP", "http_url")
	os.Setenv("DD_PROXY_HTTPS", "https_url")
	os.Setenv("DD_PROXY_NO_PROXY", "a b c") // space-separated list

	loadProxyFromEnv(config)

	proxies := GetProxies()
	assert.Equal(t,
		&Proxy{
			HTTP:    "http_url",
			HTTPS:   "https_url",
			NoProxy: []string{"a", "b", "c"}},
		proxies)

	os.Unsetenv("DD_PROXY_HTTP")
	os.Unsetenv("DD_PROXY_HTTPS")
	os.Unsetenv("DD_PROXY_NO_PROXY")
}

func TestLoadProxyDDSpecificEnvPrecedenceOverStdEnv(t *testing.T) {
	config := setupConf()

	os.Setenv("DD_PROXY_HTTP", "dd_http_url")
	os.Setenv("DD_PROXY_HTTPS", "dd_https_url")
	os.Setenv("DD_PROXY_NO_PROXY", "a b c")
	os.Setenv("HTTP_PROXY", "env_http_url")
	os.Setenv("HTTPS_PROXY", "env_https_url")
	os.Setenv("NO_PROXY", "d,e,f")

	loadProxyFromEnv(config)

	proxies := GetProxies()
	assert.Equal(t,
		&Proxy{
			HTTP:    "dd_http_url",
			HTTPS:   "dd_https_url",
			NoProxy: []string{"a", "b", "c"}},
		proxies)

	os.Unsetenv("NO_PROXY")
	os.Unsetenv("HTTPS_PROXY")
	os.Unsetenv("HTTP_PROXY")
	os.Unsetenv("DD_PROXY_HTTP")
	os.Unsetenv("DD_PROXY_HTTPS")
	os.Unsetenv("DD_PROXY_NO_PROXY")
}

func TestLoadProxyStdEnvAndConf(t *testing.T) {
	config := setupConf()

	os.Setenv("HTTP_PROXY", "http_env")
	config.Set("proxy.no_proxy", []string{"d", "e", "f"})
	config.Set("proxy.http", "http_conf")
	defer os.Unsetenv("HTTP")

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

	os.Setenv("DD_PROXY_HTTP", "http_env")
	config.Set("proxy.no_proxy", []string{"d", "e", "f"})
	config.Set("proxy.http", "http_conf")
	defer os.Unsetenv("DD_PROXY_HTTP")

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

	os.Setenv("DD_PROXY_HTTP", "")
	os.Setenv("DD_PROXY_NO_PROXY", "a b c")
	os.Setenv("HTTP_PROXY", "env_http_url")
	os.Setenv("HTTPS_PROXY", "")
	os.Setenv("NO_PROXY", "")
	config.Set("proxy.https", "https_conf")

	loadProxyFromEnv(config)

	proxies := GetProxies()
	assert.Equal(t,
		&Proxy{
			HTTP:    "",
			HTTPS:   "",
			NoProxy: []string{"a", "b", "c"}},
		proxies)

	os.Unsetenv("NO_PROXY")
	os.Unsetenv("HTTPS_PROXY")
	os.Unsetenv("HTTP_PROXY")
	os.Unsetenv("DD_PROXY_HTTP")
	os.Unsetenv("DD_PROXY_HTTPS")
	os.Unsetenv("DD_PROXY_NO_PROXY")
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
	err := load(conf, "datadog_secrets.yaml", true)
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
	applyOverrides(config)

	assert.Equal(config.GetString("api_key"), "overrided", "the api key should have been overrided")
	assert.Equal(config.GetString("dd_url"), "https://app.datadoghq.eu", "this shouldn't be overrided")

	AddOverrides(map[string]interface{}{
		"dd_url": "http://localhost",
	})
	applyOverrides(config)

	assert.Equal(config.GetString("api_key"), "overrided", "the api key should have been overrided")
	assert.Equal(config.GetString("dd_url"), "http://localhost", "this dd_url should have been overrided")
}

func TestDogstatsdMappingProfilesOk(t *testing.T) {
	datadogYaml := `
dogstatsd_mapper_profiles:
  - name: "airflow"
    prefix: "airflow."
    mappings:
      - match: "airflow.job.duration_sec.*.*"
        name: "airflow.job.duration"
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
					Match: "airflow.job.duration_sec.*.*",
					Name:  "airflow.job.duration",
					Tags:  map[string]string{"job_type": "$1", "job_name": "$2"},
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
