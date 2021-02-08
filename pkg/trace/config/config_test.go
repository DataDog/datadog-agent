// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package config

import (
	"os"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/DataDog/datadog-agent/pkg/config"
	"github.com/stretchr/testify/assert"
)

func cleanConfig() func() {
	oldConfig := config.Datadog
	config.Datadog = config.NewConfig("datadog", "DD", strings.NewReplacer(".", "_"))
	config.InitConfig(config.Datadog)
	return func() { config.Datadog = oldConfig }
}

func TestConfigHostname(t *testing.T) {
	t.Run("nothing", func(t *testing.T) {
		defer cleanConfig()()
		assert := assert.New(t)
		fallbackHostnameFunc = func() (string, error) {
			return "", nil
		}
		defer func() {
			fallbackHostnameFunc = os.Hostname
		}()
		_, err := Load("./testdata/site_override.yaml")
		assert.Equal(ErrMissingHostname, err)
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

func TestAcquireHostname(t *testing.T) {
	c := New()
	err := c.acquireHostname()
	assert.Nil(t, err)
	host, _ := os.Hostname()
	assert.Equal(t, host, c.Hostname)
}
