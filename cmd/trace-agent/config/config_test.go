// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package config

import (
	"errors"
	"html/template"
	"io/ioutil"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"regexp"
	"strings"
	"testing"
	"time"

	coreconfig "github.com/DataDog/datadog-agent/pkg/config"
	"github.com/DataDog/datadog-agent/pkg/trace/config"
	"github.com/DataDog/datadog-agent/pkg/trace/config/features"
	"github.com/cihub/seelog"
	"github.com/stretchr/testify/assert"
)

func TestMain(m *testing.M) {
	seelog.UseLogger(seelog.Disabled)
	os.Exit(m.Run())
}

func cleanConfig() func() {
	oldConfig := coreconfig.Datadog
	coreconfig.Datadog = coreconfig.NewConfig("datadog", "DD", strings.NewReplacer(".", "_"))
	coreconfig.InitConfig(coreconfig.Datadog)
	return func() { coreconfig.Datadog = oldConfig }
}

// TestParseReplaceRules tests the compileReplaceRules helper function.
func TestParseRepaceRules(t *testing.T) {
	assert := assert.New(t)
	rules := []*config.ReplaceRule{
		{Name: "http.url", Pattern: "(token/)([^/]*)", Repl: "${1}?"},
		{Name: "http.url", Pattern: "guid", Repl: "[REDACTED]"},
		{Name: "custom.tag", Pattern: "(/foo/bar/).*", Repl: "${1}extra"},
	}
	err := compileReplaceRules(rules)
	if err != nil {
		t.Fatal(err)
	}
	for _, r := range rules {
		assert.Equal(r.Pattern, r.Re.String())
	}
}

func TestSplitTag(t *testing.T) {
	for _, tt := range []struct {
		tag string
		kv  *config.Tag
	}{
		{
			tag: "",
			kv:  &config.Tag{K: ""},
		},
		{
			tag: "key:value",
			kv:  &config.Tag{K: "key", V: "value"},
		},
		{
			tag: "env:prod",
			kv:  &config.Tag{K: "env", V: "prod"},
		},
		{
			tag: "env:staging:east",
			kv:  &config.Tag{K: "env", V: "staging:east"},
		},
		{
			tag: "key",
			kv:  &config.Tag{K: "key"},
		},
	} {
		t.Run("", func(t *testing.T) {
			assert.Equal(t, splitTag(tt.tag), tt.kv)
		})
	}
}

func TestTelemetryEndpointsConfig(t *testing.T) {
	t.Run("default", func(t *testing.T) {
		cfg := config.New()
		err := applyDatadogConfig(cfg)

		assert := assert.New(t)
		assert.NoError(err)
		assert.True(cfg.TelemetryConfig.Enabled)
		assert.Len(cfg.TelemetryConfig.Endpoints, 1)
		assert.Equal("https://instrumentation-telemetry-intake.datadoghq.com", cfg.TelemetryConfig.Endpoints[0].Host)
	})

	t.Run("dd_url", func(t *testing.T) {
		defer cleanConfig()
		coreconfig.Datadog.Set("apm_config.telemetry.dd_url", "http://example.com/")

		cfg := config.New()
		err := applyDatadogConfig(cfg)

		assert := assert.New(t)
		assert.NoError(err)
		assert.True(cfg.TelemetryConfig.Enabled)
		assert.Equal("http://example.com/", cfg.TelemetryConfig.Endpoints[0].Host)
	})

	t.Run("dd_url-malformed", func(t *testing.T) {
		defer cleanConfig()
		coreconfig.Datadog.Set("apm_config.telemetry.dd_url", "111://abc.com")

		cfg := config.New()
		err := applyDatadogConfig(cfg)

		assert := assert.New(t)
		assert.NoError(err)
		assert.True(cfg.TelemetryConfig.Enabled)
		assert.Equal(cfg.TelemetryConfig.Endpoints[0].Host, "111://abc.com")
	})

	t.Run("site", func(t *testing.T) {
		defer cleanConfig()
		coreconfig.Datadog.Set("site", "new_site.example.com")

		cfg := config.New()
		err := applyDatadogConfig(cfg)
		assert := assert.New(t)
		assert.NoError(err)
		assert.True(cfg.TelemetryConfig.Enabled)
		assert.Len(cfg.TelemetryConfig.Endpoints, 1)
		assert.Equal("https://instrumentation-telemetry-intake.new_site.example.com", cfg.TelemetryConfig.Endpoints[0].Host)
	})

	t.Run("additional-hosts", func(t *testing.T) {
		defer cleanConfig()
		additionalEndpoints := map[string]string{
			"http://test_backend_2.example.com": "test_apikey_2",
			"http://test_backend_3.example.com": "test_apikey_3",
		}
		coreconfig.Datadog.Set("apm_config.telemetry.additional_endpoints", additionalEndpoints)

		cfg := config.New()
		err := applyDatadogConfig(cfg)

		assert := assert.New(t)
		assert.NoError(err)
		assert.Equal("https://instrumentation-telemetry-intake.datadoghq.com", cfg.TelemetryConfig.Endpoints[0].Host)

		assert.True(cfg.TelemetryConfig.Enabled)
		assert.Len(cfg.TelemetryConfig.Endpoints, 3)

		for _, endpoint := range cfg.TelemetryConfig.Endpoints[1:] {
			assert.NotNil(additionalEndpoints[endpoint.Host])
			assert.Equal(endpoint.APIKey, additionalEndpoints[endpoint.Host])
		}
	})

	t.Run("additional-urls", func(t *testing.T) {
		defer cleanConfig()
		additionalEndpoints := map[string]string{
			"http://test_backend_2.example.com": "test_apikey_2",
			"http://test_backend_3.example.com": "test_apikey_3",
		}
		coreconfig.Datadog.Set("apm_config.telemetry.additional_endpoints", additionalEndpoints)

		cfg := config.New()
		err := applyDatadogConfig(cfg)

		assert := assert.New(t)
		assert.NoError(err)
		assert.Equal("https://instrumentation-telemetry-intake.datadoghq.com", cfg.TelemetryConfig.Endpoints[0].Host)
		assert.True(cfg.TelemetryConfig.Enabled)
		assert.Len(cfg.TelemetryConfig.Endpoints, 3)
		for _, endpoint := range cfg.TelemetryConfig.Endpoints[1:] {
			assert.NotNil(additionalEndpoints[endpoint.Host])
			assert.Equal(endpoint.APIKey, additionalEndpoints[endpoint.Host])
		}
	})

	t.Run("keep-malformed", func(t *testing.T) {
		defer cleanConfig()
		additionalEndpoints := map[string]string{
			"11://test_backend_2.example.com///": "test_apikey_2",
			"http://test_backend_3.example.com/": "test_apikey_3",
		}
		coreconfig.Datadog.Set("apm_config.telemetry.additional_endpoints", additionalEndpoints)

		cfg := config.New()
		err := applyDatadogConfig(cfg)
		assert := assert.New(t)
		assert.NoError(err)

		assert.True(cfg.TelemetryConfig.Enabled)
		assert.Len(cfg.TelemetryConfig.Endpoints, 3)
		assert.Equal("https://instrumentation-telemetry-intake.datadoghq.com", cfg.TelemetryConfig.Endpoints[0].Host)
		for _, endpoint := range cfg.TelemetryConfig.Endpoints[1:] {
			assert.Contains(additionalEndpoints, endpoint.Host)
		}
	})
}

