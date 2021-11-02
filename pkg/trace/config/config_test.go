// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package config

import (
	"errors"
	"io/ioutil"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
	"text/template"
	"time"

	"github.com/DataDog/datadog-agent/pkg/config"
	"github.com/DataDog/datadog-agent/pkg/trace/config/features"
	"github.com/stretchr/testify/assert"
)

func cleanConfig() func() {
	oldConfig := config.Datadog
	config.Datadog = config.NewConfig("datadog", "DD", strings.NewReplacer(".", "_"))
	config.InitConfig(config.Datadog)
	return func() { config.Datadog = oldConfig }
}

func TestConfigHostname(t *testing.T) {
	t.Run("fail", func(t *testing.T) {
		defer cleanConfig()()
		config.Datadog.Set("apm_config.dd_agent_bin", "/not/exist")
		assert := assert.New(t)
		fallbackHostnameFunc = func() (string, error) {
			return "", errors.New("could not get hostname")
		}
		defer func() {
			fallbackHostnameFunc = os.Hostname
		}()
		_, err := Load("./testdata/site_override.yaml")
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
		cfg, err := Load("./testdata/site_override.yaml")
		assert.NoError(err)
		assert.Equal(host, cfg.Hostname)
	})

	t.Run("file", func(t *testing.T) {
		defer cleanConfig()()
		assert := assert.New(t)
		cfg, err := Load("./testdata/full.yaml")
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
		cfg, err := Load("./testdata/site_override.yaml")
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
		cfg, err := Load("./testdata/full.yaml")
		assert.NoError(err)
		assert.Equal("envoverride", cfg.Hostname)
	})

	t.Run("external", func(t *testing.T) {
		body, err := ioutil.ReadFile("../test/fixtures/stringcode.go.tmpl")
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
			cfg := AgentConfig{DDAgentBin: makeProgram("host.name", 0)}
			defer os.Remove(cfg.DDAgentBin)
			assert.NoError(t, cfg.acquireHostnameFallback())
			assert.Equal(t, cfg.Hostname, "host.name")
		})

		t.Run("empty", func(t *testing.T) {
			cfg := AgentConfig{DDAgentBin: makeProgram("", 0)}
			defer os.Remove(cfg.DDAgentBin)
			assert.NoError(t, cfg.acquireHostnameFallback())
			assert.Empty(t, cfg.Hostname)
		})

		t.Run("empty+disallowed", func(t *testing.T) {
			features.Set("disable_empty_hostname")
			defer func() { features.Set(os.Getenv("DD_APM_FEATURES")) }()

			cfg := AgentConfig{DDAgentBin: makeProgram("", 0)}
			defer os.Remove(cfg.DDAgentBin)
			assert.NoError(t, cfg.acquireHostnameFallback())
			assert.Equal(t, "fallback.host", cfg.Hostname)
		})

		t.Run("fallback1", func(t *testing.T) {
			cfg := AgentConfig{DDAgentBin: makeProgram("", 1)}
			defer os.Remove(cfg.DDAgentBin)
			assert.NoError(t, cfg.acquireHostnameFallback())
			assert.Equal(t, cfg.Hostname, "fallback.host")
		})

		t.Run("fallback2", func(t *testing.T) {
			cfg := AgentConfig{DDAgentBin: makeProgram("some text", 1)}
			defer os.Remove(cfg.DDAgentBin)
			assert.NoError(t, cfg.acquireHostnameFallback())
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
			cfg, err := Load(tt.file)
			assert.NoError(t, err)
			assert.Equal(t, tt.url, cfg.Endpoints[0].Host)
		})
	}
}

func TestDefaultConfig(t *testing.T) {
	assert := assert.New(t)
	c := New()

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
	assert.NoError(c.applyDatadogConfig())

	assert.Equal("thing", c.Hostname)
	assert.Equal("apikey_12", c.Endpoints[0].APIKey)
	assert.Equal("0.0.0.0", c.ReceiverHost)
	assert.Equal(28125, c.StatsdPort)
	assert.Equal("DEBUG", c.LogLevel)
}

