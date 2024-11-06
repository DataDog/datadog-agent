// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package setup

import (
	"bytes"
	"fmt"
	"os"
	"path"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/fx"
	"gopkg.in/yaml.v2"

	"github.com/DataDog/datadog-agent/comp/core/secrets"
	"github.com/DataDog/datadog-agent/comp/core/secrets/secretsimpl"
	nooptelemetry "github.com/DataDog/datadog-agent/comp/core/telemetry/noopsimpl"
	pkgconfigmodel "github.com/DataDog/datadog-agent/pkg/config/model"
	"github.com/DataDog/datadog-agent/pkg/util/fxutil"
	"github.com/DataDog/datadog-agent/pkg/util/optional"
	"github.com/DataDog/datadog-agent/pkg/util/scrubber"
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

func confFromYAML(t *testing.T, yamlConfig string) pkgconfigmodel.Config {
	conf := newTestConf()
	conf.SetConfigType("yaml")
	err := conf.ReadConfig(bytes.NewBuffer([]byte(yamlConfig)))
	require.NoError(t, err)
	return conf
}

func TestDefaults(t *testing.T) {
	config := newTestConf()

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

	testConfig := confFromYAML(t, keyYaml)

	warnings := findUnexpectedUnicode(testConfig)
	require.Len(t, warnings, 1)

	assert.Contains(t, warnings[0], "Configuration key string")
	assert.Contains(t, warnings[0], "U+202A")

	testConfig = confFromYAML(t, valueYaml)

	warnings = findUnexpectedUnicode(testConfig)

	require.Len(t, warnings, 1)
	assert.Contains(t, warnings[0], "For key 'api_key'")
	assert.Contains(t, warnings[0], "U+202A")
}

func TestUnexpectedNestedUnicode(t *testing.T) {
	yaml := "runtime_security_config:\n  activity_dump:\n    remote_storage:\n      endpoints:\n        logs_dd_url: \"http://\u202adatadawg.com\""
	testConfig := confFromYAML(t, yaml)

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
		testConfig := confFromYAML(t, tc.yaml)
		warnings := findUnexpectedUnicode(testConfig)
		require.Len(t, warnings, 1)

		assert.Contains(t, warnings[0], tc.expectedPosition)
		assert.Contains(t, warnings[0], tc.expectedPosition)
	}
}

func TestUnknownKeysWarning(t *testing.T) {
	conf := newTestConf()
	conf.SetWithoutSource("site", "datadoghq.eu")
	assert.Len(t, findUnknownKeys(conf), 0)

	conf.SetWithoutSource("unknown_key.unknown_subkey", "true")
	assert.Len(t, findUnknownKeys(conf), 1)

	conf.SetKnown("unknown_key.*")
	assert.Len(t, findUnknownKeys(conf), 0)
}