func TestConfigHostname(t *testing.T) {
	t.Run("fail", func(t *testing.T) {
		defer cleanConfig()()
		coreconfig.Datadog.Set("apm_config.dd_agent_bin", "/not/exist")
		coreconfig.Datadog.Set("cmd_port", "-1")
		assert := assert.New(t)
		fallbackHostnameFunc = func() (string, error) {
			return "", errors.New("could not get hostname")
		}
		defer func() {
			fallbackHostnameFunc = os.Hostname
		}()
		_, err := LoadConfigFile("./testdata/site_override.yaml")
		assert.Contains(err.Error(), "nor from OS")
	})

	t.Run("fallback", func(t *testing.T) {
		defer cleanConfig()()
		host, err := os.Hostname()
		if err != nil || host == "" {
			// can't say
			t.Skip()
		}
		assert := assert.New(t)
		cfg, err := LoadConfigFile("./testdata/site_override.yaml")
		assert.NoError(err)
		assert.Equal(host, cfg.Hostname)
	})

	t.Run("file", func(t *testing.T) {
		defer cleanConfig()()
		assert := assert.New(t)
		cfg, err := LoadConfigFile("./testdata/full.yaml")
		assert.NoError(err)
		assert.Equal("mymachine", cfg.Hostname)
	})

	t.Run("env", func(t *testing.T) {
		defer cleanConfig()()
		// hostname from env
		assert := assert.New(t)
		err := os.Setenv("DD_HOSTNAME", "onlyenv")
		defer os.Unsetenv("DD_HOSTNAME")
		assert.NoError(err)
		cfg, err := LoadConfigFile("./testdata/site_override.yaml")
		assert.NoError(err)
		assert.Equal("onlyenv", cfg.Hostname)
	})

	t.Run("file+env", func(t *testing.T) {
		defer cleanConfig()()
		// hostname from file, overwritten from env
		assert := assert.New(t)
		err := os.Setenv("DD_HOSTNAME", "envoverride")
		defer os.Unsetenv("DD_HOSTNAME")
		assert.NoError(err)
		cfg, err := LoadConfigFile("./testdata/full.yaml")
		assert.NoError(err)
		assert.Equal("envoverride", cfg.Hostname)
	})

	t.Run("external", func(t *testing.T) {
		body, err := ioutil.ReadFile("testdata/stringcode.go.tmpl")
		if err != nil {
			t.Fatal(err)
		}
		// makeProgram creates a new binary file which returns the given response and exits to the OS
		// given the specified code, returning the path of the program.
		makeProgram := func(response string, code int) string {
			f, err := ioutil.TempFile("", "trace-test-hostname.*.go")
			if err != nil {
				t.Fatal(err)
			}
			tmpl, err := template.New("program").Parse(string(body))
			if err != nil {
				t.Fatal(err)
			}
			if err := tmpl.Execute(f, struct {
				Response string
				ExitCode int
			}{response, code}); err != nil {
				t.Fatal(err)
			}
			stat, err := f.Stat()
			if err != nil {
				t.Fatal(err)
			}
			srcpath := filepath.Join(os.TempDir(), stat.Name())
			binpath := strings.TrimSuffix(srcpath, ".go")
			if err := exec.Command("go", "build", "-o", binpath, srcpath).Run(); err != nil {
				t.Fatal(err)
			}
			os.Remove(srcpath)
			return binpath
		}

		defer func(old func() (string, error)) { fallbackHostnameFunc = old }(fallbackHostnameFunc)
		fallbackHostnameFunc = func() (string, error) { return "fallback.host", nil }

		t.Run("good", func(t *testing.T) {
			cfg := config.AgentConfig{DDAgentBin: makeProgram("host.name", 0)}
			defer os.Remove(cfg.DDAgentBin)
			assert.NoError(t, acquireHostnameFallback(&cfg))
			assert.Equal(t, cfg.Hostname, "host.name")
		})

		t.Run("empty", func(t *testing.T) {
			cfg := config.AgentConfig{DDAgentBin: makeProgram("", 0)}
			defer os.Remove(cfg.DDAgentBin)
			assert.NoError(t, acquireHostnameFallback(&cfg))
			assert.Empty(t, cfg.Hostname)
		})

		t.Run("empty+disallowed", func(t *testing.T) {
			features.Set("disable_empty_hostname")
			defer func() { features.Set(os.Getenv("DD_APM_FEATURES")) }()

			cfg := config.AgentConfig{DDAgentBin: makeProgram("", 0)}
			defer os.Remove(cfg.DDAgentBin)
			assert.NoError(t, acquireHostnameFallback(&cfg))
			assert.Equal(t, "fallback.host", cfg.Hostname)
		})

		t.Run("fallback1", func(t *testing.T) {
			cfg := config.AgentConfig{DDAgentBin: makeProgram("", 1)}
			defer os.Remove(cfg.DDAgentBin)
			assert.NoError(t, acquireHostnameFallback(&cfg))
			assert.Equal(t, cfg.Hostname, "fallback.host")
		})

		t.Run("fallback2", func(t *testing.T) {
			cfg := config.AgentConfig{DDAgentBin: makeProgram("some text", 1)}
			defer os.Remove(cfg.DDAgentBin)
			assert.NoError(t, acquireHostnameFallback(&cfg))
			assert.Equal(t, cfg.Hostname, "fallback.host")
		})
	})
}