func TestFullYamlConfig(t *testing.T) {
	defer cleanConfig()()
	origcfg := config.Datadog
	config.Datadog = config.NewConfig("datadog", "DD", strings.NewReplacer(".", "_"))
	defer func() {
		config.Datadog = origcfg
	}()

	assert := assert.New(t)

	c, err := prepareConfig("./testdata/full.yaml")
	assert.NoError(err)
	assert.NoError(c.applyDatadogConfig())

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

	noProxy := true
	if _, ok := os.LookupEnv("NO_PROXY"); ok {
		// Happens in CircleCI: if the environment variable is set,
		// it will overwrite our loaded configuration and will cause
		// this test to fail.
		noProxy = false
	}

	assert.ElementsMatch([]*Endpoint{
		{Host: "https://datadog.unittests", APIKey: "api_key_test"},
		{Host: "https://my1.endpoint.com", APIKey: "apikey1"},
		{Host: "https://my1.endpoint.com", APIKey: "apikey2"},
		{Host: "https://my2.endpoint.eu", APIKey: "apikey3", NoProxy: noProxy},
		{Host: "https://my2.endpoint.eu", APIKey: "apikey4", NoProxy: noProxy},
		{Host: "https://my2.endpoint.eu", APIKey: "apikey5", NoProxy: noProxy},
	}, c.Endpoints)

	assert.ElementsMatch([]*Tag{{K: "env", V: "prod"}, {K: "db", V: "mongodb"}}, c.RequireTags)
	assert.ElementsMatch([]*Tag{{K: "outcome", V: "success"}}, c.RejectTags)

	assert.ElementsMatch([]*ReplaceRule{
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

	assert.Equal("0.0.0.0", c.OTLPReceiver.BindHost)
	assert.Equal(0, c.OTLPReceiver.HTTPPort)
	assert.Equal(50053, c.OTLPReceiver.GRPCPort)

	o := c.Obfuscation
	assert.NotNil(o)
	assert.True(o.ES.Enabled)
	assert.EqualValues([]string{"user_id", "category_id"}, o.ES.KeepValues)
	assert.True(o.Mongo.Enabled)
	assert.EqualValues([]string{"uid", "cat_id"}, o.Mongo.KeepValues)
	assert.True(o.HTTP.RemoveQueryString)
	assert.True(o.HTTP.RemovePathDigits)
	assert.True(o.RemoveStackTraces)
	assert.True(c.Obfuscation.Redis.Enabled)
	assert.True(c.Obfuscation.Memcached.Enabled)
	assert.True(c.Obfuscation.CreditCards.Enabled)
	assert.True(c.Obfuscation.CreditCards.Luhn)
}

func TestUndocumentedYamlConfig(t *testing.T) {
	defer cleanConfig()()
	origcfg := config.Datadog
	config.Datadog = config.NewConfig("datadog", "DD", strings.NewReplacer(".", "_"))
	defer func() {
		config.Datadog = origcfg
	}()
	assert := assert.New(t)

	c, err := prepareConfig("./testdata/undocumented.yaml")
	assert.NoError(err)
	assert.NoError(c.applyDatadogConfig())

	assert.Equal("/path/to/bin", c.DDAgentBin)
	assert.Equal("thing", c.Hostname)
	assert.Equal("apikey_12", c.Endpoints[0].APIKey)
	assert.Equal(0.33, c.ExtraSampleRate)
	assert.Equal(100.0, c.TargetTPS)
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
	c := New()
	err := c.acquireHostnameFallback()
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
			cfg, err := Load("./testdata/no_apm_config.yaml")
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
			cfg, err := Load("./testdata/no_apm_config.yaml")
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
			cfg, err := Load(cfgFile)
			assert.NoError(err)
			assert.Equal("staging", cfg.DefaultEnv)
		})
	}
}