func TestUnknownVarsWarning(t *testing.T) {
	test := func(v string, unknown bool, additional []string) func(*testing.T) {
		return func(t *testing.T) {
			env := []string{fmt.Sprintf("%s=foo", v)}
			var exp []string
			if unknown {
				exp = append(exp, v)
			}
			assert.Equal(t, exp, findUnknownEnvVars(newTestConf(), env, additional))
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
	testConfig := newTestConf()
	assert.Equal(t, true, testConfig.Get("serverless.trace_managed_services"))
}

func TestExplicitFalseTraceManagedServicesEnvVar(t *testing.T) {
	t.Setenv("DD_TRACE_MANAGED_SERVICES", "false")
	testConfig := newTestConf()
	assert.Equal(t, false, testConfig.Get("serverless.trace_managed_services"))
}

func TestDDHostnameFileEnvVar(t *testing.T) {
	t.Setenv("DD_API_KEY", "fakeapikey")
	t.Setenv("DD_HOSTNAME_FILE", "somefile")
	testConfig := newTestConf()

	assert.Equal(t, "somefile", testConfig.Get("hostname_file"))
}

func TestIsCloudProviderEnabled(t *testing.T) {
	config := newTestConf()

	config.SetWithoutSource("cloud_provider_metadata", []string{"aws", "gcp", "azure", "alibaba", "tencent"})
	assert.True(t, IsCloudProviderEnabled("AWS", config))
	assert.True(t, IsCloudProviderEnabled("GCP", config))
	assert.True(t, IsCloudProviderEnabled("Alibaba", config))
	assert.True(t, IsCloudProviderEnabled("Azure", config))
	assert.True(t, IsCloudProviderEnabled("Tencent", config))

	config.SetWithoutSource("cloud_provider_metadata", []string{"aws"})
	assert.True(t, IsCloudProviderEnabled("AWS", config))
	assert.False(t, IsCloudProviderEnabled("GCP", config))
	assert.False(t, IsCloudProviderEnabled("Alibaba", config))
	assert.False(t, IsCloudProviderEnabled("Azure", config))
	assert.False(t, IsCloudProviderEnabled("Tencent", config))

	config.SetWithoutSource("cloud_provider_metadata", []string{"tencent"})
	assert.False(t, IsCloudProviderEnabled("AWS", config))
	assert.False(t, IsCloudProviderEnabled("GCP", config))
	assert.False(t, IsCloudProviderEnabled("Alibaba", config))
	assert.False(t, IsCloudProviderEnabled("Azure", config))
	assert.True(t, IsCloudProviderEnabled("Tencent", config))

	config.SetWithoutSource("cloud_provider_metadata", []string{})
	assert.False(t, IsCloudProviderEnabled("AWS", config))
	assert.False(t, IsCloudProviderEnabled("GCP", config))
	assert.False(t, IsCloudProviderEnabled("Alibaba", config))
	assert.False(t, IsCloudProviderEnabled("Azure", config))
	assert.False(t, IsCloudProviderEnabled("Tencent", config))
}

func TestEnvNestedConfig(t *testing.T) {
	config := newTestConf()
	config.BindEnv("foo.bar.nested")
	t.Setenv("DD_FOO_BAR_NESTED", "baz")

	assert.Equal(t, "baz", config.GetString("foo.bar.nested"))
}

func TestProxy(t *testing.T) {
	type testCase struct {
		name                  string
		setup                 func(t *testing.T, config pkgconfigmodel.Config)
		tests                 func(t *testing.T, config pkgconfigmodel.Config)
		proxyForCloudMetadata bool
	}

	expectedProxy := &pkgconfigmodel.Proxy{
		HTTP:    "http_url",
		HTTPS:   "https_url",
		NoProxy: []string{"a", "b", "c"},
	}

	cases := []testCase{
		{
			name: "no values",
			tests: func(t *testing.T, config pkgconfigmodel.Config) {
				assert.Nil(t, config.Get("proxy"))
				assert.Nil(t, config.GetProxies())
			},
			proxyForCloudMetadata: true,
		},
		{
			name: "from configuration",
			setup: func(_ *testing.T, config pkgconfigmodel.Config) {
				config.SetWithoutSource("proxy", expectedProxy)
			},
			tests: func(t *testing.T, config pkgconfigmodel.Config) {
				assert.Equal(t, expectedProxy, config.GetProxies())
			},
			proxyForCloudMetadata: true,
		},
		{
			name: "from UNIX env only upper case",
			setup: func(t *testing.T, _ pkgconfigmodel.Config) {
				t.Setenv("HTTP_PROXY", "http_url")
				t.Setenv("HTTPS_PROXY", "https_url")
				t.Setenv("NO_PROXY", "a,b,c") // comma-separated list
			},
			tests: func(t *testing.T, config pkgconfigmodel.Config) {
				assert.Equal(t, expectedProxy, config.GetProxies())
			},
			proxyForCloudMetadata: true,
		},
		{
			name: "from env only lower case",
			setup: func(t *testing.T, _ pkgconfigmodel.Config) {
				t.Setenv("http_proxy", "http_url")
				t.Setenv("https_proxy", "https_url")
				t.Setenv("no_proxy", "a,b,c") // comma-separated list
			},
			tests: func(t *testing.T, config pkgconfigmodel.Config) {
				assert.Equal(t, expectedProxy, config.GetProxies())
			},
			proxyForCloudMetadata: true,
		},
		{
			name: "from DD env vars only",
			setup: func(t *testing.T, _ pkgconfigmodel.Config) {
				t.Setenv("DD_PROXY_HTTP", "http_url")
				t.Setenv("DD_PROXY_HTTPS", "https_url")
				t.Setenv("DD_PROXY_NO_PROXY", "a b c") // space-separated list
			},
			tests: func(t *testing.T, config pkgconfigmodel.Config) {
				assert.Equal(t, expectedProxy, config.GetProxies())
			},
			proxyForCloudMetadata: true,
		},
		{
			name: "from DD env vars precedence over UNIX env vars",
			setup: func(t *testing.T, _ pkgconfigmodel.Config) {
				t.Setenv("DD_PROXY_HTTP", "dd_http_url")
				t.Setenv("DD_PROXY_HTTPS", "dd_https_url")
				t.Setenv("DD_PROXY_NO_PROXY", "a b c")
				t.Setenv("HTTP_PROXY", "env_http_url")
				t.Setenv("HTTPS_PROXY", "env_https_url")
				t.Setenv("NO_PROXY", "d,e,f")
			},
			tests: func(t *testing.T, config pkgconfigmodel.Config) {
				assert.Equal(t,
					&pkgconfigmodel.Proxy{
						HTTP:    "dd_http_url",
						HTTPS:   "dd_https_url",
						NoProxy: []string{"a", "b", "c"},
					},
					config.GetProxies())
			},
			proxyForCloudMetadata: true,
		},
		{
			name: "from UNIX env vars and conf",
			setup: func(t *testing.T, config pkgconfigmodel.Config) {
				t.Setenv("HTTP_PROXY", "http_env")
				config.SetWithoutSource("proxy.no_proxy", []string{"d", "e", "f"})
				config.SetWithoutSource("proxy.http", "http_conf")
			},
			tests: func(t *testing.T, config pkgconfigmodel.Config) {
				assert.Equal(t,
					&pkgconfigmodel.Proxy{
						HTTP:    "http_env",
						HTTPS:   "",
						NoProxy: []string{"d", "e", "f"},
					},
					config.GetProxies())
			},
			proxyForCloudMetadata: true,
		},
		{
			name: "from DD env vars and conf",
			setup: func(t *testing.T, config pkgconfigmodel.Config) {
				t.Setenv("DD_PROXY_HTTP", "http_env")
				config.SetWithoutSource("proxy.no_proxy", []string{"d", "e", "f"})
				config.SetWithoutSource("proxy.http", "http_conf")
			},
			tests: func(t *testing.T, config pkgconfigmodel.Config) {
				assert.Equal(t,
					&pkgconfigmodel.Proxy{
						HTTP:    "http_env",
						HTTPS:   "",
						NoProxy: []string{"d", "e", "f"},
					},
					config.GetProxies())
			},
			proxyForCloudMetadata: true,
		},
		{
			name: "empty values precedence",
			setup: func(t *testing.T, config pkgconfigmodel.Config) {
				t.Setenv("DD_PROXY_HTTP", "")
				t.Setenv("DD_PROXY_NO_PROXY", "a b c")
				t.Setenv("HTTP_PROXY", "env_http_url")
				t.Setenv("HTTPS_PROXY", "")
				t.Setenv("NO_PROXY", "")
				config.SetWithoutSource("proxy.https", "https_conf")
			},
			tests: func(t *testing.T, config pkgconfigmodel.Config) {
				assert.Equal(t,
					&pkgconfigmodel.Proxy{
						HTTP:    "",
						HTTPS:   "",
						NoProxy: []string{"a", "b", "c"},
					},
					config.GetProxies())
			},
			proxyForCloudMetadata: true,
		},
		{
			name: "proxy withou no_proxy",
			setup: func(t *testing.T, _ pkgconfigmodel.Config) {
				t.Setenv("DD_PROXY_HTTP", "http_url")
				t.Setenv("DD_PROXY_HTTPS", "https_url")
			},
			tests: func(t *testing.T, config pkgconfigmodel.Config) {
				assert.Equal(t,
					&pkgconfigmodel.Proxy{
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
			setup: func(*testing.T, pkgconfigmodel.Config) {},
			tests: func(t *testing.T, config pkgconfigmodel.Config) {
				assert.Equal(t,
					&pkgconfigmodel.Proxy{
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
			setup: func(t *testing.T, _ pkgconfigmodel.Config) {
				t.Setenv("DD_PROXY_HTTP", "http_url")
				t.Setenv("DD_PROXY_HTTPS", "https_url")
				t.Setenv("DD_PROXY_NO_PROXY", "a b c")
			},
			tests: func(t *testing.T, config pkgconfigmodel.Config) {
				assert.Equal(t,
					&pkgconfigmodel.Proxy{
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

			config := newTestConf()
			config.SetWithoutSource("use_proxy_for_cloud_metadata", c.proxyForCloudMetadata)

			path := t.TempDir()
			configPath := filepath.Join(path, "empty_conf.yaml")
			os.WriteFile(configPath, nil, 0o600)
			config.SetConfigFile(configPath)

			resolver := fxutil.Test[secrets.Component](t, fx.Options(
				secretsimpl.MockModule(),
				nooptelemetry.Module(),
			))
			if c.setup != nil {
				c.setup(t, config)
			}

			_, err := LoadDatadogCustom(config, "unit_test", optional.NewOption[secrets.Component](resolver), nil)
			require.NoError(t, err)

			c.tests(t, config)
		})
	}
}

func TestDatabaseMonitoringAurora(t *testing.T) {
	testCases := []struct {
		name  string
		setup func(t *testing.T, config pkgconfigmodel.Config)
		tests func(t *testing.T, config pkgconfigmodel.Config)
	}{
		{
			name:  "auto discovery is disabled by default",
			setup: func(_ *testing.T, _ pkgconfigmodel.Config) {},
			tests: func(t *testing.T, config pkgconfigmodel.Config) {
				assert.False(t, config.GetBool("database_monitoring.autodiscovery.aurora.enabled"))
			},
		},
		{
			name: "default auto discovery configuration is enabled from DD env vars",
			setup: func(t *testing.T, _ pkgconfigmodel.Config) {
				t.Setenv("DD_DATABASE_MONITORING_AUTODISCOVERY_AURORA_ENABLED", "true")
			},
			tests: func(t *testing.T, config pkgconfigmodel.Config) {
				assert.True(t, config.GetBool("database_monitoring.autodiscovery.aurora.enabled"))
				assert.Equal(t, config.GetInt("database_monitoring.autodiscovery.aurora.discovery_interval"), 300)
				assert.Equal(t, config.GetInt("database_monitoring.autodiscovery.aurora.query_timeout"), 10)
				assert.Equal(t, config.Get("database_monitoring.autodiscovery.aurora.tags"), []string{"datadoghq.com/scrape:true"})
				assert.Equal(t, config.GetString("database_monitoring.autodiscovery.aurora.region"), "")
			},
		},
		{
			name: "auto discovery query timeout, region and discovery interval are set from DD env vars",
			setup: func(t *testing.T, _ pkgconfigmodel.Config) {
				t.Setenv("DD_DATABASE_MONITORING_AUTODISCOVERY_AURORA_ENABLED", "true")
				t.Setenv("DD_DATABASE_MONITORING_AUTODISCOVERY_AURORA_DISCOVERY_INTERVAL", "15")
				t.Setenv("DD_DATABASE_MONITORING_AUTODISCOVERY_AURORA_QUERY_TIMEOUT", "1")
				t.Setenv("DD_DATABASE_MONITORING_AUTODISCOVERY_AURORA_REGION", "us-west-2")
			},
			tests: func(t *testing.T, config pkgconfigmodel.Config) {
				assert.True(t, config.GetBool("database_monitoring.autodiscovery.aurora.enabled"))
				assert.Equal(t, config.GetInt("database_monitoring.autodiscovery.aurora.discovery_interval"), 15)
				assert.Equal(t, config.GetInt("database_monitoring.autodiscovery.aurora.query_timeout"), 1)
				assert.Equal(t, config.GetString("database_monitoring.autodiscovery.aurora.region"), "us-west-2")
				assert.Equal(t, config.Get("database_monitoring.autodiscovery.aurora.tags"), []string{"datadoghq.com/scrape:true"})
			},
		},
		{
			name: "auto discovery tag configuration set through DD env vars",
			setup: func(t *testing.T, _ pkgconfigmodel.Config) {
				t.Setenv("DD_DATABASE_MONITORING_AUTODISCOVERY_AURORA_ENABLED", "true")
				t.Setenv("DD_DATABASE_MONITORING_AUTODISCOVERY_AURORA_TAGS", "foo:bar other:tag")
			},
			tests: func(t *testing.T, config pkgconfigmodel.Config) {
				assert.True(t, config.GetBool("database_monitoring.autodiscovery.aurora.enabled"))
				assert.Equal(t, config.GetInt("database_monitoring.autodiscovery.aurora.discovery_interval"), 300)
				assert.Equal(t, config.GetInt("database_monitoring.autodiscovery.aurora.query_timeout"), 10)
				assert.Equal(t, config.Get("database_monitoring.autodiscovery.aurora.tags"), []string{"foo:bar", "other:tag"})
			},
		},
		{
			name: "default auto discovery is enabled from configuration",
			setup: func(_ *testing.T, config pkgconfigmodel.Config) {
				config.SetWithoutSource("database_monitoring.autodiscovery.aurora.enabled", true)
			},
			tests: func(t *testing.T, config pkgconfigmodel.Config) {
				assert.True(t, config.GetBool("database_monitoring.autodiscovery.aurora.enabled"))
				assert.Equal(t, config.GetInt("database_monitoring.autodiscovery.aurora.discovery_interval"), 300)
				assert.Equal(t, config.GetInt("database_monitoring.autodiscovery.aurora.query_timeout"), 10)
				assert.Equal(t, config.Get("database_monitoring.autodiscovery.aurora.tags"), []string{"datadoghq.com/scrape:true"})
			},
		},
		{
			name: "auto discovery interval and tags are set from configuration",
			setup: func(_ *testing.T, config pkgconfigmodel.Config) {
				config.SetWithoutSource("database_monitoring.autodiscovery.aurora.enabled", true)
				config.SetWithoutSource("database_monitoring.autodiscovery.aurora.discovery_interval", 10)
				config.SetWithoutSource("database_monitoring.autodiscovery.aurora.query_timeout", 4)
				config.SetWithoutSource("database_monitoring.autodiscovery.aurora.tags", []string{"foo:bar"})
			},
			tests: func(t *testing.T, config pkgconfigmodel.Config) {
				assert.True(t, config.GetBool("database_monitoring.autodiscovery.aurora.enabled"))
				assert.Equal(t, config.GetInt("database_monitoring.autodiscovery.aurora.discovery_interval"), 10)
				assert.Equal(t, config.GetInt("database_monitoring.autodiscovery.aurora.query_timeout"), 4)
				assert.Equal(t, config.Get("database_monitoring.autodiscovery.aurora.tags"), []string{"foo:bar"})
			},
		},
	}
	for _, c := range testCases {
		t.Run(c.name, func(t *testing.T) {
			config := newTestConf()

			path := t.TempDir()
			configPath := filepath.Join(path, "empty_conf.yaml")
			os.WriteFile(configPath, nil, 0o600)
			config.SetConfigFile(configPath)

			resolver := fxutil.Test[secrets.Component](t, fx.Options(
				secretsimpl.MockModule(),
				nooptelemetry.Module(),
			))
			if c.setup != nil {
				c.setup(t, config)
			}

			_, err := LoadDatadogCustom(config, "unit_test", optional.NewOption[secrets.Component](resolver), nil)
			require.NoError(t, err)

			c.tests(t, config)
		})
	}
}

func TestSanitizeAPIKeyConfig(t *testing.T) {
	config := newTestConf()

	config.SetWithoutSource("api_key", "foo")
	sanitizeAPIKeyConfig(config, "api_key")
	assert.Equal(t, "foo", config.GetString("api_key"))

	config.SetWithoutSource("api_key", "foo\n")
	sanitizeAPIKeyConfig(config, "api_key")
	assert.Equal(t, "foo", config.GetString("api_key"))

	config.SetWithoutSource("api_key", "foo\n\n")
	sanitizeAPIKeyConfig(config, "api_key")
	assert.Equal(t, "foo", config.GetString("api_key"))

	config.SetWithoutSource("api_key", " \n  foo   \n")
	sanitizeAPIKeyConfig(config, "api_key")
	assert.Equal(t, "foo", config.GetString("api_key"))
}

func TestNumWorkers(t *testing.T) {
	config := newTestConf()

	config.SetWithoutSource("python_version", "2")
	config.SetWithoutSource("tracemalloc_debug", true)
	config.SetWithoutSource("check_runners", 4)

	setNumWorkers(config)
	workers := config.GetInt("check_runners")
	assert.Equal(t, workers, config.GetInt("check_runners"))

	config.SetWithoutSource("tracemalloc_debug", false)
	setNumWorkers(config)
	workers = config.GetInt("check_runners")
	assert.Equal(t, workers, config.GetInt("check_runners"))

	config.SetWithoutSource("python_version", "3")
	setNumWorkers(config)
	workers = config.GetInt("check_runners")
	assert.Equal(t, workers, config.GetInt("check_runners"))

	config.SetWithoutSource("tracemalloc_debug", true)
	setNumWorkers(config)
	workers = config.GetInt("check_runners")
	assert.Equal(t, workers, 1)
}

// TestOverrides validates that the config overrides system works well.
func TestApplyOverrides(t *testing.T) {
	pkgconfigmodel.CleanOverride(t)
	assert := assert.New(t)

	datadogYaml := `
dd_url: "https://app.datadoghq.eu"
api_key: fakeapikey

external_config:
  external_agent_dd_url: "https://custom.external-agent.datadoghq.eu"
`
	pkgconfigmodel.AddOverrides(map[string]interface{}{
		"api_key": "overrided",
	})

	config := confFromYAML(t, datadogYaml)
	pkgconfigmodel.ApplyOverrideFuncs(config)

	assert.Equal(config.GetString("api_key"), "overrided", "the api key should have been overrided")
	assert.Equal(config.GetString("dd_url"), "https://app.datadoghq.eu", "this shouldn't be overrided")

	pkgconfigmodel.AddOverrides(map[string]interface{}{
		"dd_url": "http://localhost",
	})
	pkgconfigmodel.ApplyOverrideFuncs(config)

	assert.Equal(config.GetString("api_key"), "overrided", "the api key should have been overrided")
	assert.Equal(config.GetString("dd_url"), "http://localhost", "this dd_url should have been overrided")
}

func TestGetValidHostAliasesWithConfig(t *testing.T) {
	config := newTestConf()
	config.SetWithoutSource("host_aliases", []string{"foo", "-bar"})
	assert.EqualValues(t, getValidHostAliasesWithConfig(config), []string{"foo"})
}

func TestNetworkDevicesNamespace(t *testing.T) {
	datadogYaml := `
network_devices:
`
	config := confFromYAML(t, datadogYaml)
	assert.Equal(t, "default", config.GetString("network_devices.namespace"))

	datadogYaml = `
network_devices:
  namespace: dev
`
	config = confFromYAML(t, datadogYaml)
	assert.Equal(t, "dev", config.GetString("network_devices.namespace"))
}

func TestNetworkPathDefaults(t *testing.T) {
	datadogYaml := ""
	config := confFromYAML(t, datadogYaml)

	assert.Equal(t, false, config.GetBool("network_path.connections_monitoring.enabled"))
	assert.Equal(t, 4, config.GetInt("network_path.collector.workers"))
	assert.Equal(t, 1000, config.GetInt("network_path.collector.timeout"))
	assert.Equal(t, 30, config.GetInt("network_path.collector.max_ttl"))
	assert.Equal(t, 100000, config.GetInt("network_path.collector.input_chan_size"))
	assert.Equal(t, 100000, config.GetInt("network_path.collector.processing_chan_size"))
	assert.Equal(t, 100000, config.GetInt("network_path.collector.pathtest_contexts_limit"))
	assert.Equal(t, 15*time.Minute, config.GetDuration("network_path.collector.pathtest_ttl"))
	assert.Equal(t, 5*time.Minute, config.GetDuration("network_path.collector.pathtest_interval"))
	assert.Equal(t, 10*time.Second, config.GetDuration("network_path.collector.flush_interval"))
	assert.Equal(t, true, config.GetBool("network_path.collector.reverse_dns_enrichment.enabled"))
	assert.Equal(t, 5000, config.GetInt("network_path.collector.reverse_dns_enrichment.timeout"))
}

func TestUsePodmanLogsAndDockerPathOverride(t *testing.T) {
	// If use_podman_logs is true and docker_path_override is set, the config should return an error
	datadogYaml := `
logs_config:
  use_podman_logs: true
  docker_path_override: "/custom/path"
`

	config := confFromYAML(t, datadogYaml)
	err := checkConflictingOptions(config)

	assert.NotNil(t, err)
}

func TestUsePodmanLogs(t *testing.T) {
	// If use_podman_logs is true and docker_path_override is not set, the config should not return an error
	datadogYaml := `
logs_config:
  use_podman_logs: true
`

	config := confFromYAML(t, datadogYaml)
	err := checkConflictingOptions(config)

	assert.NoError(t, err)
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
    logs_dd_url: somehost:1234
  activity:
    logs_dd_url: somehost:1234
  samples:
    logs_dd_url: somehost:1234

network_devices:
  metadata:
    logs_dd_url: somehost:1234
  snmp_traps:
    forwarder:
      logs_dd_url: somehost:1234
  netflow:
    forwarder:
      logs_dd_url: somehost:1234

orchestrator_explorer:
    orchestrator_dd_url: https://somehost:1234

runtime_security_config:
    endpoints:
        logs_dd_url: somehost:1234

compliance_config:
    endpoints:
        logs_dd_url: somehost:1234

proxy:
  http: http://localhost:1234
  https: https://localhost:1234
`
	expectedURL := "somehost:1234"
	expectedHTTPURL := "https://" + expectedURL
	testConfig := confFromYAML(t, datadogYaml)
	LoadProxyFromEnv(testConfig)
	err := setupFipsEndpoints(testConfig)
	require.NoError(t, err)

	assertFipsProxyExpectedConfig(t, expectedHTTPURL, expectedURL, false, testConfig)
	assert.Equal(t, false, testConfig.GetBool("logs_config.use_http"))
	assert.Equal(t, false, testConfig.GetBool("logs_config.logs_no_ssl"))
	assert.Equal(t, false, testConfig.GetBool("runtime_security_config.endpoints.use_http"))
	assert.Equal(t, false, testConfig.GetBool("runtime_security_config.endpoints.logs_no_ssl"))
	assert.Equal(t, false, testConfig.GetBool("compliance_config.endpoints.use_http"))
	assert.Equal(t, false, testConfig.GetBool("compliance_config.endpoints.logs_no_ssl"))
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
	testConfig = confFromYAML(t, datadogYamlFips)
	LoadProxyFromEnv(testConfig)
	err = setupFipsEndpoints(testConfig)
	require.NoError(t, err)

	assertFipsProxyExpectedConfig(t, expectedHTTPURL, expectedURL, true, testConfig)
	assert.Equal(t, true, testConfig.GetBool("logs_config.use_http"))
	assert.Equal(t, true, testConfig.GetBool("logs_config.logs_no_ssl"))
	assert.Equal(t, true, testConfig.GetBool("runtime_security_config.endpoints.use_http"))
	assert.Equal(t, true, testConfig.GetBool("runtime_security_config.endpoints.logs_no_ssl"))
	assert.Equal(t, true, testConfig.GetBool("compliance_config.endpoints.use_http"))
	assert.Equal(t, true, testConfig.GetBool("compliance_config.endpoints.logs_no_ssl"))
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
	testConfig = confFromYAML(t, datadogYamlFips)
	testConfig.Set("skip_ssl_validation", false, pkgconfigmodel.SourceAgentRuntime) // should be overridden by fips.tls_verify
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

	testConfig.Set("skip_ssl_validation", true, pkgconfigmodel.SourceAgentRuntime) // should be overridden by fips.tls_verify
	testConfig.Set("fips.tls_verify", true, pkgconfigmodel.SourceAgentRuntime)
	LoadProxyFromEnv(testConfig)
	err = setupFipsEndpoints(testConfig)
	require.NoError(t, err)

	assert.Equal(t, false, testConfig.GetBool("skip_ssl_validation"))
	assert.Nil(t, testConfig.GetProxies())
}

func assertFipsProxyExpectedConfig(t *testing.T, expectedBaseHTTPURL, expectedBaseURL string, rng bool, c pkgconfigmodel.Config) {
	if rng {
		assert.Equal(t, expectedBaseHTTPURL+"01", c.GetString("dd_url"))
		assert.Equal(t, expectedBaseHTTPURL+"02", c.GetString("apm_config.apm_dd_url"))
		assert.Equal(t, expectedBaseHTTPURL+"03"+"/api/v2/profile", c.GetString("apm_config.profiling_dd_url"))
		assert.Equal(t, expectedBaseHTTPURL+"10", c.GetString("apm_config.telemetry.dd_url"))
		assert.Equal(t, expectedBaseHTTPURL+"04", c.GetString("process_config.process_dd_url"))
		assert.Equal(t, expectedBaseURL+"05", c.GetString("logs_config.logs_dd_url"))
		assert.Equal(t, expectedBaseURL+"06", c.GetString("database_monitoring.metrics.logs_dd_url"))
		assert.Equal(t, expectedBaseURL+"06", c.GetString("database_monitoring.activity.logs_dd_url"))
		assert.Equal(t, expectedBaseURL+"07", c.GetString("database_monitoring.samples.logs_dd_url"))
		assert.Equal(t, expectedBaseURL+"08", c.GetString("network_devices.metadata.logs_dd_url"))
		assert.Equal(t, expectedBaseURL+"09", c.GetString("network_devices.snmp_traps.forwarder.logs_dd_url"))
		assert.Equal(t, expectedBaseHTTPURL+"12", c.GetString("orchestrator_explorer.orchestrator_dd_url"))
		assert.Equal(t, expectedBaseURL+"13", c.GetString("runtime_security_config.endpoints.logs_dd_url"))
		assert.Equal(t, expectedBaseURL+"14", c.GetString("compliance_config.endpoints.logs_dd_url"))
		assert.Equal(t, expectedBaseURL+"15", c.GetString("network_devices.netflow.forwarder.logs_dd_url"))

	} else {
		assert.Equal(t, expectedBaseHTTPURL, c.GetString("dd_url"))
		assert.Equal(t, expectedBaseHTTPURL, c.GetString("apm_config.apm_dd_url"))
		assert.Equal(t, expectedBaseHTTPURL, c.GetString("apm_config.profiling_dd_url")) // Omitting "/api/v2/profile" as the config is not overwritten
		assert.Equal(t, expectedBaseHTTPURL, c.GetString("apm_config.telemetry.dd_url"))
		assert.Equal(t, expectedBaseHTTPURL, c.GetString("process_config.process_dd_url"))
		assert.Equal(t, expectedBaseURL, c.GetString("logs_config.logs_dd_url"))
		assert.Equal(t, expectedBaseURL, c.GetString("database_monitoring.metrics.logs_dd_url"))
		assert.Equal(t, expectedBaseURL, c.GetString("database_monitoring.activity.logs_dd_url"))
		assert.Equal(t, expectedBaseURL, c.GetString("database_monitoring.samples.logs_dd_url"))
		assert.Equal(t, expectedBaseURL, c.GetString("network_devices.metadata.logs_dd_url"))
		assert.Equal(t, expectedBaseURL, c.GetString("network_devices.snmp_traps.forwarder.logs_dd_url"))
		assert.Equal(t, expectedBaseHTTPURL, c.GetString("orchestrator_explorer.orchestrator_dd_url"))
		assert.Equal(t, expectedBaseURL, c.GetString("runtime_security_config.endpoints.logs_dd_url"))
		assert.Equal(t, expectedBaseURL, c.GetString("compliance_config.endpoints.logs_dd_url"))
		assert.Equal(t, expectedBaseURL, c.GetString("network_devices.netflow.forwarder.logs_dd_url"))
	}
}

func TestSetupFipsEndpointsNonLocalAddress(t *testing.T) {
	datadogYaml := `
fips:
  enabled: true
  local_address: 1.2.3.4
  port_range_start: 5000
`

	testConfig := confFromYAML(t, datadogYaml)
	err := setupFipsEndpoints(testConfig)
	require.Error(t, err)
}

func TestEnablePeerServiceStatsAggregationYAML(t *testing.T) {
	datadogYaml := `
apm_config:
  peer_service_aggregation: true
`
	testConfig := confFromYAML(t, datadogYaml)
	err := setupFipsEndpoints(testConfig)
	require.NoError(t, err)
	require.True(t, testConfig.GetBool("apm_config.peer_service_aggregation"))

	datadogYaml = `
apm_config:
  peer_service_aggregation: false
`
	testConfig = confFromYAML(t, datadogYaml)
	err = setupFipsEndpoints(testConfig)
	require.NoError(t, err)
	require.False(t, testConfig.GetBool("apm_config.peer_service_aggregation"))
}

func TestEnablePeerTagsAggregationYAML(t *testing.T) {
	datadogYaml := `
apm_config:
  peer_tags_aggregation: true
`
	testConfig := confFromYAML(t, datadogYaml)
	err := setupFipsEndpoints(testConfig)
	require.NoError(t, err)
	require.True(t, testConfig.GetBool("apm_config.peer_tags_aggregation"))

	datadogYaml = `
apm_config:
  peer_tags_aggregation: false
`
	testConfig = confFromYAML(t, datadogYaml)
	err = setupFipsEndpoints(testConfig)
	require.NoError(t, err)
	require.False(t, testConfig.GetBool("apm_config.peer_tags_aggregation"))
}

func TestEnablePeerServiceStatsAggregationEnv(t *testing.T) {
	t.Setenv("DD_APM_PEER_SERVICE_AGGREGATION", "true")
	testConfig := newTestConf()
	require.True(t, testConfig.GetBool("apm_config.peer_service_aggregation"))
	t.Setenv("DD_APM_PEER_SERVICE_AGGREGATION", "false")
	testConfig = newTestConf()
	require.False(t, testConfig.GetBool("apm_config.peer_service_aggregation"))
}

func TestEnablePeerTagsAggregationEnv(t *testing.T) {
	testConfig := newTestConf()
	require.True(t, testConfig.GetBool("apm_config.peer_tags_aggregation"))

	t.Setenv("DD_APM_PEER_TAGS_AGGREGATION", "true")
	testConfig = newTestConf()
	require.True(t, testConfig.GetBool("apm_config.peer_tags_aggregation"))

	t.Setenv("DD_APM_PEER_TAGS_AGGREGATION", "false")
	testConfig = newTestConf()
	require.False(t, testConfig.GetBool("apm_config.peer_tags_aggregation"))
}

func TestEnableStatsComputationBySpanKindYAML(t *testing.T) {
	datadogYaml := ""
	testConfig := confFromYAML(t, datadogYaml)
	err := setupFipsEndpoints(testConfig)
	require.NoError(t, err)
	require.True(t, testConfig.GetBool("apm_config.compute_stats_by_span_kind"))

	datadogYaml = `
apm_config:
  compute_stats_by_span_kind: false
`
	testConfig = confFromYAML(t, datadogYaml)
	err = setupFipsEndpoints(testConfig)
	require.NoError(t, err)
	require.False(t, testConfig.GetBool("apm_config.compute_stats_by_span_kind"))

	datadogYaml = `
apm_config:
  compute_stats_by_span_kind: true
`
	testConfig = confFromYAML(t, datadogYaml)
	err = setupFipsEndpoints(testConfig)
	require.NoError(t, err)
	require.True(t, testConfig.GetBool("apm_config.compute_stats_by_span_kind"))
}

func TestComputeStatsBySpanKindEnv(t *testing.T) {
	testConfig := newTestConf()
	require.True(t, testConfig.GetBool("apm_config.compute_stats_by_span_kind"))

	t.Setenv("DD_APM_COMPUTE_STATS_BY_SPAN_KIND", "false")
	testConfig = newTestConf()
	require.False(t, testConfig.GetBool("apm_config.compute_stats_by_span_kind"))

	t.Setenv("DD_APM_COMPUTE_STATS_BY_SPAN_KIND", "true")
	testConfig = newTestConf()
	require.True(t, testConfig.GetBool("apm_config.compute_stats_by_span_kind"))
}

func TestIsRemoteConfigEnabled(t *testing.T) {
	t.Setenv("DD_REMOTE_CONFIGURATION_ENABLED", "true")
	testConfig := newTestConf()
	require.True(t, IsRemoteConfigEnabled(testConfig))

	t.Setenv("DD_FIPS_ENABLED", "true")
	testConfig = newTestConf()
	require.False(t, IsRemoteConfigEnabled(testConfig))

	t.Setenv("DD_FIPS_ENABLED", "false")
	t.Setenv("DD_SITE", "ddog-gov.com")
	testConfig = newTestConf()
	require.False(t, IsRemoteConfigEnabled(testConfig))
}

func TestGetRemoteConfigurationAllowedIntegrations(t *testing.T) {
	// EMPTY configuration
	testConfig := newTestConf()
	require.Equal(t, map[string]bool{}, GetRemoteConfigurationAllowedIntegrations(testConfig))

	t.Setenv("DD_REMOTE_CONFIGURATION_AGENT_INTEGRATIONS_ALLOW_LIST", "[\"POSTgres\", \"redisDB\"]")
	testConfig = newTestConf()
	require.Equal(t,
		map[string]bool{"postgres": true, "redisdb": true},
		GetRemoteConfigurationAllowedIntegrations(testConfig),
	)

	t.Setenv("DD_REMOTE_CONFIGURATION_AGENT_INTEGRATIONS_BLOCK_LIST", "[\"mySQL\", \"redisDB\"]")
	testConfig = newTestConf()
	require.Equal(t,
		map[string]bool{"postgres": true, "redisdb": false, "mysql": false},
		GetRemoteConfigurationAllowedIntegrations(testConfig),
	)
}

func TestLanguageDetectionSettings(t *testing.T) {
	testConfig := newTestConf()
	require.False(t, testConfig.GetBool("language_detection.enabled"))

	t.Setenv("DD_LANGUAGE_DETECTION_ENABLED", "true")
	testConfig = newTestConf()
	require.True(t, testConfig.GetBool("language_detection.enabled"))
}

func TestPeerTagsYAML(t *testing.T) {
	testConfig := newTestConf()
	require.Nil(t, testConfig.GetStringSlice("apm_config.peer_tags"))

	datadogYaml := `
apm_config:
  peer_tags: ["aws.s3.bucket", "db.instance", "db.system"]
`
	testConfig = confFromYAML(t, datadogYaml)
	require.Equal(t, []string{"aws.s3.bucket", "db.instance", "db.system"}, testConfig.GetStringSlice("apm_config.peer_tags"))
}

func TestPeerTagsEnv(t *testing.T) {
	testConfig := newTestConf()
	require.Nil(t, testConfig.GetStringSlice("apm_config.peer_tags"))

	t.Setenv("DD_APM_PEER_TAGS", `["aws.s3.bucket","db.instance","db.system"]`)
	testConfig = newTestConf()
	require.Equal(t, []string{"aws.s3.bucket", "db.instance", "db.system"}, testConfig.GetStringSlice("apm_config.peer_tags"))
}

func TestLogDefaults(t *testing.T) {
	// New config
	c := pkgconfigmodel.NewConfig("test", "DD", strings.NewReplacer(".", "_")) // nolint: forbidigo // legit use case
	require.Equal(t, 0, c.GetInt("log_file_max_rolls"))
	require.Equal(t, "", c.GetString("log_file_max_size"))
	require.Equal(t, "", c.GetString("log_file"))
	require.Equal(t, "", c.GetString("log_level"))
	require.False(t, c.GetBool("log_to_console"))
	require.False(t, c.GetBool("log_format_json"))

	// Test Config (same as Datadog)
	testConfig := newTestConf()
	require.Equal(t, 1, testConfig.GetInt("log_file_max_rolls"))
	require.Equal(t, "10Mb", testConfig.GetString("log_file_max_size"))
	require.Equal(t, "", testConfig.GetString("log_file"))
	require.Equal(t, "info", testConfig.GetString("log_level"))
	require.True(t, testConfig.GetBool("log_to_console"))
	require.False(t, testConfig.GetBool("log_format_json"))

	// SystemProbe config

	SystemProbe := pkgconfigmodel.NewConfig("datadog", "DD", strings.NewReplacer(".", "_")) // nolint: forbidigo // legit use case
	InitSystemProbeConfig(SystemProbe)

	require.Equal(t, 1, SystemProbe.GetInt("log_file_max_rolls"))
	require.Equal(t, "10Mb", SystemProbe.GetString("log_file_max_size"))
	require.Equal(t, defaultSystemProbeLogFilePath, SystemProbe.GetString("log_file"))
	require.Equal(t, "info", SystemProbe.GetString("log_level"))
	require.True(t, SystemProbe.GetBool("log_to_console"))
	require.False(t, SystemProbe.GetBool("log_format_json"))
}

func TestProxyNotLoaded(t *testing.T) {
	conf := newTestConf()
	t.Setenv("AWS_LAMBDA_FUNCTION_NAME", "TestFunction")

	proxyHTTP := "http://localhost:1234"
	proxyHTTPS := "https://localhost:1234"
	t.Setenv("DD_PROXY_HTTP", proxyHTTP)
	t.Setenv("DD_PROXY_HTTPS", proxyHTTPS)

	proxyHTTPConfig := conf.GetString("proxy.http")
	proxyHTTPSConfig := conf.GetString("proxy.https")
	assert.Equal(t, 0, len(proxyHTTPConfig))
	assert.Equal(t, 0, len(proxyHTTPSConfig))
}

func TestProxyLoadedFromEnvVars(t *testing.T) {
	conf := newTestConf()
	t.Setenv("AWS_LAMBDA_FUNCTION_NAME", "TestFunction")

	proxyHTTP := "http://localhost:1234"
	proxyHTTPS := "https://localhost:1234"
	t.Setenv("DD_PROXY_HTTP", proxyHTTP)
	t.Setenv("DD_PROXY_HTTPS", proxyHTTPS)

	LoadWithoutSecret(conf, []string{})

	proxyHTTPConfig := conf.GetString("proxy.http")
	proxyHTTPSConfig := conf.GetString("proxy.https")

	assert.Equal(t, proxyHTTP, proxyHTTPConfig)
	assert.Equal(t, proxyHTTPS, proxyHTTPSConfig)
}

func TestProxyLoadedFromConfigFile(t *testing.T) {
	conf := newTestConf()
	t.Setenv("AWS_LAMBDA_FUNCTION_NAME", "TestFunction")

	tempDir := t.TempDir()
	configTest := path.Join(tempDir, "datadog.yaml")
	os.WriteFile(configTest, []byte("proxy:\n  http: \"http://localhost:1234\"\n  https: \"https://localhost:1234\""), 0o644)

	conf.AddConfigPath(tempDir)
	LoadWithoutSecret(conf, []string{})

	proxyHTTPConfig := conf.GetString("proxy.http")
	proxyHTTPSConfig := conf.GetString("proxy.https")

	assert.Equal(t, "http://localhost:1234", proxyHTTPConfig)
	assert.Equal(t, "https://localhost:1234", proxyHTTPSConfig)
}

func TestProxyLoadedFromConfigFileAndEnvVars(t *testing.T) {
	conf := newTestConf()
	t.Setenv("AWS_LAMBDA_FUNCTION_NAME", "TestFunction")

	proxyHTTPEnvVar := "http://localhost:1234"
	proxyHTTPSEnvVar := "https://localhost:1234"
	t.Setenv("DD_PROXY_HTTP", proxyHTTPEnvVar)
	t.Setenv("DD_PROXY_HTTPS", proxyHTTPSEnvVar)

	tempDir := t.TempDir()
	configTest := path.Join(tempDir, "datadog.yaml")
	os.WriteFile(configTest, []byte("proxy:\n  http: \"http://localhost:5678\"\n  https: \"http://localhost:5678\""), 0o644)

	conf.AddConfigPath(tempDir)
	LoadWithoutSecret(conf, []string{})

	proxyHTTPConfig := conf.GetString("proxy.http")
	proxyHTTPSConfig := conf.GetString("proxy.https")

	assert.Equal(t, proxyHTTPEnvVar, proxyHTTPConfig)
	assert.Equal(t, proxyHTTPSEnvVar, proxyHTTPSConfig)
}

var testExampleConf = []byte(`
secret_backend_command: some command
additional_endpoints:
  https://url1.com:
    - first
    - second
  https://url2.eu:
    - third
process_config:
  additional_endpoints:
    https://url1.com:
      - fourth
      - fifth
    https://url2.eu:
      - sixth
`)

func TestConfigAssignAtPath(t *testing.T) {
	// CircleCI sets NO_PROXY, so unset it for this test
	unsetEnvForTest(t, "NO_PROXY")

	config := newTestConf()
	config.SetWithoutSource("use_proxy_for_cloud_metadata", true)
	configPath := filepath.Join(t.TempDir(), "datadog.yaml")
	os.WriteFile(configPath, testExampleConf, 0o600)
	config.SetConfigFile(configPath)

	err := LoadCustom(config, nil)
	assert.NoError(t, err)

	err = configAssignAtPath(config, []string{"secret_backend_command"}, "different")
	assert.NoError(t, err)

	err = configAssignAtPath(config, []string{"additional_endpoints", "https://url1.com", "1"}, "changed")
	assert.NoError(t, err)

	err = configAssignAtPath(config, []string{"process_config", "additional_endpoints", "https://url2.eu", "0"}, "modified")
	assert.NoError(t, err)

	expectedYaml := `additional_endpoints:
  https://url1.com:
  - first
  - changed
  https://url2.eu:
  - third
process_config:
  additional_endpoints:
    https://url1.com:
    - fourth
    - fifth
    https://url2.eu:
    - modified
secret_backend_command: different
use_proxy_for_cloud_metadata: true
`
	yamlConf, err := yaml.Marshal(config.AllSettingsWithoutDefault())
	assert.NoError(t, err)
	yamlText := string(yamlConf)
	assert.Equal(t, expectedYaml, yamlText)

	err = configAssignAtPath(config, []string{"0"}, "invalid")
	assert.Error(t, err)
	assert.Equal(t, err.Error(), "unknown config setting '[0]'")

	err = configAssignAtPath(config, []string{"additional_endpoints", "https://url1.com", "5"}, "invalid")
	assert.Error(t, err)
	assert.Equal(t, err.Error(), "index out of range 5 >= 2")
}

func TestConfigAssignAtPathWorksWithGet(t *testing.T) {
	// CircleCI sets NO_PROXY, so unset it for this test
	unsetEnvForTest(t, "NO_PROXY")

	config := newTestConf()
	config.SetWithoutSource("use_proxy_for_cloud_metadata", true)
	configPath := filepath.Join(t.TempDir(), "datadog.yaml")
	os.WriteFile(configPath, testExampleConf, 0o600)
	config.SetConfigFile(configPath)

	err := LoadCustom(config, nil)
	assert.NoError(t, err)

	err = configAssignAtPath(config, []string{"secret_backend_command"}, "different")
	assert.NoError(t, err)

	err = configAssignAtPath(config, []string{"additional_endpoints", "https://url1.com", "1"}, "changed")
	assert.NoError(t, err)

	err = configAssignAtPath(config, []string{"process_config", "additional_endpoints", "https://url2.eu", "0"}, "modified")
	assert.NoError(t, err)

	var expected interface{} = `different`
	res := config.Get("secret_backend_command")
	require.Equal(t, expected, res)

	expected = []interface{}([]interface{}{"first", "changed"})
	res = config.Get("additional_endpoints.https://url1.com")
	require.Equal(t, expected, res)

	expected = []interface{}([]interface{}{"modified"})
	res = config.Get("process_config.additional_endpoints.https://url2.eu")
	require.Equal(t, expected, res)
}

var testSimpleConf = []byte(`secret_backend_command: some command
secret_backend_arguments:
- ENC[pass1]
`)

func TestConfigAssignAtPathSimple(t *testing.T) {
	// CircleCI sets NO_PROXY, so unset it for this test
	unsetEnvForTest(t, "NO_PROXY")

	config := newTestConf()
	config.SetWithoutSource("use_proxy_for_cloud_metadata", true)
	configPath := filepath.Join(t.TempDir(), "datadog.yaml")
	os.WriteFile(configPath, testSimpleConf, 0o600)
	config.SetConfigFile(configPath)

	err := LoadCustom(config, nil)
	assert.NoError(t, err)

	err = configAssignAtPath(config, []string{"secret_backend_arguments", "0"}, "password1")
	assert.NoError(t, err)

	expectedYaml := `secret_backend_arguments:
- password1
secret_backend_command: some command
use_proxy_for_cloud_metadata: true
`
	yamlConf, err := yaml.Marshal(config.AllSettingsWithoutDefault())
	assert.NoError(t, err)
	yamlText := string(yamlConf)
	assert.Equal(t, expectedYaml, yamlText)
}

func TestConfigMustMatchOrigin(t *testing.T) {
	// CircleCI sets NO_PROXY, so unset it for this test
	unsetEnvForTest(t, "NO_PROXY")

	testMinimalConf := []byte(`apm_config:
  apm_dd_url: ENC[some_url]
secret_backend_command: command
use_proxy_for_cloud_metadata: true
`)

	testMinimalDiffConf := []byte(`apm_config:
  apm_dd_url: ENC[diff_url]
secret_backend_command: command
use_proxy_for_cloud_metadata: true
`)

	expectedYaml := `apm_config:
  apm_dd_url: first_value
secret_backend_command: command
use_proxy_for_cloud_metadata: true
`
	expectedDiffYaml := `apm_config:
  apm_dd_url: second_value
secret_backend_command: command
use_proxy_for_cloud_metadata: true
`

	config := newTestConf()
	configPath := filepath.Join(t.TempDir(), "datadog.yaml")
	os.WriteFile(configPath, testMinimalConf, 0o600)
	config.SetConfigFile(configPath)

	resolver := fxutil.Test[secrets.Component](t, fx.Options(
		secretsimpl.MockModule(),
		nooptelemetry.Module(),
	))

	mockresolver := resolver.(secrets.Mock)
	mockresolver.SetBackendCommand("command")
	mockresolver.SetFetchHookFunc(func(_ []string) (map[string]string, error) {
		return map[string]string{
			"some_url": "first_value",
			"diff_url": "second_value",
		}, nil
	})

	err := LoadCustom(config, nil)
	assert.NoError(t, err)

	err = ResolveSecrets(config, resolver, "unit_test")
	require.NoError(t, err)

	yamlConf, err := yaml.Marshal(config.AllSettingsWithoutDefault())
	assert.NoError(t, err)
	assert.YAMLEq(t, expectedYaml, string(yamlConf))

	// use resolver to modify a 2nd config with a different origin
	diffYaml, err := resolver.Resolve(testMinimalDiffConf, "diff_test")
	assert.NoError(t, err)
	assert.YAMLEq(t, expectedDiffYaml, string(diffYaml))

	// verify that the original config was not changed because origin is different
	yamlConf, err = yaml.Marshal(config.AllSettingsWithoutDefault())
	assert.NoError(t, err)
	assert.YAMLEq(t, expectedYaml, string(yamlConf))

	// use resolver again, but with the original origin now
	diffYaml, err = resolver.Resolve(testMinimalDiffConf, "unit_test")
	assert.NoError(t, err)
	assert.YAMLEq(t, expectedDiffYaml, string(diffYaml))

	// now the original config was modified because of the origin match
	yamlConf, err = yaml.Marshal(config.AllSettingsWithoutDefault())
	assert.NoError(t, err)
	assert.YAMLEq(t, expectedDiffYaml, string(yamlConf))
}

func TestConfigAssignAtPathForIntMapKeys(t *testing.T) {
	// CircleCI sets NO_PROXY, so unset it for this test
	unsetEnvForTest(t, "NO_PROXY")

	// Even if a map is using keys that looks like stringified ints, calling
	// configAssignAtPath will still work correctly
	testIntKeysConf := []byte(`
additional_endpoints:
  0: apple
  1: banana
  2: carrot
`)
	config := newTestConf()
	config.SetWithoutSource("use_proxy_for_cloud_metadata", true)
	configPath := filepath.Join(t.TempDir(), "datadog.yaml")
	os.WriteFile(configPath, testIntKeysConf, 0o600)
	config.SetConfigFile(configPath)

	err := LoadCustom(config, nil)
	assert.NoError(t, err)

	err = configAssignAtPath(config, []string{"additional_endpoints", "2"}, "cherry")
	assert.NoError(t, err)

	expectedYaml := `additional_endpoints:
  "0": apple
  "1": banana
  "2": cherry
use_proxy_for_cloud_metadata: true
`
	yamlConf, err := yaml.Marshal(config.AllSettingsWithoutDefault())
	assert.NoError(t, err)
	yamlText := string(yamlConf)
	assert.Equal(t, expectedYaml, yamlText)
}

func TestServerlessConfigNumComponents(t *testing.T) {
	// Enforce the number of config "components" reachable by the serverless agent
	// to avoid accidentally adding entire components if it's not needed
	require.Len(t, serverlessConfigComponents, 22)
}

func TestServerlessConfigInit(t *testing.T) {
	conf := pkgconfigmodel.NewConfig("datadog", "DD", strings.NewReplacer(".", "_")) // nolint: forbidigo // legit use case

	initCommonWithServerless(conf)

	// ensure some core configs are declared
	assert.True(t, conf.IsKnown("api_key"))
	assert.True(t, conf.IsKnown("use_dogstatsd"))
	assert.True(t, conf.IsKnown("forwarder_timeout"))

	// ensure some non-serverless configs are not declared
	assert.False(t, conf.IsKnown("sbom.enabled"))
	assert.False(t, conf.IsKnown("inventories_enabled"))
}

func TestAgentConfigInit(t *testing.T) {
	conf := newTestConf()

	assert.True(t, conf.IsKnown("api_key"))
	assert.True(t, conf.IsKnown("use_dogstatsd"))
	assert.True(t, conf.IsKnown("forwarder_timeout"))
	assert.True(t, conf.IsKnown("sbom.enabled"))
	assert.True(t, conf.IsKnown("inventories_enabled"))
}

func TestENVAdditionalKeysToScrubber(t *testing.T) {
	// Test that the scrubber is correctly configured with the expected keys
	cfg := pkgconfigmodel.NewConfig("test", "DD", strings.NewReplacer(".", "_")) // nolint: forbidigo // legit use case

	data := `scrubber.additional_keys:
- yet_another_key
flare_stripped_keys:
- some_other_key`

	path := t.TempDir()
	configPath := filepath.Join(path, "empty_conf.yaml")
	err := os.WriteFile(configPath, []byte(data), 0o600)
	require.NoError(t, err)
	cfg.SetConfigFile(configPath)

	_, err = LoadDatadogCustom(cfg, "test", optional.NewNoneOption[secrets.Component](), []string{})
	require.NoError(t, err)

	stringToScrub := `api_key: 'aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa'
some_other_key: 'bbbb'
app_key: 'aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaacccc'
yet_another_key: 'dddd'`

	scrubbed, err := scrubber.ScrubYamlString(stringToScrub)
	assert.Nil(t, err)
	expected := `api_key: '***************************aaaaa'
some_other_key: "********"
app_key: '***********************************acccc'
yet_another_key: "********"`
	assert.YAMLEq(t, expected, scrubbed)
}