func TestSite(t *testing.T) {
	for name, tt := range map[string]struct {
		file string
		url  string
	}{
		"default":  {"./testdata/site_default.yaml", "https://trace.agent.datadoghq.com"},
		"eu":       {"./testdata/site_eu.yaml", "https://trace.agent.datadoghq.eu"},
		"url":      {"./testdata/site_url.yaml", "some.other.datadoghq.eu"},
		"override": {"./testdata/site_override.yaml", "some.other.datadoghq.eu"},
	} {
		t.Run(name, func(t *testing.T) {
			defer cleanConfig()()
			cfg, err := LoadConfigFile(tt.file)
			assert.NoError(t, err)
			assert.Equal(t, tt.url, cfg.Endpoints[0].Host)
		})
	}
}

func TestDefaultConfig(t *testing.T) {
	assert := assert.New(t)
	c := config.New()

	// assert that some sane defaults are set
	assert.Equal("localhost", c.ReceiverHost)
	assert.Equal(8126, c.ReceiverPort)

	assert.Equal("localhost", c.StatsdHost)
	assert.Equal(8125, c.StatsdPort)

	assert.Equal("INFO", c.LogLevel)
	assert.Equal(true, c.Enabled)
}

func TestNoAPMConfig(t *testing.T) {
	defer cleanConfig()()
	assert := assert.New(t)

	c, err := prepareConfig("./testdata/no_apm_config.yaml")
	assert.NoError(err)
	assert.NoError(applyDatadogConfig(c))

	assert.Equal("thing", c.Hostname)
	assert.Equal("apikey_12", c.Endpoints[0].APIKey)
	assert.Equal("0.0.0.0", c.ReceiverHost)
	assert.Equal(28125, c.StatsdPort)
	assert.Equal("DEBUG", c.LogLevel)
}

