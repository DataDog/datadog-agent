package config

import (
	"bytes"
	"path"
	"runtime"
	"testing"

	"github.com/spf13/viper"
	"github.com/stretchr/testify/assert"
)

func TestDefaults(t *testing.T) {
	assert.Equal(t, Datadog.GetString("dd_url"), "http://localhost:17123")
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
		"https://app.datadoghq.com": {
			"fakeapikey",
			"fakeapikey2",
			"fakeapikey3",
		},
		"https://foo.datadoghq.com": {
			"someapikey",
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

	testConfig := setupViperConf(datadogYaml)

	multipleEndpoints, err := getMultipleEndpoints(testConfig)

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

	testConfig := setupViperConf(datadogYaml)

	multipleEndpoints, err := getMultipleEndpoints(testConfig)

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

func TestLegacyIniConfig(t *testing.T) {
	origCfgPath := Datadog.GetString("conf_path")
	defer Datadog.Set("conf_path", origCfgPath)

	_, f, _, _ := runtime.Caller(0)
	cfgIni := path.Join(path.Dir(f), "test", "datadog.conf.example")

	Datadog.Set("conf_path", cfgIni)
	err := ReadLegacyConfig()

	assert.Nil(t, err)

	assert.EqualValues(t, Datadog.GetString("hostname"), "foo.bar")
	assert.EqualValues(t, Datadog.GetString("api_key"), "deadbeeffeebdaeddeadbeeffeebdaed")
	assert.EqualValues(t, Datadog.GetBool("use_dogstatsd"), true)
	assert.EqualValues(t, Datadog.GetInt64("dogstatsd_port"), 8126)
	assert.EqualValues(t, Datadog.GetInt64("dogstatsd_buffer_size"), 16384)
	assert.EqualValues(t, Datadog.GetBool("dogstatsd_stats_enable"), true)
}
