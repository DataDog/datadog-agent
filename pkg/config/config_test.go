// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package config

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/DataDog/datadog-agent/pkg/autodiscovery/common/types"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func unsetEnvForTest(t *testing.T, env string) {
	oldValue, ok := os.LookupEnv(env)
	os.Unsetenv(env)

	t.Cleanup(func() {
		if !ok {
			os.Unsetenv(env)
		} else {
			os.Setenv(env, oldValue)
		}
	})
}

func TestDefaults(t *testing.T) {
	config := SetupConf()

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

func TestUnexpectedUnicode(t *testing.T) {
	keyYaml := "api_\u202akey: fakeapikey\n"
	valueYaml := "api_key: fa\u202akeapikey\n"

	testConfig := SetupConfFromYAML(keyYaml)

	warnings := findUnexpectedUnicode(testConfig)
	require.Len(t, warnings, 1)

	assert.Contains(t, warnings[0], "Configuration key string")
	assert.Contains(t, warnings[0], "U+202A")

	testConfig = SetupConfFromYAML(valueYaml)

	warnings = findUnexpectedUnicode(testConfig)

	require.Len(t, warnings, 1)
	assert.Contains(t, warnings[0], "For key 'api_key'")
	assert.Contains(t, warnings[0], "U+202A")
}

func TestUnexpectedNestedUnicode(t *testing.T) {
	yaml := "runtime_security_config:\n  activity_dump:\n    remote_storage:\n      endpoints:\n        logs_dd_url: \"http://\u202adatadawg.com\""
	testConfig := SetupConfFromYAML(yaml)

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
		testConfig := SetupConfFromYAML(tc.yaml)
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
	confBase := SetupConfFromYAML(yamlBase)
	assert.Len(t, findUnknownKeys(confBase), 0)

	yamlWithUnknownKeys := `
site: datadoghq.eu
unknown_key.unknown_subkey: true
`
	confWithUnknownKeys := SetupConfFromYAML(yamlWithUnknownKeys)
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

func TestDefaultTraceManagedServicesEnvVarValue(t *testing.T) {
	testConfig := SetupConfFromYAML("")
	assert.Equal(t, true, testConfig.Get("serverless.trace_managed_services"))
}

func TestExplicitFalseTraceManagedServicesEnvVar(t *testing.T) {
	t.Setenv("DD_TRACE_MANAGED_SERVICES", "false")
	testConfig := SetupConfFromYAML("")
	assert.Equal(t, false, testConfig.Get("serverless.trace_managed_services"))
}

func TestDDHostnameFileEnvVar(t *testing.T) {
	t.Setenv("DD_API_KEY", "fakeapikey")
	t.Setenv("DD_HOSTNAME_FILE", "somefile")
	testConfig := SetupConfFromYAML("")

	assert.Equal(t, "somefile", testConfig.Get("hostname_file"))
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
	config := SetupConf()
	config.BindEnv("foo.bar.nested")
	t.Setenv("DD_FOO_BAR_NESTED", "baz")

	assert.Equal(t, "baz", config.GetString("foo.bar.nested"))
}

func TestProxy(t *testing.T) {
	type testCase struct {
		name                  string
		setup                 func(t *testing.T, config Config)
		tests                 func(t *testing.T, config Config)
		proxyForCloudMetadata bool
	}

	expectedProxy := &Proxy{
		HTTP:    "http_url",
		HTTPS:   "https_url",
		NoProxy: []string{"a", "b", "c"}}

	cases := []testCase{
		{
			name: "no values",
			tests: func(t *testing.T, config Config) {
				assert.Nil(t, config.Get("proxy"))
				assert.Nil(t, config.GetProxies())
			},
			proxyForCloudMetadata: true,
		},
		{
			name: "from configuration",
			setup: func(t *testing.T, config Config) {
				config.Set("proxy", expectedProxy)
			},
			tests: func(t *testing.T, config Config) {
				assert.Equal(t, expectedProxy, config.GetProxies())
			},
			proxyForCloudMetadata: true,
		},
		{
			name: "from UNIX env only upper case",
			setup: func(t *testing.T, config Config) {
				t.Setenv("HTTP_PROXY", "http_url")
				t.Setenv("HTTPS_PROXY", "https_url")
				t.Setenv("NO_PROXY", "a,b,c") // comma-separated list
			},
			tests: func(t *testing.T, config Config) {
				assert.Equal(t, expectedProxy, config.GetProxies())
			},
			proxyForCloudMetadata: true,
		},
		{
			name: "from env only lower case",
			setup: func(t *testing.T, config Config) {
				t.Setenv("http_proxy", "http_url")
				t.Setenv("https_proxy", "https_url")
				t.Setenv("no_proxy", "a,b,c") // comma-separated list
			},
			tests: func(t *testing.T, config Config) {
				assert.Equal(t, expectedProxy, config.GetProxies())
			},
			proxyForCloudMetadata: true,
		},
		{
			name: "from DD env vars only",
			setup: func(t *testing.T, config Config) {
				t.Setenv("DD_PROXY_HTTP", "http_url")
				t.Setenv("DD_PROXY_HTTPS", "https_url")
				t.Setenv("DD_PROXY_NO_PROXY", "a b c") // space-separated list
			},
			tests: func(t *testing.T, config Config) {
				assert.Equal(t, expectedProxy, config.GetProxies())
			},
			proxyForCloudMetadata: true,
		},
		{
			name: "from DD env vars precedence over UNIX env vars",
			setup: func(t *testing.T, config Config) {
				t.Setenv("DD_PROXY_HTTP", "dd_http_url")
				t.Setenv("DD_PROXY_HTTPS", "dd_https_url")
				t.Setenv("DD_PROXY_NO_PROXY", "a b c")
				t.Setenv("HTTP_PROXY", "env_http_url")
				t.Setenv("HTTPS_PROXY", "env_https_url")
				t.Setenv("NO_PROXY", "d,e,f")
			},
			tests: func(t *testing.T, config Config) {
				assert.Equal(t,
					&Proxy{
						HTTP:    "dd_http_url",
						HTTPS:   "dd_https_url",
						NoProxy: []string{"a", "b", "c"}},
					config.GetProxies())
			},
			proxyForCloudMetadata: true,
		},
		{
			name: "from UNIX env vars and conf",
			setup: func(t *testing.T, config Config) {
				t.Setenv("HTTP_PROXY", "http_env")
				config.Set("proxy.no_proxy", []string{"d", "e", "f"})
				config.Set("proxy.http", "http_conf")
			},
			tests: func(t *testing.T, config Config) {
				assert.Equal(t,
					&Proxy{
						HTTP:    "http_env",
						HTTPS:   "",
						NoProxy: []string{"d", "e", "f"}},
					config.GetProxies())
			},
			proxyForCloudMetadata: true,
		},
		{
			name: "from DD env vars and conf",
			setup: func(t *testing.T, config Config) {
				t.Setenv("DD_PROXY_HTTP", "http_env")
				config.Set("proxy.no_proxy", []string{"d", "e", "f"})
				config.Set("proxy.http", "http_conf")
			},
			tests: func(t *testing.T, config Config) {
				assert.Equal(t,
					&Proxy{
						HTTP:    "http_env",
						HTTPS:   "",
						NoProxy: []string{"d", "e", "f"}},
					config.GetProxies())
			},
			proxyForCloudMetadata: true,
		},
		{
			name: "empty values precedence",
			setup: func(t *testing.T, config Config) {
				t.Setenv("DD_PROXY_HTTP", "")
				t.Setenv("DD_PROXY_NO_PROXY", "a b c")
				t.Setenv("HTTP_PROXY", "env_http_url")
				t.Setenv("HTTPS_PROXY", "")
				t.Setenv("NO_PROXY", "")
				config.Set("proxy.https", "https_conf")
			},
			tests: func(t *testing.T, config Config) {
				assert.Equal(t,
					&Proxy{
						HTTP:    "",
						HTTPS:   "",
						NoProxy: []string{"a", "b", "c"}},
					config.GetProxies())
			},
			proxyForCloudMetadata: true,
		},
		{
			name: "proxy withou no_proxy",
			setup: func(t *testing.T, config Config) {
				t.Setenv("DD_PROXY_HTTP", "http_url")
				t.Setenv("DD_PROXY_HTTPS", "https_url")
			},
			tests: func(t *testing.T, config Config) {
				assert.Equal(t,
					&Proxy{
						HTTP:  "http_url",
						HTTPS: "https_url",
					},
					config.GetProxies())
				assert.Equal(t, []interface{}{}, config.Get("proxy.no_proxy"))
			},
			proxyForCloudMetadata: true,
		},
		{
			name:  "empty config with use_proxy_for_cloud_metadata",
			setup: func(t *testing.T, config Config) {},
			tests: func(t *testing.T, config Config) {
				assert.Equal(t,
					&Proxy{
						HTTP:    "",
						HTTPS:   "",
						NoProxy: []string{"169.254.169.254", "100.100.100.200"},
					},
					config.GetProxies())
			},
			proxyForCloudMetadata: false,
		},
		{
			name: "use proxy for cloud metadata",
			setup: func(t *testing.T, config Config) {
				t.Setenv("DD_PROXY_HTTP", "http_url")
				t.Setenv("DD_PROXY_HTTPS", "https_url")
				t.Setenv("DD_PROXY_NO_PROXY", "a b c")
			},
			tests: func(t *testing.T, config Config) {
				assert.Equal(t,
					&Proxy{
						HTTP:    "http_url",
						HTTPS:   "https_url",
						NoProxy: []string{"a", "b", "c", "169.254.169.254", "100.100.100.200"},
					},
					config.GetProxies())
			},
			proxyForCloudMetadata: false,
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {

			// CircleCI sets NO_PROXY, so unset it for this test
			unsetEnvForTest(t, "NO_PROXY")

			config := SetupConf()
			config.Set("use_proxy_for_cloud_metadata", c.proxyForCloudMetadata)

			// Viper.MergeConfigOverride, which is used when secrets is enabled, will silently fail if a
			// config file is never set.
			path := t.TempDir()
			configPath := filepath.Join(path, "empty_conf.yaml")
			os.WriteFile(configPath, nil, 0600)
			config.SetConfigFile(configPath)

			if c.setup != nil {
				c.setup(t, config)
			}

			_, err := LoadCustom(config, "unit_test", true, nil)
			require.NoError(t, err)

			c.tests(t, config)
		})
	}
}

func TestSanitizeAPIKeyConfig(t *testing.T) {
	config := SetupConf()

	config.Set("api_key", "foo")
	sanitizeAPIKeyConfig(config, "api_key")
	assert.Equal(t, "foo", config.GetString("api_key"))

	config.Set("api_key", "foo\n")
	sanitizeAPIKeyConfig(config, "api_key")
	assert.Equal(t, "foo", config.GetString("api_key"))

	config.Set("api_key", "foo\n\n")
	sanitizeAPIKeyConfig(config, "api_key")
	assert.Equal(t, "foo", config.GetString("api_key"))

	config.Set("api_key", " \n  foo   \n")
	sanitizeAPIKeyConfig(config, "api_key")
	assert.Equal(t, "foo", config.GetString("api_key"))
}

func TestNumWorkers(t *testing.T) {
	config := SetupConf()

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

	config := SetupConfFromYAML(datadogYaml)
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
	testConfig := SetupConfFromYAML(datadogYaml)

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

	assert.NoError(t, err)
	assert.EqualValues(t, expectedProfiles, profiles)
}

func TestDogstatsdMappingProfilesEmpty(t *testing.T) {
	datadogYaml := `
dogstatsd_mapper_profiles:
`
	testConfig := SetupConfFromYAML(datadogYaml)

	profiles, err := getDogstatsdMappingProfilesConfig(testConfig)

	var expectedProfiles []MappingProfile

	assert.NoError(t, err)
	assert.EqualValues(t, expectedProfiles, profiles)
}

func TestDogstatsdMappingProfilesError(t *testing.T) {
	datadogYaml := `
dogstatsd_mapper_profiles:
  - abc
`
	testConfig := SetupConfFromYAML(datadogYaml)
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
	config := SetupConfFromYAML(`host_aliases: ["foo", "-bar"]`)
	assert.EqualValues(t, getValidHostAliasesWithConfig(config), []string{"foo"})
}

func TestNetworkDevicesNamespace(t *testing.T) {
	datadogYaml := `
network_devices:
`
	config := SetupConfFromYAML(datadogYaml)
	assert.Equal(t, "default", config.GetString("network_devices.namespace"))

	datadogYaml = `
network_devices:
  namespace: dev
`
	config = SetupConfFromYAML(datadogYaml)
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

	config := SetupConfFromYAML(datadogYaml)
	err := checkConflictingOptions(config)

	assert.NotNil(t, err)
}

func TestUsePodmanLogs(t *testing.T) {
	// If use_podman_logs is true and docker_path_override is not set, the config should not return an error
	datadogYaml := `
logs_config:
  use_podman_logs: true
`

	config := SetupConfFromYAML(datadogYaml)
	err := checkConflictingOptions(config)

	assert.Nil(t, err)
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
	testConfig := SetupConfFromYAML(datadogYaml)
	LoadProxyFromEnv(testConfig)
	err := setupFipsEndpoints(testConfig)
	require.NoError(t, err)

	assertFipsProxyExpectedConfig(t, expectedHTTPURL, expectedURL, false, testConfig)
	assert.Equal(t, false, testConfig.GetBool("logs_config.use_http"))
	assert.Equal(t, false, testConfig.GetBool("logs_config.logs_no_ssl"))
	assert.Equal(t, false, testConfig.GetBool("runtime_security_config.endpoints.use_http"))
	assert.Equal(t, false, testConfig.GetBool("runtime_security_config.endpoints.logs_no_ssl"))
	assert.NotNil(t, testConfig.GetProxies())

	datadogYamlFips := datadogYaml + `
fips:
  enabled: true
  local_address: localhost
  port_range_start: 5000
  https: false
`

	expectedURL = "localhost:50"
	expectedHTTPURL = "http://" + expectedURL
	testConfig = SetupConfFromYAML(datadogYamlFips)
	LoadProxyFromEnv(testConfig)
	err = setupFipsEndpoints(testConfig)
	require.NoError(t, err)

	assertFipsProxyExpectedConfig(t, expectedHTTPURL, expectedURL, true, testConfig)
	assert.Equal(t, true, testConfig.GetBool("logs_config.use_http"))
	assert.Equal(t, true, testConfig.GetBool("logs_config.logs_no_ssl"))
	assert.Equal(t, true, testConfig.GetBool("runtime_security_config.endpoints.use_http"))
	assert.Equal(t, true, testConfig.GetBool("runtime_security_config.endpoints.logs_no_ssl"))
	assert.Nil(t, testConfig.GetProxies())

	datadogYamlFips = datadogYaml + `
fips:
  enabled: true
  local_address: localhost
  port_range_start: 5000
  https: true
  tls_verify: false
`

	expectedHTTPURL = "https://" + expectedURL
	testConfig = SetupConfFromYAML(datadogYamlFips)
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
	assert.Nil(t, testConfig.GetProxies())

	testConfig.Set("skip_ssl_validation", true) // should be overridden by fips.tls_verify
	testConfig.Set("fips.tls_verify", true)
	LoadProxyFromEnv(testConfig)
	err = setupFipsEndpoints(testConfig)
	require.NoError(t, err)

	assert.Equal(t, false, testConfig.GetBool("skip_ssl_validation"))
	assert.Nil(t, testConfig.GetProxies())
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

	testConfig := SetupConfFromYAML(datadogYaml)
	err := setupFipsEndpoints(testConfig)
	require.Error(t, err)
}

func TestEnablePeerServiceStatsAggregationYAML(t *testing.T) {
	datadogYaml := `
apm_config:
  peer_service_aggregation: true
`
	testConfig := SetupConfFromYAML(datadogYaml)
	err := setupFipsEndpoints(testConfig)
	require.NoError(t, err)
	require.True(t, testConfig.GetBool("apm_config.peer_service_aggregation"))

	datadogYaml = `
apm_config:
  peer_service_aggregation: false
`
	testConfig = SetupConfFromYAML(datadogYaml)
	err = setupFipsEndpoints(testConfig)
	require.NoError(t, err)
	require.False(t, testConfig.GetBool("apm_config.peer_service_aggregation"))
}

func TestEnablePeerServiceStatsAggregationEnv(t *testing.T) {
	t.Setenv("DD_APM_PEER_SERVICE_AGGREGATION", "true")
	testConfig := SetupConfFromYAML("")
	require.True(t, testConfig.GetBool("apm_config.peer_service_aggregation"))
	t.Setenv("DD_APM_PEER_SERVICE_AGGREGATION", "false")
	testConfig = SetupConfFromYAML("")
	require.False(t, testConfig.GetBool("apm_config.peer_service_aggregation"))
}

func TestEnableStatsComputationBySpanKindYAML(t *testing.T) {
	datadogYaml := `
apm_config:
  compute_stats_by_span_kind: false
`
	testConfig := SetupConfFromYAML(datadogYaml)
	err := setupFipsEndpoints(testConfig)
	require.NoError(t, err)
	require.False(t, testConfig.GetBool("apm_config.compute_stats_by_span_kind"))

	datadogYaml = `
apm_config:
  compute_stats_by_span_kind: true
`
	testConfig = SetupConfFromYAML(datadogYaml)
	err = setupFipsEndpoints(testConfig)
	require.NoError(t, err)
	require.True(t, testConfig.GetBool("apm_config.compute_stats_by_span_kind"))

}

func TestComputeStatsBySpanKindEnv(t *testing.T) {
	t.Setenv("DD_APM_COMPUTE_STATS_BY_SPAN_KIND", "false")
	testConfig := SetupConfFromYAML("")
	require.False(t, testConfig.GetBool("apm_config.compute_stats_by_span_kind"))
	t.Setenv("DD_APM_COMPUTE_STATS_BY_SPAN_KIND", "true")
	testConfig = SetupConfFromYAML("")
	require.True(t, testConfig.GetBool("apm_config.compute_stats_by_span_kind"))
}

func TestIsRemoteConfigEnabled(t *testing.T) {
	t.Setenv("DD_REMOTE_CONFIGURATION_ENABLED", "true")
	testConfig := SetupConfFromYAML("")
	require.True(t, IsRemoteConfigEnabled(testConfig))

	t.Setenv("DD_FIPS_ENABLED", "true")
	testConfig = SetupConfFromYAML("")
	require.False(t, IsRemoteConfigEnabled(testConfig))

	t.Setenv("DD_FIPS_ENABLED", "false")
	t.Setenv("DD_SITE", "ddog-gov.com")
	testConfig = SetupConfFromYAML("")
	require.False(t, IsRemoteConfigEnabled(testConfig))
}

func TestLanguageDetectionSettings(t *testing.T) {
	testConfig := SetupConfFromYAML("")
	require.False(t, testConfig.GetBool("language_detection.enabled"))

	t.Setenv("DD_LANGUAGE_DETECTION_ENABLED", "true")
	testConfig = SetupConfFromYAML("")
	require.True(t, testConfig.GetBool("language_detection.enabled"))
}