func TestFullYamlConfig(t *testing.T) {
	defer cleanConfig()()
	origcfg := coreconfig.Datadog
	coreconfig.Datadog = coreconfig.NewConfig("datadog", "DD", strings.NewReplacer(".", "_"))
	defer func() {
		coreconfig.Datadog = origcfg
	}()

	assert := assert.New(t)

	c, err := prepareConfig("./testdata/full.yaml")
	assert.NoError(err)
	assert.NoError(applyDatadogConfig(c))

	assert.Equal("mymachine", c.Hostname)
	assert.Equal("https://user:password@proxy_for_https:1234", c.ProxyURL.String())
	assert.True(c.SkipSSLValidation)
	assert.Equal("info", c.LogLevel)
	assert.Equal(18125, c.StatsdPort)
	assert.False(c.Enabled)
	assert.Equal("abc", c.LogFilePath)
	assert.Equal("test", c.DefaultEnv)
	assert.Equal(123, c.ConnectionLimit)
	assert.Equal(18126, c.ReceiverPort)
	assert.Equal(0.5, c.ExtraSampleRate)
	assert.Equal(5.0, c.TargetTPS)
	assert.Equal(50.0, c.MaxEPS)
	assert.Equal(0.5, c.MaxCPU)
	assert.EqualValues(123.4, c.MaxMemory)
	assert.Equal("0.0.0.0", c.ReceiverHost)
	assert.True(c.LogThrottling)
	assert.True(c.OTLPReceiver.SpanNameAsResourceName)
	assert.Equal(map[string]string{"a": "b", "and:colons": "in:values", "c": "d", "with.dots": "in.side"}, c.OTLPReceiver.SpanNameRemappings)

	noProxy := true
	if _, ok := os.LookupEnv("NO_PROXY"); ok {
		// Happens in CircleCI: if the environment variable is set,
		// it will overwrite our loaded configuration and will cause
		// this test to fail.
		noProxy = false
	}
	assert.ElementsMatch([]*config.Endpoint{
		{Host: "https://datadog.unittests", APIKey: "api_key_test"},
		{Host: "https://my1.endpoint.com", APIKey: "apikey1"},
		{Host: "https://my1.endpoint.com", APIKey: "apikey2"},
		{Host: "https://my2.endpoint.eu", APIKey: "apikey3", NoProxy: noProxy},
		{Host: "https://my2.endpoint.eu", APIKey: "apikey4", NoProxy: noProxy},
		{Host: "https://my2.endpoint.eu", APIKey: "apikey5", NoProxy: noProxy},
	}, c.Endpoints)

	assert.ElementsMatch([]*config.Tag{{K: "env", V: "prod"}, {K: "db", V: "mongodb"}}, c.RequireTags)
	assert.ElementsMatch([]*config.Tag{{K: "outcome", V: "success"}}, c.RejectTags)

	assert.ElementsMatch([]*config.ReplaceRule{
		{
			Name:    "http.method",
			Pattern: "\\?.*$",
			Repl:    "GET",
			Re:      regexp.MustCompile("\\?.*$"),
		},
		{
			Name:    "http.url",
			Pattern: "\\?.*$",
			Repl:    "!",
			Re:      regexp.MustCompile("\\?.*$"),
		},
		{
			Name:    "error.stack",
			Pattern: "(?s).*",
			Repl:    "?",
			Re:      regexp.MustCompile("(?s).*"),
		},
	}, c.ReplaceTags)

	assert.EqualValues([]string{"/health", "/500"}, c.Ignore["resource"])

	o := c.Obfuscation
	assert.NotNil(o)
	assert.True(o.ES.Enabled)
	assert.EqualValues([]string{"user_id", "category_id"}, o.ES.KeepValues)
	assert.True(o.Mongo.Enabled)
	assert.EqualValues([]string{"uid", "cat_id"}, o.Mongo.KeepValues)
	assert.True(o.HTTP.RemoveQueryString)
	assert.True(o.HTTP.RemovePathDigits)
	assert.True(o.RemoveStackTraces)
	assert.True(o.Redis.Enabled)
	assert.True(o.Memcached.Enabled)
	assert.True(o.CreditCards.Enabled)
	assert.True(o.CreditCards.Luhn)
}

func TestUndocumentedYamlConfig(t *testing.T) {
	defer cleanConfig()()
	origcfg := coreconfig.Datadog
	coreconfig.Datadog = coreconfig.NewConfig("datadog", "DD", strings.NewReplacer(".", "_"))
	defer func() {
		coreconfig.Datadog = origcfg
	}()
	assert := assert.New(t)

	c, err := prepareConfig("./testdata/undocumented.yaml")
	assert.NoError(err)
	assert.NoError(applyDatadogConfig(c))

	assert.Equal("/path/to/bin", c.DDAgentBin)
	assert.Equal("thing", c.Hostname)
	assert.Equal("apikey_12", c.Endpoints[0].APIKey)
	assert.Equal(0.33, c.ExtraSampleRate)
	assert.Equal(100.0, c.TargetTPS)
	assert.Equal(37.0, c.ErrorTPS)
	assert.Equal(true, c.DisableRareSampler)
	assert.Equal(127.0, c.MaxRemoteTPS)
	assert.Equal(1000.0, c.MaxEPS)
	assert.Equal(25, c.ReceiverPort)
	assert.Equal(120*time.Second, c.ConnectionResetInterval)
	// watchdog
	assert.Equal(0.07, c.MaxCPU)
	assert.Equal(30e6, c.MaxMemory)

	// Assert Trace Writer
	assert.Equal(1, c.TraceWriter.ConnectionLimit)
	assert.Equal(2, c.TraceWriter.QueueSize)
	assert.Equal(5, c.StatsWriter.ConnectionLimit)
	assert.Equal(6, c.StatsWriter.QueueSize)
	// analysis legacy
	assert.Equal(1.0, c.AnalyzedRateByServiceLegacy["db"])
	assert.Equal(0.9, c.AnalyzedRateByServiceLegacy["web"])
	assert.Equal(0.5, c.AnalyzedRateByServiceLegacy["index"])
	// analysis
	assert.Len(c.AnalyzedSpansByService, 2)
	assert.Len(c.AnalyzedSpansByService["web"], 2)
	assert.Len(c.AnalyzedSpansByService["db"], 1)
	assert.Equal(0.8, c.AnalyzedSpansByService["web"]["request"])
	assert.Equal(0.9, c.AnalyzedSpansByService["web"]["django.request"])
	assert.Equal(0.05, c.AnalyzedSpansByService["db"]["intake"])
}

