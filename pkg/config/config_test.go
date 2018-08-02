// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2018 Datadog, Inc.

package config

import (
	"bytes"
	"os"
	"testing"

	"github.com/spf13/viper"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestDefaults(t *testing.T) {
	assert.Equal(t, Datadog.GetString("dd_url"), "https://app.datadoghq.com")
}

func setupViperConf(yamlConfig string) *viper.Viper {
	conf := viper.New()
	conf.SetConfigType("yaml")
	conf.ReadConfig(bytes.NewBuffer([]byte(yamlConfig)))
	return conf
}

func TestGetMultipleEndpoints(t *testing.T) {
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

	testConfig := setupViperConf(datadogYaml)

	multipleEndpoints, err := getMultipleEndpoints(testConfig)

	expectedMultipleEndpoints := map[string][]string{
		"https://foo.datadoghq.com": {
			"someapikey",
		},
		"https://" + getDomainPrefix("app") + ".datadoghq.com": {
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

	testConfig := setupViperConf(datadogYaml)

	multipleEndpoints, err := getMultipleEndpoints(testConfig)

	expectedMultipleEndpoints := map[string][]string{
		"https://" + getDomainPrefix("app") + ".datadoghq.com": {
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

	testConfig := setupViperConf(datadogYaml)

	multipleEndpoints, err := getMultipleEndpoints(testConfig)

	expectedMultipleEndpoints := map[string][]string{
		"https://" + getDomainPrefix("app") + ".datadoghq.com": {
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

	testConfig := setupViperConf(datadogYaml)

	multipleEndpoints, err := getMultipleEndpoints(testConfig)

	expectedMultipleEndpoints := map[string][]string{
		"https://" + getDomainPrefix("app") + ".datadoghq.com": {
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
	newURL, err := addAgentVersionToDomain("https://app.datadoghq.com", "app")
	require.Nil(t, err)
	assert.Equal(t, "https://"+getDomainPrefix("app")+".datadoghq.com", newURL)

	newURL, err = addAgentVersionToDomain("https://app.datadoghq.com", "flare")
	require.Nil(t, err)
	assert.Equal(t, "https://"+getDomainPrefix("flare")+".datadoghq.com", newURL)

	newURL, err = addAgentVersionToDomain("https://app.myproxy.com", "app")
	require.Nil(t, err)
	assert.Equal(t, "https://app.myproxy.com", newURL)
}

func TestEnvNestedConfig(t *testing.T) {
	Datadog.BindEnv("foo.bar.nested")
	os.Setenv("DD_FOO_BAR_NESTED", "baz")

	assert.Equal(t, "baz", Datadog.GetString("foo.bar.nested"))
	os.Unsetenv("DD_FOO_BAR_NESTED")
}

func TestLoadProxyFromStdEnvNoValue(t *testing.T) {
	// circleCI set some proxy setting
	ciValue := os.Getenv("NO_PROXY")
	os.Unsetenv("NO_PROXY")
	defer os.Setenv("NO_PROXY", ciValue)

	loadProxyFromEnv()
	assert.Nil(t, Datadog.Get("proxy"))

	proxies := GetProxies()
	require.Nil(t, proxies)
}

func TestLoadProxyConfOnly(t *testing.T) {
	// check value loaded before aren't overwrite when no env variables are set
	p := &Proxy{HTTP: "test", HTTPS: "test2", NoProxy: []string{"a", "b", "c"}}
	Datadog.Set("proxy", p)
	defer Datadog.Set("proxy", nil)

	// circleCI set some proxy setting
	ciValue := os.Getenv("NO_PROXY")
	os.Unsetenv("NO_PROXY")
	defer os.Setenv("NO_PROXY", ciValue)

	loadProxyFromEnv()
	proxies := GetProxies()
	assert.Equal(t, p, proxies)
}

func TestLoadProxyStdEnvOnly(t *testing.T) {
	// uppercase
	os.Setenv("HTTP_PROXY", "http_url")
	os.Setenv("HTTPS_PROXY", "https_url")
	os.Setenv("NO_PROXY", "a,b,c") // comma-separated list

	loadProxyFromEnv()

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
	Datadog.Set("proxy", nil)

	// lowercase
	os.Setenv("http_proxy", "http_url2")
	os.Setenv("https_proxy", "https_url2")
	os.Setenv("no_proxy", "1,2,3") // comma-separated list

	loadProxyFromEnv()
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
	Datadog.Set("proxy", nil)
}

func TestLoadProxyDDSpecificEnvOnly(t *testing.T) {
	os.Setenv("DD_PROXY_HTTP", "http_url")
	os.Setenv("DD_PROXY_HTTPS", "https_url")
	os.Setenv("DD_PROXY_NO_PROXY", "a b c") // space-separated list

	loadProxyFromEnv()

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
	Datadog.Set("proxy", nil)
}

func TestLoadProxyDDSpecificEnvPrecedenceOverStdEnv(t *testing.T) {
	os.Setenv("DD_PROXY_HTTP", "dd_http_url")
	os.Setenv("DD_PROXY_HTTPS", "dd_https_url")
	os.Setenv("DD_PROXY_NO_PROXY", "a b c")
	os.Setenv("HTTP_PROXY", "env_http_url")
	os.Setenv("HTTPS_PROXY", "env_https_url")
	os.Setenv("NO_PROXY", "d,e,f")

	loadProxyFromEnv()

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
	Datadog.Set("proxy", nil)
}

func TestLoadProxyStdEnvAndConf(t *testing.T) {
	os.Setenv("HTTP_PROXY", "http_env")
	Datadog.Set("proxy.no_proxy", []string{"d", "e", "f"})
	Datadog.Set("proxy.http", "http_conf")
	defer os.Unsetenv("HTTP")
	defer Datadog.Set("proxy", nil)

	loadProxyFromEnv()
	proxies := GetProxies()
	assert.Equal(t,
		&Proxy{
			HTTP:    "http_env",
			HTTPS:   "",
			NoProxy: []string{"d", "e", "f"}},
		proxies)
}

func TestLoadProxyDDSpecificEnvAndConf(t *testing.T) {
	os.Setenv("DD_PROXY_HTTP", "http_env")
	Datadog.Set("proxy.no_proxy", []string{"d", "e", "f"})
	Datadog.Set("proxy.http", "http_conf")
	defer os.Unsetenv("DD_PROXY_HTTP")
	defer Datadog.Set("proxy", nil)

	loadProxyFromEnv()
	proxies := GetProxies()
	assert.Equal(t,
		&Proxy{
			HTTP:    "http_env",
			HTTPS:   "",
			NoProxy: []string{"d", "e", "f"}},
		proxies)
}

func TestLoadProxyEmptyValuePrecedence(t *testing.T) {
	os.Setenv("DD_PROXY_HTTP", "")
	os.Setenv("DD_PROXY_NO_PROXY", "a b c")
	os.Setenv("HTTP_PROXY", "env_http_url")
	os.Setenv("HTTPS_PROXY", "")
	os.Setenv("NO_PROXY", "")
	Datadog.Set("proxy.https", "https_conf")

	loadProxyFromEnv()

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
	Datadog.Set("proxy", nil)
}
