// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package config

import (
	"os"
	"reflect"
	"testing"

	"github.com/DataDog/datadog-agent/pkg/config"
	"github.com/cihub/seelog"
	"github.com/stretchr/testify/assert"
)

func TestMain(m *testing.M) {
	seelog.UseLogger(seelog.Disabled)
	os.Exit(m.Run())
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
			_, err = Load("./testdata/full.yaml")
			assert.NoError(err)
			if tt.envNew == "DD_APM_IGNORE_RESOURCES" {
				assert.Equal([]string{"4", "5", "6"}, config.Datadog.GetStringSlice(tt.key))
			} else {
				assert.Equal("4,5,6", config.Datadog.GetString(tt.key))
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
		cfg, err := Load("./testdata/full.yaml")
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
		cfg, err := Load("./testdata/undocumented.yaml")
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
		cfg, err := Load("./testdata/full.yaml")
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
		cfg, err := Load("./testdata/full.yaml")
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
		cfg, err := Load("./testdata/full.yaml")
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
		cfg, err := Load("./testdata/full.yaml")
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
		cfg, err := Load("./testdata/full.yaml")
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
		cfg, err := Load("./testdata/full.yaml")
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
			cfg, err := Load("./testdata/full.yaml")
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
		cfg, err := Load("./testdata/full.yaml")
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
		cfg, err := Load("./testdata/undocumented.yaml")
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
			cfg, err := Load("./testdata/full.yaml")
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
		cfg, err := Load("./testdata/full.yaml")
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
		cfg, err := Load("./testdata/full.yaml")
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
		cfg, err := Load("./testdata/full.yaml")
		assert.NoError(err)
		rule1 := &ReplaceRule{
			Name:    "name1",
			Pattern: "pattern1",
			Repl:    "",
		}
		rule2 := &ReplaceRule{
			Name:    "name2",
			Pattern: "pattern2",
			Repl:    "replace2",
		}
		compileReplaceRules([]*ReplaceRule{rule1, rule2})
		assert.Contains(cfg.ReplaceTags, rule1)
		assert.Contains(cfg.ReplaceTags, rule2)
	})

	env = "DD_APM_FILTER_TAGS"
	t.Run(env, func(t *testing.T) {
		defer cleanConfig()()
		assert := assert.New(t)
		err := os.Setenv(env, `[{"require": ["important1", "important2:value1"], "reject": ["bad1:value1"]}]`)
		assert.NoError(err)
		defer os.Unsetenv(env)
		cfg, err := Load("./testdata/full.yaml")
		assert.NoError(err)
		reqTags := make([]*Tag, 0)
		reqTags = append(reqTags, splitTag("important1"))
		reqTags = append(reqTags, splitTag("important2:value1"))
		rejTags := make([]*Tag, 0)
		rejTags = append(rejTags, splitTag("outcome:success"))
		assert.Contains(cfg.RequireTags, reqTags)
		assert.Contains(cfg.RejectTags, rejTags)
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
			cfg, err := Load("./testdata/full.yaml")
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
			cfg, err := Load("./testdata/full.yaml")
			assert.NoError(err)
			assert.Equal(6., cfg.TargetTPS)
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
			cfg, err := Load("./testdata/full.yaml")
			assert.NoError(err)
			assert.Equal(7., cfg.MaxEPS)
		})
	}

	env = "DD_APM_ADDITIONAL_ENDPOINTS"
	t.Run(env, func(t *testing.T) {
		defer cleanConfig()()
		assert := assert.New(t)
		err := os.Setenv(env, `{"url1": ["key1", "key2"], "url2": ["key3"]}`)
		assert.NoError(err)
		defer os.Unsetenv(env)
		cfg, err := Load("./testdata/full.yaml")
		assert.NoError(err)
		assert.Contains(cfg.Endpoints, &Endpoint{APIKey: "key1", Host: "url1"})
		assert.Contains(cfg.Endpoints, &Endpoint{APIKey: "key2", Host: "url1"})
		assert.Contains(cfg.Endpoints, &Endpoint{APIKey: "key3", Host: "url2"})
	})

	env = "DD_APM_PROFILING_DD_URL"
	t.Run(env, func(t *testing.T) {
		defer cleanConfig()()
		assert := assert.New(t)
		err := os.Setenv(env, "my-site.com")
		assert.NoError(err)
		defer os.Unsetenv(env)
		_, err = Load("./testdata/full.yaml")
		assert.NoError(err)
		assert.Equal("my-site.com", config.Datadog.GetString("apm_config.profiling_dd_url"))
	})

	env = "DD_APM_PROFILING_ADDITIONAL_ENDPOINTS"
	t.Run(env, func(t *testing.T) {
		defer cleanConfig()()
		assert := assert.New(t)
		err := os.Setenv(env, `{"url1": ["key1", "key2"], "url2": ["key3"]}`)
		assert.NoError(err)
		defer os.Unsetenv(env)
		_, err = Load("./testdata/full.yaml")
		assert.NoError(err)
		expected := map[string][]string{
			"url1": {"key1", "key2"},
			"url2": {"key3"},
		}
		actual := config.Datadog.GetStringMapStringSlice(("apm_config.profiling_additional_endpoints"))
		if !reflect.DeepEqual(actual, expected) {
			t.Fatalf("Failed to process env var %s, expected %v and got %v", env, expected, actual)
		}
	})
}