func TestAcquireHostnameFallback(t *testing.T) {
	c := config.New()
	err := acquireHostnameFallback(c)
	assert.Nil(t, err)
	host, _ := os.Hostname()
	assert.Equal(t, host, c.Hostname)
}

func TestNormalizeEnvFromDDEnv(t *testing.T) {
	assert := assert.New(t)

	for in, out := range map[string]string{
		"staging":   "staging",
		"stAging":   "staging",
		"staging 1": "staging_1",
	} {
		t.Run("", func(t *testing.T) {
			defer cleanConfig()()
			err := os.Setenv("DD_ENV", in)
			defer os.Unsetenv("DD_ENV")
			assert.NoError(err)
			cfg, err := LoadConfigFile("./testdata/no_apm_config.yaml")
			assert.NoError(err)
			assert.Equal(out, cfg.DefaultEnv)
		})
	}
}

func TestNormalizeEnvFromDDTags(t *testing.T) {
	assert := assert.New(t)

	for in, out := range map[string]string{
		"env:staging": "staging",
		"env:stAging": "staging",
		// The value of DD_TAGS is parsed with a space delimiter.
		"tag:value env:STAGING tag2:value2": "staging",
	} {
		t.Run("", func(t *testing.T) {
			defer cleanConfig()()
			err := os.Setenv("DD_TAGS", in)
			defer os.Unsetenv("DD_TAGS")
			assert.NoError(err)
			cfg, err := LoadConfigFile("./testdata/no_apm_config.yaml")
			assert.NoError(err)
			assert.Equal(out, cfg.DefaultEnv)
		})
	}
}

func TestNormalizeEnvFromConfig(t *testing.T) {
	assert := assert.New(t)

	for _, cfgFile := range []string{
		"./testdata/ok_env_apm_config.yaml",
		"./testdata/ok_env_top_level.yaml",
		"./testdata/ok_env_host_tag.yaml",
		"./testdata/non-normalized_env_apm_config.yaml",
		"./testdata/non-normalized_env_top_level.yaml",
		"./testdata/non-normalized_env_host_tag.yaml",
	} {
		t.Run("", func(t *testing.T) {
			defer cleanConfig()()
			cfg, err := LoadConfigFile(cfgFile)
			assert.NoError(err)
			assert.Equal("staging", cfg.DefaultEnv)
		})
	}
}

func TestLoadEnv(t *testing.T) {
	t.Run("overrides", func(t *testing.T) {
		// tests that newer envs. override deprecated ones
		for _, tt := range []struct {
			envOld, envNew, key string
		}{
			{"HTTPS_PROXY", "DD_PROXY_HTTPS", "proxy.https"},
			{"DD_CONNECTION_LIMIT", "DD_APM_CONNECTION_LIMIT", "apm_config.connection_limit"},
			{"DD_RECEIVER_PORT", "DD_APM_RECEIVER_PORT", "apm_config.receiver_port"},
			{"DD_MAX_EPS", "DD_MAX_EPS", "apm_config.max_events_per_second"},
			{"DD_MAX_TPS", "DD_APM_MAX_TPS", "apm_config.max_traces_per_second"},
			{"DD_IGNORE_RESOURCE", "DD_APM_IGNORE_RESOURCES", "apm_config.ignore_resources"},
		} {
			assert := assert.New(t)
			err := os.Setenv(tt.envOld, "1,2,3")
			assert.NoError(err)
			defer os.Unsetenv(tt.envOld)
			err = os.Setenv(tt.envNew, "4,5,6")
			assert.NoError(err)
			defer os.Unsetenv(tt.envNew)
			_, err = LoadConfigFile("./testdata/full.yaml")
			assert.NoError(err)
			if tt.envNew == "DD_APM_IGNORE_RESOURCES" {
				assert.Equal([]string{"4", "5", "6"}, coreconfig.Datadog.GetStringSlice(tt.key))
			} else {
				assert.Equal("4,5,6", coreconfig.Datadog.GetString(tt.key))
			}
		}
	})

	env := "DD_API_KEY"
	t.Run(env, func(t *testing.T) {
		defer cleanConfig()()
		assert := assert.New(t)
		err := os.Setenv(env, "123")
		assert.NoError(err)
		defer os.Unsetenv(env)
		cfg, err := LoadConfigFile("./testdata/full.yaml")
		assert.NoError(err)
		assert.Equal("123", cfg.Endpoints[0].APIKey)
	})

	env = "DD_SITE"
	t.Run(env, func(t *testing.T) {
		defer cleanConfig()()
		assert := assert.New(t)
		err := os.Setenv(env, "my-site.com")
		assert.NoError(err)
		defer os.Unsetenv(env)
		cfg, err := LoadConfigFile("./testdata/undocumented.yaml")
		assert.NoError(err)
		assert.Equal(apiEndpointPrefix+"my-site.com", cfg.Endpoints[0].Host)
	})

	env = "DD_APM_ENABLED"
	t.Run(env, func(t *testing.T) {
		defer cleanConfig()()
		assert := assert.New(t)
		err := os.Setenv(env, "true")
		assert.NoError(err)
		defer os.Unsetenv(env)
		cfg, err := LoadConfigFile("./testdata/full.yaml")
		assert.NoError(err)
		assert.True(cfg.Enabled)
	})

	env = "DD_APM_DD_URL"
	t.Run(env, func(t *testing.T) {
		defer cleanConfig()()
		assert := assert.New(t)
		err := os.Setenv(env, "my-site.com")
		assert.NoError(err)
		defer os.Unsetenv(env)
		cfg, err := LoadConfigFile("./testdata/full.yaml")
		assert.NoError(err)
		assert.Equal("my-site.com", cfg.Endpoints[0].Host)
	})

	env = "HTTPS_PROXY"
	t.Run(env, func(t *testing.T) {
		defer cleanConfig()()
		assert := assert.New(t)
		err := os.Setenv(env, "my-proxy.url")
		assert.NoError(err)
		defer os.Unsetenv(env)
		cfg, err := LoadConfigFile("./testdata/full.yaml")
		assert.NoError(err)
		assert.Equal("my-proxy.url", cfg.ProxyURL.String())
	})

	env = "DD_PROXY_HTTPS"
	t.Run(env, func(t *testing.T) {
		defer cleanConfig()()
		assert := assert.New(t)
		err := os.Setenv(env, "my-proxy.url")
		assert.NoError(err)
		defer os.Unsetenv(env)
		cfg, err := LoadConfigFile("./testdata/full.yaml")
		assert.NoError(err)
		assert.Equal("my-proxy.url", cfg.ProxyURL.String())
	})

	env = "DD_HOSTNAME"
	t.Run(env, func(t *testing.T) {
		defer cleanConfig()()
		assert := assert.New(t)
		err := os.Setenv(env, "local.host")
		assert.NoError(err)
		defer os.Unsetenv(env)
		cfg, err := LoadConfigFile("./testdata/full.yaml")
		assert.NoError(err)
		assert.Equal("local.host", cfg.Hostname)
	})

	env = "DD_BIND_HOST"
	t.Run(env, func(t *testing.T) {
		defer cleanConfig()()
		assert := assert.New(t)
		err := os.Setenv(env, "bindhost.com")
		assert.NoError(err)
		defer os.Unsetenv(env)
		cfg, err := LoadConfigFile("./testdata/full.yaml")
		assert.NoError(err)
		assert.Equal("bindhost.com", cfg.StatsdHost)
	})

	for _, envKey := range []string{
		"DD_RECEIVER_PORT", // deprecated
		"DD_APM_RECEIVER_PORT",
	} {
		t.Run(envKey, func(t *testing.T) {
			defer cleanConfig()()
			assert := assert.New(t)
			err := os.Setenv(envKey, "1234")
			assert.NoError(err)
			defer os.Unsetenv(envKey)
			cfg, err := LoadConfigFile("./testdata/full.yaml")
			assert.NoError(err)
			assert.Equal(1234, cfg.ReceiverPort)
		})
	}

	env = "DD_DOGSTATSD_PORT"
	t.Run(env, func(t *testing.T) {
		defer cleanConfig()()
		assert := assert.New(t)
		err := os.Setenv(env, "4321")
		assert.NoError(err)
		defer os.Unsetenv(env)
		cfg, err := LoadConfigFile("./testdata/full.yaml")
		assert.NoError(err)
		assert.Equal(4321, cfg.StatsdPort)
	})

	env = "DD_APM_NON_LOCAL_TRAFFIC"
	t.Run(env, func(t *testing.T) {
		defer cleanConfig()()
		assert := assert.New(t)
		err := os.Setenv(env, "true")
		assert.NoError(err)
		defer os.Unsetenv(env)
		cfg, err := LoadConfigFile("./testdata/undocumented.yaml")
		assert.NoError(err)
		assert.Equal("0.0.0.0", cfg.ReceiverHost)
	})

	for _, envKey := range []string{
		"DD_IGNORE_RESOURCE", // deprecated
		"DD_APM_IGNORE_RESOURCES",
	} {
		t.Run(envKey, func(t *testing.T) {
			defer cleanConfig()()
			assert := assert.New(t)
			err := os.Setenv(envKey, "1,2,3")
			assert.NoError(err)
			defer os.Unsetenv(envKey)
			cfg, err := LoadConfigFile("./testdata/full.yaml")
			assert.NoError(err)
			assert.Equal([]string{"1", "2", "3"}, cfg.Ignore["resource"])
		})
	}

	env = "DD_LOG_LEVEL"
	t.Run(env, func(t *testing.T) {
		defer cleanConfig()()
		assert := assert.New(t)
		err := os.Setenv(env, "warn")
		assert.NoError(err)
		defer os.Unsetenv(env)
		cfg, err := LoadConfigFile("./testdata/full.yaml")
		assert.NoError(err)
		assert.Equal("warn", cfg.LogLevel)
	})

	env = "DD_APM_ANALYZED_SPANS"
	t.Run(env, func(t *testing.T) {
		defer cleanConfig()()
		assert := assert.New(t)
		err := os.Setenv(env, "web|http.request=1,db|sql.query=0.5")
		assert.NoError(err)
		defer os.Unsetenv(env)
		cfg, err := LoadConfigFile("./testdata/full.yaml")
		assert.NoError(err)
		assert.Equal(map[string]map[string]float64{
			"web": {"http.request": 1},
			"db":  {"sql.query": 0.5},
		}, cfg.AnalyzedSpansByService)
	})

	env = "DD_APM_REPLACE_TAGS"
	t.Run(env, func(t *testing.T) {
		defer cleanConfig()()
		assert := assert.New(t)
		err := os.Setenv(env, `[{"name":"name1", "pattern":"pattern1"}, {"name":"name2","pattern":"pattern2","repl":"replace2"}]`)
		assert.NoError(err)
		defer os.Unsetenv(env)
		cfg, err := LoadConfigFile("./testdata/full.yaml")
		assert.NoError(err)
		rule1 := &config.ReplaceRule{
			Name:    "name1",
			Pattern: "pattern1",
			Repl:    "",
		}
		rule2 := &config.ReplaceRule{
			Name:    "name2",
			Pattern: "pattern2",
			Repl:    "replace2",
		}
		compileReplaceRules([]*config.ReplaceRule{rule1, rule2})
		assert.Contains(cfg.ReplaceTags, rule1)
		assert.Contains(cfg.ReplaceTags, rule2)
	})

	env = "DD_APM_FILTER_TAGS_REQUIRE"
	t.Run(env, func(t *testing.T) {
		defer cleanConfig()()
		assert := assert.New(t)
		err := os.Setenv(env, `important1 important2:value1`)
		assert.NoError(err)
		defer os.Unsetenv(env)
		cfg, err := LoadConfigFile("./testdata/full.yaml")
		assert.NoError(err)
		assert.Equal(cfg.RequireTags, []*config.Tag{{K: "important1", V: ""}, {K: "important2", V: "value1"}})
	})

	env = "DD_APM_FILTER_TAGS_REJECT"
	t.Run(env, func(t *testing.T) {
		defer cleanConfig()()
		assert := assert.New(t)
		err := os.Setenv(env, `bad1:value1`)
		assert.NoError(err)
		defer os.Unsetenv(env)
		cfg, err := LoadConfigFile("./testdata/full.yaml")
		assert.NoError(err)
		assert.Equal(cfg.RejectTags, []*config.Tag{{K: "bad1", V: "value1"}})
	})

	for _, envKey := range []string{
		"DD_CONNECTION_LIMIT", // deprecated
		"DD_APM_CONNECTION_LIMIT",
	} {
		t.Run(envKey, func(t *testing.T) {
			defer cleanConfig()()
			assert := assert.New(t)
			err := os.Setenv(envKey, "50")
			assert.NoError(err)
			defer os.Unsetenv(envKey)
			cfg, err := LoadConfigFile("./testdata/full.yaml")
			assert.NoError(err)
			assert.Equal(50, cfg.ConnectionLimit)
		})
	}

	for _, envKey := range []string{
		"DD_MAX_TPS", // deprecated
		"DD_APM_MAX_TPS",
	} {
		t.Run(envKey, func(t *testing.T) {
			defer cleanConfig()()
			assert := assert.New(t)
			err := os.Setenv(envKey, "6")
			assert.NoError(err)
			defer os.Unsetenv(envKey)
			cfg, err := LoadConfigFile("./testdata/full.yaml")
			assert.NoError(err)
			assert.Equal(6., cfg.TargetTPS)
		})
	}

	for _, envKey := range []string{
		"DD_APM_ERROR_TPS",
	} {
		t.Run(envKey, func(t *testing.T) {
			defer cleanConfig()()
			assert := assert.New(t)
			err := os.Setenv(envKey, "12")
			assert.NoError(err)
			defer os.Unsetenv(envKey)
			cfg, err := LoadConfigFile("./testdata/full.yaml")
			assert.NoError(err)
			assert.Equal(12., cfg.ErrorTPS)
		})
	}

	for _, envKey := range []string{
		"DD_APM_DISABLE_RARE_SAMPLER",
	} {
		t.Run(envKey, func(t *testing.T) {
			defer cleanConfig()()
			assert := assert.New(t)
			err := os.Setenv(envKey, "true")
			assert.NoError(err)
			defer os.Unsetenv(envKey)
			cfg, err := LoadConfigFile("./testdata/full.yaml")
			assert.NoError(err)
			assert.Equal(true, cfg.DisableRareSampler)
		})
	}

	for _, envKey := range []string{
		"DD_MAX_EPS", // deprecated
		"DD_APM_MAX_EPS",
	} {
		t.Run(envKey, func(t *testing.T) {
			defer cleanConfig()()
			assert := assert.New(t)
			err := os.Setenv(envKey, "7")
			assert.NoError(err)
			defer os.Unsetenv(envKey)
			cfg, err := LoadConfigFile("./testdata/full.yaml")
			assert.NoError(err)
			assert.Equal(7., cfg.MaxEPS)
		})
	}

	env = "DD_APM_MAX_REMOTE_TPS"
	t.Run(env, func(t *testing.T) {
		defer cleanConfig()()
		assert := assert.New(t)
		err := os.Setenv(env, "337.41")
		assert.NoError(err)
		defer os.Unsetenv(env)
		cfg, err := LoadConfigFile("./testdata/full.yaml")
		assert.NoError(err)
		assert.Equal(337.41, cfg.MaxRemoteTPS)
	})

	env = "DD_APM_ADDITIONAL_ENDPOINTS"
	t.Run(env, func(t *testing.T) {
		defer cleanConfig()()
		assert := assert.New(t)
		err := os.Setenv(env, `{"url1": ["key1", "key2"], "url2": ["key3"]}`)
		assert.NoError(err)
		defer os.Unsetenv(env)
		cfg, err := LoadConfigFile("./testdata/full.yaml")
		assert.NoError(err)
		assert.Contains(cfg.Endpoints, &config.Endpoint{APIKey: "key1", Host: "url1"})
		assert.Contains(cfg.Endpoints, &config.Endpoint{APIKey: "key2", Host: "url1"})
		assert.Contains(cfg.Endpoints, &config.Endpoint{APIKey: "key3", Host: "url2"})
	})

	env = "DD_APM_PROFILING_DD_URL"
	t.Run(env, func(t *testing.T) {
		defer cleanConfig()()
		assert := assert.New(t)
		err := os.Setenv(env, "my-site.com")
		assert.NoError(err)
		defer os.Unsetenv(env)
		_, err = LoadConfigFile("./testdata/full.yaml")
		assert.NoError(err)
		assert.Equal("my-site.com", coreconfig.Datadog.GetString("apm_config.profiling_dd_url"))
	})

	env = "DD_APM_DEBUGGER_DD_URL"
	t.Run(env, func(t *testing.T) {
		defer cleanConfig()()
		assert := assert.New(t)
		err := os.Setenv(env, "my-site.com")
		assert.NoError(err)
		defer os.Unsetenv(env)
		_, err = LoadConfigFile("./testdata/full.yaml")
		assert.NoError(err)
		assert.Equal("my-site.com", coreconfig.Datadog.GetString("apm_config.debugger_dd_url"))
	})

	env = "DD_APM_DEBUGGER_API_KEY"
	t.Run(env, func(t *testing.T) {
		defer cleanConfig()()
		assert := assert.New(t)
		err := os.Setenv(env, "my-key")
		assert.NoError(err)
		defer os.Unsetenv(env)
		_, err = LoadConfigFile("./testdata/full.yaml")
		assert.NoError(err)
		assert.Equal("my-key", coreconfig.Datadog.GetString("apm_config.debugger_api_key"))
	})

	env = "DD_APM_OBFUSCATION_CREDIT_CARDS_ENABLED"
	t.Run(env, func(t *testing.T) {
		defer cleanConfig()()
		assert := assert.New(t)
		err := os.Setenv(env, "false")
		assert.NoError(err)
		defer os.Unsetenv(env)
		_, err = LoadConfigFile("./testdata/full.yaml")
		assert.NoError(err)
		assert.False(coreconfig.Datadog.GetBool("apm_config.obfuscation.credit_cards.enabled"))
	})

	env = "DD_APM_OBFUSCATION_CREDIT_CARDS_LUHN"
	t.Run(env, func(t *testing.T) {
		defer cleanConfig()()
		assert := assert.New(t)
		err := os.Setenv(env, "false")
		assert.NoError(err)
		defer os.Unsetenv(env)
		_, err = LoadConfigFile("./testdata/full.yaml")
		assert.NoError(err)
		assert.False(coreconfig.Datadog.GetBool("apm_config.obfuscation.credit_cards.luhn"))
	})

	env = "DD_APM_PROFILING_ADDITIONAL_ENDPOINTS"
	t.Run(env, func(t *testing.T) {
		defer cleanConfig()()
		assert := assert.New(t)
		err := os.Setenv(env, `{"url1": ["key1", "key2"], "url2": ["key3"]}`)
		assert.NoError(err)
		defer os.Unsetenv(env)
		_, err = LoadConfigFile("./testdata/full.yaml")
		assert.NoError(err)
		expected := map[string][]string{
			"url1": {"key1", "key2"},
			"url2": {"key3"},
		}
		actual := coreconfig.Datadog.GetStringMapStringSlice(("apm_config.profiling_additional_endpoints"))
		if !reflect.DeepEqual(actual, expected) {
			t.Fatalf("Failed to process env var %s, expected %v and got %v", env, expected, actual)
		}
	})
}
