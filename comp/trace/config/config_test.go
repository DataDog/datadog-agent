// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package config

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"regexp"
	"strings"
	"testing"
	"text/template"
	"time"

	"github.com/cihub/seelog"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/fx"
	"gopkg.in/yaml.v2"

	corecomp "github.com/DataDog/datadog-agent/comp/core/config"
	"github.com/DataDog/datadog-agent/comp/core/tagger"
	"github.com/DataDog/datadog-agent/comp/core/tagger/taggerimpl"
	apiutil "github.com/DataDog/datadog-agent/pkg/api/util"
	"github.com/DataDog/datadog-agent/pkg/config/env"
	pkgconfigsetup "github.com/DataDog/datadog-agent/pkg/config/setup"

	traceconfig "github.com/DataDog/datadog-agent/pkg/trace/config"
	"github.com/DataDog/datadog-agent/pkg/util/fxutil"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

// team: agent-apm

// TestParseReplaceRules tests the compileReplaceRules helper function.
func TestParseRepaceRules(t *testing.T) {
	assert := assert.New(t)
	rules := []*traceconfig.ReplaceRule{
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

// TestSplitTag tests various split-tagging scenarios
func TestSplitTag(t *testing.T) {
	for _, tt := range []struct {
		tag string
		kv  *traceconfig.Tag
	}{
		{
			tag: "",
			kv:  &traceconfig.Tag{K: ""},
		},
		{
			tag: "key:value",
			kv:  &traceconfig.Tag{K: "key", V: "value"},
		},
		{
			tag: "env:prod",
			kv:  &traceconfig.Tag{K: "env", V: "prod"},
		},
		{
			tag: "env:staging:east",
			kv:  &traceconfig.Tag{K: "env", V: "staging:east"},
		},
		{
			tag: "key",
			kv:  &traceconfig.Tag{K: "key"},
		},
	} {
		t.Run("", func(t *testing.T) {
			assert.Equal(t, splitTag(tt.tag), tt.kv)
		})
	}
}

func TestSplitTagRegex(t *testing.T) {
	for _, tt := range []struct {
		tag string
		kv  *traceconfig.TagRegex
	}{
		{
			tag: "",
			kv:  &traceconfig.TagRegex{K: ""},
		},
		{
			tag: "key:^value$",
			kv:  &traceconfig.TagRegex{K: "key", V: regexp.MustCompile("^value$")},
		},
		{
			tag: "env:^prod123$",
			kv:  &traceconfig.TagRegex{K: "env", V: regexp.MustCompile("^prod123$")},
		},
		{
			tag: "env:^staging:east.*$",
			kv:  &traceconfig.TagRegex{K: "env", V: regexp.MustCompile("^staging:east.*$")},
		},
		{
			tag: "key",
			kv:  &traceconfig.TagRegex{K: "key"},
		},
	} {
		t.Run("normal", func(t *testing.T) {
			assert.Equal(t, splitTagRegex(tt.tag), tt.kv)
		})
	}
	bad := struct {
		tag string
		kv  *traceconfig.Tag
	}{

		tag: "key:[value",
		kv:  nil,
	}

	t.Run("error", func(t *testing.T) {
		var b bytes.Buffer
		w := bufio.NewWriter(&b)

		logger, err := seelog.LoggerFromWriterWithMinLevelAndFormat(w, seelog.DebugLvl, "[%LEVEL] %Msg")
		assert.Nil(t, err)
		seelog.ReplaceLogger(logger) //nolint:errcheck
		log.SetupLogger(logger, "debug")
		assert.Nil(t, splitTagRegex(bad.tag))
		w.Flush()
		assert.Contains(t, b.String(), "[ERROR] Invalid regex pattern in tag filter: \"key\":\"[value\"")
	})
}

func TestTelemetryEndpointsConfig(t *testing.T) {
	t.Run("default", func(t *testing.T) {
		config := buildConfigComponent(t)
		cfg := config.Object()
		require.NotNil(t, cfg)

		assert.True(t, cfg.TelemetryConfig.Enabled)
		assert.Len(t, cfg.TelemetryConfig.Endpoints, 1)
		assert.Equal(t, "https://instrumentation-telemetry-intake.datadoghq.com", cfg.TelemetryConfig.Endpoints[0].Host)
	})

	t.Run("dd_url", func(t *testing.T) {
		overrides := map[string]interface{}{
			"apm_config.telemetry.dd_url": "http://example.com/",
		}

		config := buildConfigComponent(t, fx.Replace(corecomp.MockParams{Overrides: overrides}))
		cfg := config.Object()
		require.NotNil(t, cfg)

		assert.True(t, cfg.TelemetryConfig.Enabled)
		assert.Len(t, cfg.TelemetryConfig.Endpoints, 1)
		assert.Equal(t, "http://example.com/", cfg.TelemetryConfig.Endpoints[0].Host)
	})

	t.Run("dd_url-malformed", func(t *testing.T) {
		overrides := map[string]interface{}{
			"apm_config.telemetry.dd_url": "111://abc.com",
		}

		config := buildConfigComponent(t, fx.Replace(corecomp.MockParams{Overrides: overrides}))

		cfg := config.Object()
		require.NotNil(t, cfg)

		assert.True(t, cfg.TelemetryConfig.Enabled)
		assert.Len(t, cfg.TelemetryConfig.Endpoints, 1)
		assert.Equal(t, cfg.TelemetryConfig.Endpoints[0].Host, "111://abc.com")
	})

	t.Run("site", func(t *testing.T) {
		overrides := map[string]interface{}{
			"site": "new_site.example.com",
		}

		config := buildConfigComponent(t, fx.Replace(corecomp.MockParams{Overrides: overrides}))
		cfg := config.Object()
		require.NotNil(t, cfg)

		assert.True(t, cfg.TelemetryConfig.Enabled)
		assert.Len(t, cfg.TelemetryConfig.Endpoints, 1)
		assert.Equal(t, "https://instrumentation-telemetry-intake.new_site.example.com", cfg.TelemetryConfig.Endpoints[0].Host)
	})

	t.Run("additional-hosts", func(t *testing.T) {
		additionalEndpoints := map[string]string{
			"http://test_backend_2.example.com": "test_apikey_2",
			"http://test_backend_3.example.com": "test_apikey_3",
		}
		overrides := map[string]interface{}{
			"apm_config.telemetry.additional_endpoints": additionalEndpoints,
		}

		config := buildConfigComponent(t, fx.Replace(corecomp.MockParams{Overrides: overrides}))
		cfg := config.Object()
		require.NotNil(t, cfg)

		assert.True(t, cfg.TelemetryConfig.Enabled)
		assert.Equal(t, "https://instrumentation-telemetry-intake.datadoghq.com", cfg.TelemetryConfig.Endpoints[0].Host)

		assert.Len(t, cfg.TelemetryConfig.Endpoints, 3)
		for _, endpoint := range cfg.TelemetryConfig.Endpoints[1:] {
			assert.NotNil(t, additionalEndpoints[endpoint.Host])
			assert.Equal(t, endpoint.APIKey, additionalEndpoints[endpoint.Host])
		}
	})

	t.Run("keep-malformed", func(t *testing.T) {
		additionalEndpoints := map[string]string{
			"11://test_backend_2.example.com":   "test_apikey_2",
			"http://test_backend_3.example.com": "test_apikey_3",
		}
		overrides := map[string]interface{}{
			"apm_config.telemetry.additional_endpoints": additionalEndpoints,
		}

		config := buildConfigComponent(t, fx.Replace(corecomp.MockParams{Overrides: overrides}))
		cfg := config.Object()
		require.NotNil(t, cfg)

		assert.True(t, cfg.TelemetryConfig.Enabled)
		assert.Equal(t, "https://instrumentation-telemetry-intake.datadoghq.com", cfg.TelemetryConfig.Endpoints[0].Host)

		assert.Len(t, cfg.TelemetryConfig.Endpoints, 3)
		for _, endpoint := range cfg.TelemetryConfig.Endpoints[1:] {
			assert.NotNil(t, additionalEndpoints[endpoint.Host])
			assert.Equal(t, endpoint.APIKey, additionalEndpoints[endpoint.Host])
		}
	})
}

func TestConfigHostname(t *testing.T) {
	t.Run("fail", func(t *testing.T) {
		overrides := map[string]interface{}{
			"apm_config.dd_agent_bin": "/not/exist",
			"cmd_port":                "-1",
		}

		fallbackHostnameFunc = func() (string, error) {
			return "", errors.New("could not get hostname")
		}
		defer func() {
			fallbackHostnameFunc = os.Hostname
		}()

		taggerComponent := fxutil.Test[tagger.Mock](t, taggerimpl.MockModule())

		fxutil.TestStart(t, fx.Options(
			corecomp.MockModule(),
			fx.Replace(corecomp.MockParams{
				Params:    corecomp.Params{ConfFilePath: "./testdata/site_override.yaml"},
				Overrides: overrides,
			}),
			fx.Provide(func() tagger.Component {
				return taggerComponent
			}),
			MockModule(),
		),
			func(t testing.TB, app *fx.App) {
				require.NotNil(t, app)

				ctx := context.Background()
				err := app.Start(ctx)
				defer app.Stop(ctx)

				require.NotNil(t, err)
				assert.Contains(t, err.Error(), "nor from OS")

			}, func(_ Component) {
				// nothing
			})
	})

	t.Run("fallback", func(t *testing.T) {
		overrides := map[string]interface{}{
			"apm_config.dd_agent_bin": "/not/exist",
			"cmd_port":                "-1",
		}
		host, err := os.Hostname()
		if err != nil || host == "" {
			// can't say
			t.Skip()
		}

		config := buildConfigComponent(t, fx.Replace(corecomp.MockParams{
			Params:    corecomp.Params{ConfFilePath: "./testdata/site_override.yaml"},
			Overrides: overrides,
		}))

		cfg := config.Object()

		require.NotNil(t, cfg)
		assert.Equal(t, host, cfg.Hostname)
	})

	t.Run("file", func(t *testing.T) {
		config := buildConfigComponent(t, fx.Replace(corecomp.MockParams{
			Params: corecomp.Params{ConfFilePath: "./testdata/full.yaml"},
		}))

		cfg := config.Object()

		require.NotNil(t, cfg)
		assert.Equal(t, "mymachine", cfg.Hostname)
	})

	t.Run("env", func(t *testing.T) {
		t.Setenv("DD_HOSTNAME", "onlyenv")

		config := buildConfigComponent(t, fx.Replace(corecomp.MockParams{
			Params: corecomp.Params{ConfFilePath: "./testdata/site_override.yaml"},
		}))
		cfg := config.Object()

		require.NotNil(t, cfg)
		assert.Equal(t, "onlyenv", cfg.Hostname)
	})

	t.Run("file+env", func(t *testing.T) {
		t.Setenv("DD_HOSTNAME", "envoverride")

		config := buildConfigComponent(t, fx.Replace(corecomp.MockParams{
			Params: corecomp.Params{ConfFilePath: "./testdata/full.yaml"},
		}))
		cfg := config.Object()

		require.NotNil(t, cfg)
		assert.Equal(t, "envoverride", cfg.Hostname)
	})

	t.Run("serverless", func(t *testing.T) {
		overrides := map[string]interface{}{
			"serverless.enabled": true,
		}

		config := buildConfigComponent(t, fx.Replace(corecomp.MockParams{
			Params:    corecomp.Params{ConfFilePath: "./testdata/site_default.yaml"},
			Overrides: overrides,
		}))
		cfg := config.Object()

		require.NotNil(t, cfg)
		assert.Equal(t, "", cfg.Hostname)
	})

	t.Run("external", func(t *testing.T) {
		body, err := os.ReadFile("testdata/stringcode.go.tmpl")
		if err != nil {
			t.Fatal(err)
		}

		// makeProgram creates a new binary file which returns the given response and exits to the OS
		// given the specified code, returning the path of the program.
		makeProgram := func(response string, code int) string {
			f, err := os.CreateTemp("", "trace-test-hostname.*.go")
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
			bin := makeProgram("host.name", 0)
			defer os.Remove(bin)

			config := buildConfigComponent(t)
			cfg := config.Object()
			require.NotNil(t, cfg)

			cfg.DDAgentBin = bin
			assert.NoError(t, acquireHostnameFallback(cfg))
			assert.Equal(t, cfg.Hostname, "host.name")

		})

		t.Run("empty", func(t *testing.T) {
			bin := makeProgram("", 0)
			defer os.Remove(bin)

			config := buildConfigComponent(t)
			cfg := config.Object()
			require.NotNil(t, cfg)

			cfg.DDAgentBin = bin
			assert.NoError(t, acquireHostnameFallback(cfg))
			assert.Empty(t, cfg.Hostname)
		})

		t.Run("empty+disallowed", func(t *testing.T) {
			bin := makeProgram("", 0)
			defer os.Remove(bin)

			config := buildConfigComponent(t)

			cfg := config.Object()
			require.NotNil(t, cfg)

			cfg.DDAgentBin = bin
			cfg.Features = map[string]struct{}{"disable_empty_hostname": {}}
			assert.NoError(t, acquireHostnameFallback(cfg))
			assert.Equal(t, "fallback.host", cfg.Hostname)
		})

		t.Run("fallback1", func(t *testing.T) {
			bin := makeProgram("", 1)
			defer os.Remove(bin)

			config := buildConfigComponent(t)
			cfg := config.Object()
			require.NotNil(t, cfg)

			cfg.DDAgentBin = bin
			assert.NoError(t, acquireHostnameFallback(cfg))
			assert.Equal(t, cfg.Hostname, "fallback.host")
		})

		t.Run("fallback2", func(t *testing.T) {
			bin := makeProgram("some text", 1)
			defer os.Remove(bin)

			config := buildConfigComponent(t)
			cfg := config.Object()
			require.NotNil(t, cfg)

			cfg.DDAgentBin = bin
			assert.NoError(t, acquireHostnameFallback(cfg))
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
		"vector":   {"./testdata/observability_pipelines_worker_override.yaml", "https://observability_pipelines_worker.domain.tld:8443"},
	} {
		t.Run(name, func(t *testing.T) {
			config := buildConfigComponent(t, fx.Replace(corecomp.MockParams{
				Params: corecomp.Params{ConfFilePath: tt.file},
			}))
			cfg := config.Object()

			require.NotNil(t, cfg)
			assert.Equal(t, tt.url, cfg.Endpoints[0].Host)
		})
	}
}

func TestDefaultConfig(t *testing.T) {
	config := buildConfigComponent(t)
	cfg := config.Object()

	require.NotNil(t, cfg)

	// assert that some sane defaults are set
	assert.Equal(t, "localhost", cfg.ReceiverHost)
	assert.Equal(t, 8126, cfg.ReceiverPort)

	assert.Equal(t, "localhost", cfg.StatsdHost)
	assert.Equal(t, 8125, cfg.StatsdPort)
	assert.Equal(t, true, cfg.StatsdEnabled)

	assert.Equal(t, true, cfg.Enabled)

	assert.False(t, cfg.InstallSignature.Found)

	assert.True(t, cfg.ReceiverEnabled)
}

func TestNoAPMConfig(t *testing.T) {
	config := buildConfigComponent(t, fx.Replace(corecomp.MockParams{
		Params: corecomp.Params{ConfFilePath: "./testdata/no_apm_config.yaml"},
	}))
	cfg := config.Object()

	require.NotNil(t, cfg)

	assert.Equal(t, "thing", cfg.Hostname)
	assert.Equal(t, "apikey_12", cfg.Endpoints[0].APIKey)
	assert.Equal(t, "0.0.0.0", cfg.ReceiverHost)
	assert.Equal(t, 28125, cfg.StatsdPort)
}

func TestDisableLoggingConfig(t *testing.T) {
	config := buildConfigComponent(t, fx.Replace(corecomp.MockParams{
		Params: corecomp.Params{ConfFilePath: "./testdata/disable_file_logging.yaml"},
	}))
	cfg := config.Object()

	require.NotNil(t, cfg)

	assert.Equal(t, "", cfg.LogFilePath)
}

func TestFullYamlConfig(t *testing.T) {
	config := buildConfigComponent(t, fx.Replace(corecomp.MockParams{
		Params: corecomp.Params{ConfFilePath: "./testdata/full.yaml"},
	}))
	cfg := config.Object()

	require.NotNil(t, cfg)
	req, err := http.NewRequest(http.MethodGet, "https://someplace.test", nil)
	assert.NoError(t, err)
	proxyURL, err := cfg.Proxy(req)
	assert.NoError(t, err)

	require.NotNil(t, proxyURL)
	assert.Equal(t, "proxy_for_https:1234", proxyURL.Host)

	assert.Equal(t, "mymachine", cfg.Hostname)
	assert.Equal(t, "https://user:password@proxy_for_https:1234", cfg.ProxyURL.String())
	assert.True(t, cfg.SkipSSLValidation)
	assert.Equal(t, 18125, cfg.StatsdPort)
	assert.False(t, cfg.Enabled)
	assert.Equal(t, "abc", cfg.LogFilePath)
	assert.Equal(t, "test", cfg.DefaultEnv)
	assert.Equal(t, 123, cfg.ConnectionLimit)
	assert.Equal(t, 18126, cfg.ReceiverPort)
	assert.Equal(t, 0.5, cfg.ExtraSampleRate)
	assert.Equal(t, 5.0, cfg.TargetTPS)
	assert.Equal(t, 50.0, cfg.MaxEPS)
	assert.Equal(t, 0.5, cfg.MaxCPU)
	assert.EqualValues(t, 123.4, cfg.MaxMemory)
	assert.Equal(t, "0.0.0.0", cfg.ReceiverHost)
	assert.True(t, cfg.OTLPReceiver.SpanNameAsResourceName)
	assert.Equal(t, map[string]string{"a": "b", "and:colons": "in:values", "c": "d", "with.dots": "in.side"}, cfg.OTLPReceiver.SpanNameRemappings)

	noProxy := true
	if _, ok := os.LookupEnv("NO_PROXY"); ok {
		// Happens in CircleCI: if the environment variable is set,
		// it will overwrite our loaded configuration and will cause
		// this test to fail.
		noProxy = false
	}
	assert.ElementsMatch(t, []*traceconfig.Endpoint{
		{Host: "https://datadog.unittests", APIKey: "api_key_test"},
		{Host: "https://my1.endpoint.com", APIKey: "apikey1"},
		{Host: "https://my1.endpoint.com", APIKey: "apikey2"},
		{Host: "https://my2.endpoint.eu", APIKey: "apikey3", NoProxy: noProxy},
		{Host: "https://my2.endpoint.eu", APIKey: "apikey4", NoProxy: noProxy},
		{Host: "https://my2.endpoint.eu", APIKey: "apikey5", NoProxy: noProxy},
	}, cfg.Endpoints)

	assert.ElementsMatch(t, []*traceconfig.Tag{{K: "env", V: "prod"}, {K: "db", V: "mongodb"}}, cfg.RequireTags)
	assert.ElementsMatch(t, []*traceconfig.Tag{{K: "outcome", V: "success"}, {K: "bad-key", V: "bad-value"}}, cfg.RejectTags)
	assert.ElementsMatch(t, []*traceconfig.TagRegex{{K: "type", V: regexp.MustCompile("^internal$")}}, cfg.RequireTagsRegex)
	assert.ElementsMatch(t, []*traceconfig.TagRegex{{K: "filter", V: regexp.MustCompile("^true$")}}, cfg.RejectTagsRegex)

	assert.ElementsMatch(t, []*traceconfig.ReplaceRule{
		{
			Name:    "http.method",
			Pattern: "\\?.*$",
			Repl:    "GET",
			Re:      regexp.MustCompile(`\?.*$`),
		},
		{
			Name:    "http.url",
			Pattern: "\\?.*$",
			Repl:    "!",
			Re:      regexp.MustCompile(`\?.*$`),
		},
		{
			Name:    "error.stack",
			Pattern: "(?s).*",
			Repl:    "?",
			Re:      regexp.MustCompile("(?s).*"),
		},
	}, cfg.ReplaceTags)

	assert.EqualValues(t, []string{"/health", "/500"}, cfg.Ignore["resource"])

	o := cfg.Obfuscation
	assert.NotNil(t, o)
	assert.True(t, o.ES.Enabled)
	assert.EqualValues(t, []string{"user_id", "category_id"}, o.ES.KeepValues)
	assert.True(t, o.Mongo.Enabled)
	assert.EqualValues(t, []string{"uid", "cat_id"}, o.Mongo.KeepValues)
	assert.True(t, o.HTTP.RemoveQueryString)
	assert.True(t, o.HTTP.RemovePathDigits)
	assert.True(t, o.RemoveStackTraces)
	assert.True(t, o.Redis.Enabled)
	assert.True(t, o.Memcached.Enabled)
	assert.True(t, o.Memcached.KeepCommand)
	assert.True(t, o.CreditCards.Enabled)
	assert.True(t, o.CreditCards.Luhn)

	assert.True(t, cfg.InstallSignature.Found)
	assert.Equal(t, traceconfig.InstallSignatureConfig{
		Found:       true,
		InstallID:   "00000014-7fcf-21ee-a501-a69841f17276",
		InstallType: "manual",
		InstallTime: 1699623821,
	}, cfg.InstallSignature)
}

func TestFileLoggingDisabled(t *testing.T) {
	config := buildConfigComponent(t, fx.Replace(corecomp.MockParams{
		Params: corecomp.Params{ConfFilePath: "./testdata/disable_file_logging.yaml"},
	}))

	cfg := config.Object()

	require.NotNil(t, cfg)
	assert.Equal(t, "", cfg.LogFilePath)
}

func TestUndocumentedYamlConfig(t *testing.T) {
	config := buildConfigComponent(t, fx.Replace(corecomp.MockParams{
		Params: corecomp.Params{ConfFilePath: "./testdata/undocumented.yaml"},
	}))
	cfg := config.Object()

	require.NotNil(t, cfg)

	assert.Equal(t, "/path/to/bin", cfg.DDAgentBin)
	assert.Equal(t, "thing", cfg.Hostname)
	assert.Equal(t, "apikey_12", cfg.Endpoints[0].APIKey)
	assert.Equal(t, 0.33, cfg.ExtraSampleRate)
	assert.Equal(t, 100.0, cfg.TargetTPS)
	assert.Equal(t, 37.0, cfg.ErrorTPS)
	assert.Equal(t, true, cfg.RareSamplerEnabled)
	assert.Equal(t, 127.0, cfg.MaxRemoteTPS)
	assert.Equal(t, 1000.0, cfg.MaxEPS)
	assert.Equal(t, 25, cfg.ReceiverPort)
	assert.Equal(t, 120*time.Second, cfg.ConnectionResetInterval)
	// watchdog
	assert.Equal(t, 0.07, cfg.MaxCPU)
	assert.Equal(t, 30e6, cfg.MaxMemory)

	// Assert Trace Writer
	assert.Equal(t, 1, cfg.TraceWriter.ConnectionLimit)
	assert.Equal(t, 2, cfg.TraceWriter.QueueSize)
	assert.Equal(t, 5, cfg.StatsWriter.ConnectionLimit)
	assert.Equal(t, 6, cfg.StatsWriter.QueueSize)
	// analysis legacy
	assert.Equal(t, 1.0, cfg.AnalyzedRateByServiceLegacy["db"])
	assert.Equal(t, 0.9, cfg.AnalyzedRateByServiceLegacy["web"])
	assert.Equal(t, 0.5, cfg.AnalyzedRateByServiceLegacy["index"])
	// analysis
	assert.Len(t, cfg.AnalyzedSpansByService, 2)
	assert.Len(t, cfg.AnalyzedSpansByService["web"], 2)
	assert.Len(t, cfg.AnalyzedSpansByService["db"], 1)
	assert.Equal(t, 0.8, cfg.AnalyzedSpansByService["web"]["request"])
	assert.Equal(t, 0.9, cfg.AnalyzedSpansByService["web"]["django.request"])
	assert.Equal(t, 0.05, cfg.AnalyzedSpansByService["db"]["intake"])

}

func TestAcquireHostnameFallback(t *testing.T) {
	c := traceconfig.New()
	err := acquireHostnameFallback(c)
	assert.Nil(t, err)
	host, _ := os.Hostname()
	assert.Equal(t, host, c.Hostname)
}

func TestNormalizeEnvFromDDEnv(t *testing.T) {
	for in, out := range map[string]string{
		"staging":   "staging",
		"stAging":   "staging",
		"staging 1": "staging_1",
	} {
		t.Run("", func(t *testing.T) {
			t.Setenv("DD_ENV", in)

			config := buildConfigComponent(t, fx.Replace(corecomp.MockParams{
				Params: corecomp.Params{ConfFilePath: "./testdata/no_apm_config.yaml"},
			}))
			cfg := config.Object()

			assert.NotNil(t, cfg)

			assert.Equal(t, out, cfg.DefaultEnv)
		})
	}
}

func TestNormalizeEnvFromDDTags(t *testing.T) {
	for in, out := range map[string]string{
		"env:staging": "staging",
		"env:stAging": "staging",
		// The value of DD_TAGS is parsed with a space delimiter.
		"tag:value env:STAGING tag2:value2": "staging",
	} {
		t.Run("", func(t *testing.T) {
			t.Setenv("DD_TAGS", in)

			config := buildConfigComponent(t, fx.Replace(corecomp.MockParams{
				Params: corecomp.Params{ConfFilePath: "./testdata/no_apm_config.yaml"},
			}))
			cfg := config.Object()

			assert.NotNil(t, cfg)

			assert.Equal(t, out, cfg.DefaultEnv)
		})
	}
}

func TestNormalizeEnvFromConfig(t *testing.T) {
	for _, cfgFile := range []string{
		"./testdata/ok_env_apm_config.yaml",
		"./testdata/ok_env_top_level.yaml",
		"./testdata/ok_env_host_tag.yaml",
		"./testdata/non-normalized_env_apm_config.yaml",
		"./testdata/non-normalized_env_top_level.yaml",
		"./testdata/non-normalized_env_host_tag.yaml",
	} {
		t.Run("", func(t *testing.T) {
			config := buildConfigComponent(t, fx.Replace(corecomp.MockParams{
				Params: corecomp.Params{ConfFilePath: cfgFile},
			}))

			cfg := config.Object()

			assert.NotNil(t, cfg)
			assert.Equal(t, "staging", cfg.DefaultEnv)
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
			{"DD_APM_MAX_TPS", "DD_APM_TARGET_TPS", "apm_config.target_traces_per_second"},
			{"DD_IGNORE_RESOURCE", "DD_APM_IGNORE_RESOURCES", "apm_config.ignore_resources"},
		} {
			t.Setenv(tt.envOld, "1,2,3")
			t.Setenv(tt.envNew, "4,5,6")

			config := buildConfigComponent(t, fx.Replace(corecomp.MockParams{
				Params: corecomp.Params{ConfFilePath: "./testdata/full.yaml"},
			}))

			cfg := config.Object()

			assert.NotNil(t, cfg)
			if tt.envNew == "DD_APM_IGNORE_RESOURCES" {
				assert.Equal(t, []string{"4", "5", "6"}, pkgconfigsetup.Datadog().GetStringSlice(tt.key))
			} else {
				assert.Equal(t, "4,5,6", pkgconfigsetup.Datadog().GetString(tt.key))
			}
		}
	})

	env := "DD_API_KEY"
	t.Run(env, func(t *testing.T) {
		t.Setenv(env, "123")

		config := buildConfigComponent(t, fx.Replace(corecomp.MockParams{
			Params: corecomp.Params{ConfFilePath: "./testdata/full.yaml"},
		}))
		cfg := config.Object()

		assert.NotNil(t, cfg)
		assert.Equal(t, "123", cfg.Endpoints[0].APIKey)
	})

	env = "DD_SITE"
	t.Run(env, func(t *testing.T) {
		t.Setenv(env, "my-site.com")
		config := buildConfigComponent(t, fx.Replace(corecomp.MockParams{
			Params: corecomp.Params{ConfFilePath: "./testdata/site_default.yaml"},
		}))

		cfg := config.Object()

		assert.NotNil(t, cfg)
		assert.Equal(t, apiEndpointPrefix+"my-site.com", cfg.Endpoints[0].Host)
	})

	env = "DD_APM_ENABLED"
	t.Run(env, func(t *testing.T) {
		t.Setenv(env, "true")
		config := buildConfigComponent(t, fx.Replace(corecomp.MockParams{
			Params: corecomp.Params{ConfFilePath: "./testdata/full.yaml"},
		}))
		cfg := config.Object()

		assert.NotNil(t, cfg)
		assert.True(t, cfg.Enabled)
	})

	env = "DD_APM_DD_URL"
	t.Run(env, func(t *testing.T) {
		t.Setenv(env, "my-site.com")
		config := buildConfigComponent(t, fx.Replace(corecomp.MockParams{
			Params: corecomp.Params{ConfFilePath: "./testdata/full.yaml"},
		}))
		cfg := config.Object()

		assert.NotNil(t, cfg)
		assert.Equal(t, "my-site.com", cfg.Endpoints[0].Host)
	})

	env = "HTTPS_PROXY"
	t.Run(env, func(t *testing.T) {
		t.Setenv(env, "my-proxy.url")
		config := buildConfigComponent(t, fx.Replace(corecomp.MockParams{
			Params: corecomp.Params{ConfFilePath: "./testdata/full.yaml"},
		}))
		cfg := config.Object()

		assert.NotNil(t, cfg)
		assert.Equal(t, "my-proxy.url", cfg.ProxyURL.String())
	})

	env = "DD_PROXY_HTTPS"
	t.Run(env, func(t *testing.T) {
		t.Setenv(env, "my-proxy.url")
		config := buildConfigComponent(t, fx.Replace(corecomp.MockParams{
			Params: corecomp.Params{ConfFilePath: "./testdata/full.yaml"},
		}))
		cfg := config.Object()

		assert.NotNil(t, cfg)
		assert.Equal(t, "my-proxy.url", cfg.ProxyURL.String())
	})

	env = "DD_HOSTNAME"
	t.Run(env, func(t *testing.T) {
		t.Setenv(env, "local.host")
		config := buildConfigComponent(t, fx.Replace(corecomp.MockParams{
			Params: corecomp.Params{ConfFilePath: "./testdata/full.yaml"},
		}))
		cfg := config.Object()

		assert.NotNil(t, cfg)
		assert.Equal(t, "local.host", cfg.Hostname)
	})

	env = "DD_BIND_HOST"
	t.Run(env, func(t *testing.T) {
		t.Setenv(env, "bindhost.com")
		config := buildConfigComponent(t, fx.Replace(corecomp.MockParams{
			Params: corecomp.Params{ConfFilePath: "./testdata/full.yaml"},
		}))
		cfg := config.Object()

		assert.NotNil(t, cfg)
		assert.Equal(t, "bindhost.com", cfg.StatsdHost)
	})

	for _, envKey := range []string{
		"DD_RECEIVER_PORT", // deprecated
		"DD_APM_RECEIVER_PORT",
	} {
		t.Run(envKey, func(t *testing.T) {
			t.Setenv(envKey, "1234")

			config := buildConfigComponent(t, fx.Replace(corecomp.MockParams{
				Params: corecomp.Params{ConfFilePath: "./testdata/full.yaml"},
			}))
			cfg := config.Object()

			assert.NotNil(t, cfg)
			assert.Equal(t, 1234, cfg.ReceiverPort)
		})
	}

	env = "DD_DOGSTATSD_PORT"
	t.Run(env, func(t *testing.T) {
		t.Setenv(env, "4321")

		config := buildConfigComponent(t, fx.Replace(corecomp.MockParams{
			Params: corecomp.Params{ConfFilePath: "./testdata/full.yaml"},
		}))
		cfg := config.Object()

		assert.NotNil(t, cfg)
		assert.Equal(t, 4321, cfg.StatsdPort)
	})

	env = "DD_APM_NON_LOCAL_TRAFFIC"
	t.Run(env, func(t *testing.T) {
		t.Setenv(env, "true")

		config := buildConfigComponent(t, fx.Replace(corecomp.MockParams{
			Params: corecomp.Params{ConfFilePath: "./testdata/undocumented.yaml"},
		}))
		cfg := config.Object()

		assert.NotNil(t, cfg)
		assert.Equal(t, "0.0.0.0", cfg.ReceiverHost)
	})

	env = "DD_OTLP_CONFIG_TRACES_PROBABILISTIC_SAMPLER_SAMPLING_PERCENTAGE"
	t.Run(env, func(t *testing.T) {
		t.Setenv(env, "12.3")

		config := buildConfigComponent(t, fx.Replace(corecomp.MockParams{
			Params: corecomp.Params{ConfFilePath: "./testdata/undocumented.yaml"},
		}))
		cfg := config.Object()

		assert.NotNil(t, cfg)
		assert.Equal(t, 12.3, cfg.OTLPReceiver.ProbabilisticSampling)
	})

	for _, envKey := range []string{
		"DD_IGNORE_RESOURCE", // deprecated
		"DD_APM_IGNORE_RESOURCES",
	} {
		t.Run(envKey, func(t *testing.T) {
			t.Setenv(envKey, "1,2,3")

			config := buildConfigComponent(t, fx.Replace(corecomp.MockParams{
				Params: corecomp.Params{ConfFilePath: "./testdata/full.yaml"},
			}))
			cfg := config.Object()

			assert.NotNil(t, cfg)
			assert.Equal(t, []string{"1", "2", "3"}, cfg.Ignore["resource"])
		})
	}

	env = "DD_APM_ANALYZED_SPANS"
	t.Run(env, func(t *testing.T) {
		t.Setenv(env, "web|http.request=1,db|sql.query=0.5")

		config := buildConfigComponent(t, fx.Replace(corecomp.MockParams{
			Params: corecomp.Params{ConfFilePath: "./testdata/full.yaml"},
		}))

		cfg := config.Object()

		assert.NotNil(t, cfg)
		assert.Equal(t, map[string]map[string]float64{
			"web": {"http.request": 1},
			"db":  {"sql.query": 0.5},
		}, cfg.AnalyzedSpansByService)
	})

	env = "DD_APM_REPLACE_TAGS"
	t.Run(env, func(t *testing.T) {
		t.Setenv(env, `[{"name":"name1", "pattern":"pattern1"}, {"name":"name2","pattern":"pattern2","repl":"replace2"}]`)

		c := buildConfigComponent(t, fx.Replace(corecomp.MockParams{
			Params: corecomp.Params{ConfFilePath: "./testdata/full.yaml"},
		}))

		cfg := c.Object()

		assert.NotNil(t, cfg)

		rule1 := &traceconfig.ReplaceRule{
			Name:    "name1",
			Pattern: "pattern1",
			Repl:    "",
		}
		rule2 := &traceconfig.ReplaceRule{
			Name:    "name2",
			Pattern: "pattern2",
			Repl:    "replace2",
		}
		compileReplaceRules([]*traceconfig.ReplaceRule{rule1, rule2})
		assert.Contains(t, cfg.ReplaceTags, rule1)
		assert.Contains(t, cfg.ReplaceTags, rule2)
	})

	env = "DD_APM_FILTER_TAGS_REQUIRE"
	t.Run(env, func(t *testing.T) {
		t.Setenv(env, `important1 important2:value1`)

		c := buildConfigComponent(t, fx.Replace(corecomp.MockParams{
			Params: corecomp.Params{ConfFilePath: "./testdata/full.yaml"},
		}))

		cfg := c.Object()

		assert.NotNil(t, cfg)

		assert.Equal(t, cfg.RequireTags, []*traceconfig.Tag{{K: "important1"}, {K: "important2", V: "value1"}})
	})

	t.Run(env, func(t *testing.T) {
		t.Setenv(env, `["important1:value with a space"]`)

		c := buildConfigComponent(t, fx.Replace(corecomp.MockParams{
			Params: corecomp.Params{ConfFilePath: "./testdata/full.yaml"},
		}))
		cfg := c.Object()

		assert.NotNil(t, cfg)

		assert.Equal(t, cfg.RequireTags, []*traceconfig.Tag{{K: "important1", V: "value with a space"}})
	})

	env = "DD_APM_FILTER_TAGS_REGEX_REQUIRE"
	t.Run(env, func(t *testing.T) {
		t.Setenv(env, `important1 important2:^value1$`)

		c := buildConfigComponent(t, fx.Replace(corecomp.MockParams{
			Params: corecomp.Params{ConfFilePath: "./testdata/full.yaml"},
		}))

		cfg := c.Object()

		assert.NotNil(t, cfg)

		assert.Equal(t, cfg.RequireTagsRegex, []*traceconfig.TagRegex{{K: "important1"}, {K: "important2", V: regexp.MustCompile("^value1$")}})
	})

	t.Run(env, func(t *testing.T) {
		t.Setenv(env, `["important1:^value with a space$"]`)

		c := buildConfigComponent(t, fx.Replace(corecomp.MockParams{
			Params: corecomp.Params{ConfFilePath: "./testdata/full.yaml"},
		}))

		cfg := c.Object()

		assert.NotNil(t, cfg)

		assert.Equal(t, cfg.RequireTagsRegex, []*traceconfig.TagRegex{{K: "important1", V: regexp.MustCompile("^value with a space$")}})
	})

	env = "DD_APM_FILTER_TAGS_REJECT"
	t.Run(env, func(t *testing.T) {
		t.Setenv(env, `bad1:value1`)

		c := buildConfigComponent(t, fx.Replace(corecomp.MockParams{
			Params: corecomp.Params{ConfFilePath: "./testdata/full.yaml"},
		}))
		cfg := c.Object()

		assert.NotNil(t, cfg)

		assert.Equal(t, cfg.RejectTags, []*traceconfig.Tag{{K: "bad1", V: "value1"}})
	})

	t.Run(env, func(t *testing.T) {
		t.Setenv(env, `["bad1:value with a space"]`)

		c := buildConfigComponent(t, fx.Replace(corecomp.MockParams{
			Params: corecomp.Params{ConfFilePath: "./testdata/full.yaml"},
		}))
		cfg := c.Object()

		assert.NotNil(t, cfg)

		assert.Equal(t, cfg.RejectTags, []*traceconfig.Tag{{K: "bad1", V: "value with a space"}})
	})

	t.Run(env, func(t *testing.T) {
		t.Setenv(env, `["bad1:value with a space","bad2:value with spaces"]`)

		c := buildConfigComponent(t, fx.Replace(corecomp.MockParams{
			Params: corecomp.Params{ConfFilePath: "./testdata/full.yaml"},
		}))
		cfg := c.Object()

		assert.NotNil(t, cfg)

		assert.Equal(t, cfg.RejectTags, []*traceconfig.Tag{
			{K: "bad1", V: "value with a space"},
			{K: "bad2", V: "value with spaces"},
		})
	})

	env = "DD_APM_FILTER_TAGS_REGEX_REJECT"
	t.Run(env, func(t *testing.T) {
		t.Setenv(env, `bad1:^value1$`)
		c := buildConfigComponent(t, fx.Replace(corecomp.MockParams{
			Params: corecomp.Params{ConfFilePath: "./testdata/full.yaml"},
		}))
		cfg := c.Object()

		assert.NotNil(t, cfg)

		assert.Equal(t, cfg.RejectTagsRegex, []*traceconfig.TagRegex{{K: "bad1", V: regexp.MustCompile("^value1$")}})
	})

	t.Run(env, func(t *testing.T) {
		t.Setenv(env, `["bad1:value with a space"]`)
		c := buildConfigComponent(t, fx.Replace(corecomp.MockParams{
			Params: corecomp.Params{ConfFilePath: "./testdata/full.yaml"},
		}))
		cfg := c.Object()

		assert.NotNil(t, cfg)

		assert.Equal(t, cfg.RejectTagsRegex, []*traceconfig.TagRegex{{K: "bad1", V: regexp.MustCompile("value with a space")}})
	})

	for _, envKey := range []string{
		"DD_CONNECTION_LIMIT", // deprecated
		"DD_APM_CONNECTION_LIMIT",
	} {
		t.Run(envKey, func(t *testing.T) {
			t.Setenv(envKey, "50")

			c := buildConfigComponent(t, fx.Replace(corecomp.MockParams{
				Params: corecomp.Params{ConfFilePath: "./testdata/full.yaml"},
			}))
			cfg := c.Object()

			assert.NotNil(t, cfg)
			assert.Equal(t, 50, cfg.ConnectionLimit)
		})
	}

	for _, envKey := range []string{
		"DD_MAX_TPS",     // deprecated
		"DD_APM_MAX_TPS", // deprecated
		"DD_APM_TARGET_TPS",
	} {
		// First load the yaml file with the deprecated max_traces_per_second
		t.Run(envKey, func(t *testing.T) {
			t.Setenv(envKey, "6")

			c := buildConfigComponent(t, fx.Replace(corecomp.MockParams{
				Params: corecomp.Params{ConfFilePath: "./testdata/deprecated-max-tps-apm.yaml"},
			}))

			cfg := c.Object()

			assert.NotNil(t, cfg)
			assert.Equal(t, 6., cfg.TargetTPS)
		})

		// Load the yaml file with the updated target_traces_per_second. When both the deprecated setting and the
		// new one are present, the new one takes precedence.
		t.Run(envKey, func(t *testing.T) {
			t.Setenv(envKey, "6")

			c := buildConfigComponent(t, fx.Replace(corecomp.MockParams{
				Params: corecomp.Params{ConfFilePath: "./testdata/full.yaml"},
			}))
			cfg := c.Object()

			assert.NotNil(t, cfg)
			if envKey == "DD_APM_TARGET_TPS" {
				assert.Equal(t, 6., cfg.TargetTPS)
			} else {
				// target_traces_per_second from yaml config takes precedence over deprecated env vars.
				assert.Equal(t, 5., cfg.TargetTPS)
			}
		})
	}

	for _, envKey := range []string{
		"DD_APM_ERROR_TPS",
	} {
		t.Run(envKey, func(t *testing.T) {
			t.Setenv(envKey, "12")

			c := buildConfigComponent(t, fx.Replace(corecomp.MockParams{
				Params: corecomp.Params{ConfFilePath: "./testdata/full.yaml"},
			}))
			cfg := c.Object()

			assert.NotNil(t, cfg)
			assert.Equal(t, 12., cfg.ErrorTPS)
		})
	}

	for _, envKey := range []string{
		"DD_APM_ENABLE_RARE_SAMPLER",
	} {
		t.Run(envKey, func(t *testing.T) {
			t.Setenv(envKey, "true")

			c := buildConfigComponent(t, fx.Replace(corecomp.MockParams{
				Params: corecomp.Params{ConfFilePath: "./testdata/full.yaml"},
			}))
			cfg := c.Object()

			assert.NotNil(t, cfg)
			assert.Equal(t, true, cfg.RareSamplerEnabled)
		})
	}

	for _, envKey := range []string{
		"DD_MAX_EPS", // deprecated
		"DD_APM_MAX_EPS",
	} {
		t.Run(envKey, func(t *testing.T) {
			t.Setenv(envKey, "7")
			c := buildConfigComponent(t, fx.Replace(corecomp.MockParams{
				Params: corecomp.Params{ConfFilePath: "./testdata/full.yaml"},
			}))
			cfg := c.Object()

			assert.NotNil(t, cfg)
			assert.Equal(t, 7., cfg.MaxEPS)
		})
	}

	env = "DD_APM_MAX_REMOTE_TPS"
	t.Run(env, func(t *testing.T) {
		t.Setenv(env, "337.41")

		c := buildConfigComponent(t, fx.Replace(corecomp.MockParams{
			Params: corecomp.Params{ConfFilePath: "./testdata/full.yaml"},
		}))
		cfg := c.Object()

		assert.NotNil(t, cfg)
		assert.Equal(t, 337.41, cfg.MaxRemoteTPS)
	})

	env = "DD_APM_ADDITIONAL_ENDPOINTS"
	t.Run(env, func(t *testing.T) {
		t.Setenv(env, `{"url1": ["key1", "key2"], "url2": ["key3"]}`)

		c := buildConfigComponent(t, fx.Replace(corecomp.MockParams{
			Params: corecomp.Params{ConfFilePath: "./testdata/full.yaml"},
		}))
		cfg := c.Object()

		assert.NotNil(t, cfg)

		assert.Contains(t, cfg.Endpoints, &traceconfig.Endpoint{APIKey: "key1", Host: "url1"})
		assert.Contains(t, cfg.Endpoints, &traceconfig.Endpoint{APIKey: "key2", Host: "url1"})
		assert.Contains(t, cfg.Endpoints, &traceconfig.Endpoint{APIKey: "key3", Host: "url2"})
	})

	env = "DD_APM_PROFILING_DD_URL"
	t.Run(env, func(t *testing.T) {
		t.Setenv(env, "my-site.com")

		c := buildConfigComponent(t, fx.Replace(corecomp.MockParams{
			Params: corecomp.Params{ConfFilePath: "./testdata/full.yaml"},
		}))
		cfg := c.Object()

		assert.NotNil(t, cfg)

		assert.Equal(t, "my-site.com", pkgconfigsetup.Datadog().GetString("apm_config.profiling_dd_url"))
	})

	env = "DD_APM_DEBUGGER_DD_URL"
	t.Run(env, func(t *testing.T) {
		t.Setenv(env, "my-site.com")

		c := buildConfigComponent(t, fx.Replace(corecomp.MockParams{
			Params: corecomp.Params{ConfFilePath: "./testdata/full.yaml"},
		}))
		cfg := c.Object()

		assert.NotNil(t, cfg)

		assert.Equal(t, "my-site.com", pkgconfigsetup.Datadog().GetString("apm_config.debugger_dd_url"))
	})

	env = "DD_APM_DEBUGGER_API_KEY"
	t.Run(env, func(t *testing.T) {
		t.Setenv(env, "my-key")

		c := buildConfigComponent(t, fx.Replace(corecomp.MockParams{
			Params: corecomp.Params{ConfFilePath: "./testdata/full.yaml"},
		}))
		cfg := c.Object()

		assert.NotNil(t, cfg)

		assert.Equal(t, "my-key", pkgconfigsetup.Datadog().GetString("apm_config.debugger_api_key"))
	})

	env = "DD_APM_DEBUGGER_ADDITIONAL_ENDPOINTS"
	t.Run(env, func(t *testing.T) {
		t.Setenv(env, `{"url1": ["key1", "key2"], "url2": ["key3"]}`)

		c := buildConfigComponent(t, fx.Replace(corecomp.MockParams{
			Params: corecomp.Params{ConfFilePath: "./testdata/full.yaml"},
		}))
		cfg := c.Object()

		assert.NotNil(t, cfg)
		expected := map[string][]string{
			"url1": {"key1", "key2"},
			"url2": {"key3"},
		}

		actual := pkgconfigsetup.Datadog().GetStringMapStringSlice("apm_config.debugger_additional_endpoints")
		if !reflect.DeepEqual(actual, expected) {
			t.Fatalf("Failed to process env var %s, expected %v and got %v", env, expected, actual)
		}
	})

	env = "DD_APM_DEBUGGER_DIAGNOSTICS_DD_URL"
	t.Run(env, func(t *testing.T) {
		t.Setenv(env, "my-diagnostics-site.com")

		c := buildConfigComponent(t, fx.Replace(corecomp.MockParams{
			Params: corecomp.Params{ConfFilePath: "./testdata/full.yaml"},
		}))
		cfg := c.Object()

		assert.NotNil(t, cfg)

		assert.Equal(t, "my-diagnostics-site.com", pkgconfigsetup.Datadog().GetString("apm_config.debugger_diagnostics_dd_url"))
	})

	env = "DD_APM_DEBUGGER_DIAGNOSTICS_API_KEY"
	t.Run(env, func(t *testing.T) {
		t.Setenv(env, "my-diagnostics-key")

		c := buildConfigComponent(t, fx.Replace(corecomp.MockParams{
			Params: corecomp.Params{ConfFilePath: "./testdata/full.yaml"},
		}))
		cfg := c.Object()

		assert.NotNil(t, cfg)

		assert.Equal(t, "my-diagnostics-key", pkgconfigsetup.Datadog().GetString("apm_config.debugger_diagnostics_api_key"))
	})

	env = "DD_APM_DEBUGGER_DIAGNOSTICS_ADDITIONAL_ENDPOINTS"
	t.Run(env, func(t *testing.T) {
		t.Setenv(env, `{"diagnostics-url1": ["diagnostics-key1", "diagnostics-key2"], "diagnostics-url2": ["diagnostics-key3"]}`)

		c := buildConfigComponent(t, fx.Replace(corecomp.MockParams{
			Params: corecomp.Params{ConfFilePath: "./testdata/full.yaml"},
		}))
		cfg := c.Object()

		assert.NotNil(t, cfg)
		expected := map[string][]string{
			"diagnostics-url1": {"diagnostics-key1", "diagnostics-key2"},
			"diagnostics-url2": {"diagnostics-key3"},
		}

		actual := pkgconfigsetup.Datadog().GetStringMapStringSlice("apm_config.debugger_diagnostics_additional_endpoints")
		if !reflect.DeepEqual(actual, expected) {
			t.Fatalf("Failed to process env var %s, expected %v and got %v", env, expected, actual)
		}
	})

	env = "DD_APM_SYMDB_DD_URL"
	t.Run(env, func(t *testing.T) {
		t.Setenv(env, "my-site.com")

		c := buildConfigComponent(t, fx.Replace(corecomp.MockParams{
			Params: corecomp.Params{ConfFilePath: "./testdata/full.yaml"},
		}))
		cfg := c.Object()

		assert.NotNil(t, cfg)
		assert.Equal(t, "my-site.com", pkgconfigsetup.Datadog().GetString("apm_config.symdb_dd_url"))
	})

	env = "DD_APM_SYMDB_API_KEY"
	t.Run(env, func(t *testing.T) {
		t.Setenv(env, "my-key")

		c := buildConfigComponent(t, fx.Replace(corecomp.MockParams{
			Params: corecomp.Params{ConfFilePath: "./testdata/full.yaml"},
		}))
		cfg := c.Object()

		assert.NotNil(t, cfg)
		assert.Equal(t, "my-key", pkgconfigsetup.Datadog().GetString("apm_config.symdb_api_key"))
	})

	env = "DD_APM_SYMDB_ADDITIONAL_ENDPOINTS"
	t.Run(env, func(t *testing.T) {
		t.Setenv(env, `{"url1": ["key1", "key2"], "url2": ["key3"]}`)

		c := buildConfigComponent(t, fx.Replace(corecomp.MockParams{
			Params: corecomp.Params{ConfFilePath: "./testdata/full.yaml"},
		}))
		cfg := c.Object()

		assert.NotNil(t, cfg)
		expected := map[string][]string{
			"url1": {"key1", "key2"},
			"url2": {"key3"},
		}

		actual := pkgconfigsetup.Datadog().GetStringMapStringSlice("apm_config.symdb_additional_endpoints")
		if !reflect.DeepEqual(actual, expected) {
			t.Fatalf("Failed to process env var %s, expected %v and got %v", env, expected, actual)
		}
	})

	env = "DD_APM_OBFUSCATION_CREDIT_CARDS_ENABLED"
	t.Run(env, func(t *testing.T) {
		t.Setenv(env, "false")

		c := buildConfigComponent(t, fx.Replace(corecomp.MockParams{
			Params: corecomp.Params{ConfFilePath: "./testdata/full.yaml"},
		}))
		cfg := c.Object()

		assert.NotNil(t, cfg)

		assert.False(t, pkgconfigsetup.Datadog().GetBool("apm_config.obfuscation.credit_cards.enabled"))
		assert.False(t, cfg.Obfuscation.CreditCards.Enabled)
	})

	env = "DD_APM_OBFUSCATION_CREDIT_CARDS_LUHN"
	t.Run(env, func(t *testing.T) {
		t.Setenv(env, "false")

		c := buildConfigComponent(t, fx.Replace(corecomp.MockParams{
			Params: corecomp.Params{ConfFilePath: "./testdata/full.yaml"},
		}))
		cfg := c.Object()

		assert.NotNil(t, cfg)
		assert.False(t, pkgconfigsetup.Datadog().GetBool("apm_config.obfuscation.credit_cards.luhn"))
	})

	env = "DD_APM_OBFUSCATION_ELASTICSEARCH_ENABLED"
	t.Run(env, func(t *testing.T) {
		t.Setenv(env, "true")

		c := buildConfigComponent(t, fx.Replace(corecomp.MockParams{
			Params: corecomp.Params{ConfFilePath: "./testdata/full.yaml"},
		}))
		cfg := c.Object()

		assert.NotNil(t, cfg)
		assert.True(t, pkgconfigsetup.Datadog().GetBool("apm_config.obfuscation.elasticsearch.enabled"))
		assert.True(t, cfg.Obfuscation.ES.Enabled)
	})

	env = "DD_APM_OBFUSCATION_ELASTICSEARCH_KEEP_VALUES"
	t.Run(env, func(t *testing.T) {
		t.Setenv(env, `["client_id", "product_id"]`)

		c := buildConfigComponent(t, fx.Replace(corecomp.MockParams{
			Params: corecomp.Params{ConfFilePath: "./testdata/full.yaml"},
		}))
		cfg := c.Object()

		assert.NotNil(t, cfg)
		expected := []string{"client_id", "product_id"}
		actualConfig := pkgconfigsetup.Datadog().GetStringSlice("apm_config.obfuscation.elasticsearch.keep_values")
		actualParsed := cfg.Obfuscation.ES.KeepValues
		assert.Equal(t, expected, actualConfig)
		assert.Equal(t, expected, actualParsed)
	})

	env = "DD_APM_OBFUSCATION_ELASTICSEARCH_OBFUSCATE_SQL_VALUES"
	t.Run(env, func(t *testing.T) {
		t.Setenv(env, `["key1", "key2"]`)

		c := buildConfigComponent(t, fx.Replace(corecomp.MockParams{
			Params: corecomp.Params{ConfFilePath: "./testdata/full.yaml"},
		}))
		cfg := c.Object()

		assert.NotNil(t, cfg)
		expected := []string{"key1", "key2"}
		actualConfig := pkgconfigsetup.Datadog().GetStringSlice("apm_config.obfuscation.elasticsearch.obfuscate_sql_values")
		actualParsed := cfg.Obfuscation.ES.ObfuscateSQLValues
		assert.Equal(t, expected, actualConfig)
		assert.Equal(t, expected, actualParsed)
	})

	env = "DD_APM_OBFUSCATION_HTTP_REMOVE_QUERY_STRING"
	t.Run(env, func(t *testing.T) {
		t.Setenv(env, "true")

		c := buildConfigComponent(t, fx.Replace(corecomp.MockParams{
			Params: corecomp.Params{ConfFilePath: "./testdata/full.yaml"},
		}))
		cfg := c.Object()

		assert.NotNil(t, cfg)
		assert.True(t, pkgconfigsetup.Datadog().GetBool("apm_config.obfuscation.http.remove_query_string"))
		assert.True(t, cfg.Obfuscation.HTTP.RemoveQueryString)
	})

	env = "DD_APM_OBFUSCATION_HTTP_REMOVE_PATHS_WITH_DIGITS"
	t.Run(env, func(t *testing.T) {
		t.Setenv(env, "true")

		c := buildConfigComponent(t, fx.Replace(corecomp.MockParams{
			Params: corecomp.Params{ConfFilePath: "./testdata/full.yaml"},
		}))
		cfg := c.Object()

		assert.NotNil(t, cfg)
		assert.True(t, pkgconfigsetup.Datadog().GetBool("apm_config.obfuscation.memcached.enabled"))
		assert.True(t, cfg.Obfuscation.Memcached.Enabled)
	})

	env = "DD_APM_OBFUSCATION_MEMCACHED_ENABLED"
	t.Run(env, func(t *testing.T) {
		t.Setenv(env, "true")

		c := buildConfigComponent(t, fx.Replace(corecomp.MockParams{
			Params: corecomp.Params{ConfFilePath: "./testdata/full.yaml"},
		}))
		cfg := c.Object()

		assert.NotNil(t, cfg)
		assert.True(t, pkgconfigsetup.Datadog().GetBool("apm_config.obfuscation.memcached.enabled"))
		assert.True(t, cfg.Obfuscation.Memcached.Enabled)
	})

	env = "DD_APM_OBFUSCATION_MEMCACHED_KEEP_COMMAND"
	t.Run(env, func(t *testing.T) {
		t.Setenv(env, "true")

		c := buildConfigComponent(t, fx.Replace(corecomp.MockParams{
			Params: corecomp.Params{ConfFilePath: "./testdata/full.yaml"},
		}))
		cfg := c.Object()

		assert.NotNil(t, cfg)
		assert.True(t, pkgconfigsetup.Datadog().GetBool("apm_config.obfuscation.memcached.enabled"))
		assert.True(t, cfg.Obfuscation.Memcached.Enabled)
		assert.True(t, pkgconfigsetup.Datadog().GetBool("apm_config.obfuscation.memcached.keep_command"))
		assert.True(t, cfg.Obfuscation.Memcached.KeepCommand)
	})

	env = "DD_APM_OBFUSCATION_MONGODB_ENABLED"
	t.Run(env, func(t *testing.T) {
		t.Setenv(env, "true")

		c := buildConfigComponent(t, fx.Replace(corecomp.MockParams{
			Params: corecomp.Params{ConfFilePath: "./testdata/full.yaml"},
		}))
		cfg := c.Object()

		assert.NotNil(t, cfg)
		assert.True(t, pkgconfigsetup.Datadog().GetBool("apm_config.obfuscation.mongodb.enabled"))
		assert.True(t, cfg.Obfuscation.Mongo.Enabled)
	})

	env = "DD_APM_OBFUSCATION_MONGODB_KEEP_VALUES"
	t.Run(env, func(t *testing.T) {
		t.Setenv(env, `["document_id", "template_id"]`)

		c := buildConfigComponent(t, fx.Replace(corecomp.MockParams{
			Params: corecomp.Params{ConfFilePath: "./testdata/full.yaml"},
		}))
		cfg := c.Object()

		assert.NotNil(t, cfg)
		expected := []string{"document_id", "template_id"}
		actualConfig := pkgconfigsetup.Datadog().GetStringSlice("apm_config.obfuscation.mongodb.keep_values")
		actualParsed := cfg.Obfuscation.Mongo.KeepValues
		assert.Equal(t, expected, actualConfig)
		assert.Equal(t, expected, actualParsed)
	})

	env = "DD_APM_OBFUSCATION_MONGODB_OBFUSCATE_SQL_VALUES"
	t.Run(env, func(t *testing.T) {
		t.Setenv(env, `["key1", "key2"]`)

		c := buildConfigComponent(t, fx.Replace(corecomp.MockParams{
			Params: corecomp.Params{ConfFilePath: "./testdata/full.yaml"},
		}))
		cfg := c.Object()

		assert.NotNil(t, cfg)
		expected := []string{"key1", "key2"}
		actualConfig := pkgconfigsetup.Datadog().GetStringSlice("apm_config.obfuscation.mongodb.obfuscate_sql_values")
		actualParsed := cfg.Obfuscation.Mongo.ObfuscateSQLValues
		assert.Equal(t, expected, actualConfig)
		assert.Equal(t, expected, actualParsed)
	})

	env = "DD_APM_OBFUSCATION_REDIS_ENABLED"
	t.Run(env, func(t *testing.T) {
		t.Setenv(env, "true")

		c := buildConfigComponent(t, fx.Replace(corecomp.MockParams{
			Params: corecomp.Params{ConfFilePath: "./testdata/full.yaml"},
		}))
		cfg := c.Object()

		assert.NotNil(t, cfg)
		assert.True(t, pkgconfigsetup.Datadog().GetBool("apm_config.obfuscation.redis.enabled"))
		assert.True(t, cfg.Obfuscation.Redis.Enabled)
	})

	env = "DD_APM_OBFUSCATION_REDIS_REMOVE_ALL_ARGS"
	t.Run(env, func(t *testing.T) {
		t.Setenv(env, "true")

		c := buildConfigComponent(t, fx.Replace(corecomp.MockParams{
			Params: corecomp.Params{ConfFilePath: "./testdata/full.yaml"},
		}))
		cfg := c.Object()

		assert.NotNil(t, cfg)
		assert.True(t, pkgconfigsetup.Datadog().GetBool("apm_config.obfuscation.redis.remove_all_args"))
		assert.True(t, cfg.Obfuscation.Redis.RemoveAllArgs)
	})

	env = "DD_APM_OBFUSCATION_REMOVE_STACK_TRACES"
	t.Run(env, func(t *testing.T) {
		t.Setenv(env, "true")

		c := buildConfigComponent(t, fx.Replace(corecomp.MockParams{
			Params: corecomp.Params{ConfFilePath: "./testdata/full.yaml"},
		}))
		cfg := c.Object()

		assert.NotNil(t, cfg)
		assert.True(t, pkgconfigsetup.Datadog().GetBool("apm_config.obfuscation.remove_stack_traces"))
		assert.True(t, cfg.Obfuscation.RemoveStackTraces)
	})

	env = "DD_APM_OBFUSCATION_SQL_EXEC_PLAN_ENABLED"
	t.Run(env, func(t *testing.T) {
		t.Setenv(env, "true")

		c := buildConfigComponent(t, fx.Replace(corecomp.MockParams{
			Params: corecomp.Params{ConfFilePath: "./testdata/full.yaml"},
		}))
		cfg := c.Object()

		assert.NotNil(t, cfg)
		assert.True(t, pkgconfigsetup.Datadog().GetBool("apm_config.obfuscation.sql_exec_plan.enabled"))
		assert.True(t, cfg.Obfuscation.SQLExecPlan.Enabled)
	})

	env = "DD_APM_OBFUSCATION_SQL_EXEC_PLAN_KEEP_VALUES"
	t.Run(env, func(t *testing.T) {
		t.Setenv(env, `["id1", "id2"]`)

		c := buildConfigComponent(t, fx.Replace(corecomp.MockParams{
			Params: corecomp.Params{ConfFilePath: "./testdata/full.yaml"},
		}))
		cfg := c.Object()

		assert.NotNil(t, cfg)
		expected := []string{"id1", "id2"}
		actualConfig := pkgconfigsetup.Datadog().GetStringSlice("apm_config.obfuscation.sql_exec_plan.keep_values")
		actualParsed := cfg.Obfuscation.SQLExecPlan.KeepValues
		assert.Equal(t, expected, actualConfig)
		assert.Equal(t, expected, actualParsed)
	})

	env = "DD_APM_OBFUSCATION_SQL_EXEC_PLAN_OBFUSCATE_SQL_VALUES"
	t.Run(env, func(t *testing.T) {
		t.Setenv(env, `["key1", "key2"]`)

		c := buildConfigComponent(t, fx.Replace(corecomp.MockParams{
			Params: corecomp.Params{ConfFilePath: "./testdata/full.yaml"},
		}))
		cfg := c.Object()

		assert.NotNil(t, cfg)
		expected := []string{"key1", "key2"}
		actualConfig := pkgconfigsetup.Datadog().GetStringSlice("apm_config.obfuscation.sql_exec_plan.obfuscate_sql_values")
		actualParsed := cfg.Obfuscation.SQLExecPlan.ObfuscateSQLValues
		assert.Equal(t, expected, actualConfig)
		assert.Equal(t, expected, actualParsed)
	})

	env = "DD_APM_OBFUSCATION_SQL_EXEC_PLAN_NORMALIZE_ENABLED"
	t.Run(env, func(t *testing.T) {
		t.Setenv(env, "true")

		c := buildConfigComponent(t, fx.Replace(corecomp.MockParams{
			Params: corecomp.Params{ConfFilePath: "./testdata/full.yaml"},
		}))
		cfg := c.Object()

		assert.NotNil(t, cfg)
		assert.True(t, pkgconfigsetup.Datadog().GetBool("apm_config.obfuscation.sql_exec_plan_normalize.enabled"))
		assert.True(t, cfg.Obfuscation.SQLExecPlanNormalize.Enabled)
	})

	env = "DD_APM_OBFUSCATION_SQL_EXEC_PLAN_NORMALIZE_KEEP_VALUES"
	t.Run(env, func(t *testing.T) {
		t.Setenv(env, `["id1", "id2"]`)

		c := buildConfigComponent(t, fx.Replace(corecomp.MockParams{
			Params: corecomp.Params{ConfFilePath: "./testdata/full.yaml"},
		}))
		cfg := c.Object()

		assert.NotNil(t, cfg)
		expected := []string{"id1", "id2"}
		actualConfig := pkgconfigsetup.Datadog().GetStringSlice("apm_config.obfuscation.sql_exec_plan_normalize.keep_values")
		actualParsed := cfg.Obfuscation.SQLExecPlanNormalize.KeepValues
		assert.Equal(t, expected, actualConfig)
		assert.Equal(t, expected, actualParsed)
	})

	env = "DD_APM_OBFUSCATION_SQL_EXEC_PLAN_NORMALIZE_OBFUSCATE_SQL_VALUES"
	t.Run(env, func(t *testing.T) {
		t.Setenv(env, `["key1", "key2"]`)

		c := buildConfigComponent(t, fx.Replace(corecomp.MockParams{
			Params: corecomp.Params{ConfFilePath: "./testdata/full.yaml"},
		}))
		cfg := c.Object()

		assert.NotNil(t, cfg)
		expected := []string{"key1", "key2"}
		actualConfig := pkgconfigsetup.Datadog().GetStringSlice("apm_config.obfuscation.sql_exec_plan_normalize.obfuscate_sql_values")
		actualParsed := cfg.Obfuscation.SQLExecPlanNormalize.ObfuscateSQLValues
		assert.Equal(t, expected, actualConfig)
		assert.Equal(t, expected, actualParsed)
	})

	env = "DD_APM_PROFILING_ADDITIONAL_ENDPOINTS"
	t.Run(env, func(t *testing.T) {
		t.Setenv(env, `{"url1": ["key1", "key2"], "url2": ["key3"]}`)

		c := buildConfigComponent(t, fx.Replace(corecomp.MockParams{
			Params: corecomp.Params{ConfFilePath: "./testdata/full.yaml"},
		}))
		cfg := c.Object()

		assert.NotNil(t, cfg)

		expected := map[string][]string{
			"url1": {"key1", "key2"},
			"url2": {"key3"},
		}
		actual := pkgconfigsetup.Datadog().GetStringMapStringSlice("apm_config.profiling_additional_endpoints")
		if !reflect.DeepEqual(actual, expected) {
			t.Fatalf("Failed to process env var %s, expected %v and got %v", env, expected, actual)
		}
	})

	env = "DD_APM_FEATURES"
	t.Run(env, func(t *testing.T) {
		assert := func(in string, _ []string) {
			t.Setenv(env, in)
			c := buildConfigComponent(t, fx.Replace(corecomp.MockParams{
				Params: corecomp.Params{ConfFilePath: "./testdata/full.yaml"},
			}))
			cfg := c.Object()

			assert.NotNil(t, cfg)
		}

		cases := map[string][]string{
			"":                      nil,
			"feat1":                 {"feat1"},
			"feat1,feat2,feat3":     {"feat1", "feat2", "feat3"},
			"feat1 feat2 feat3":     {"feat1", "feat2", "feat3"},
			"feat1,feat2 feat3":     {"feat1", "feat2 feat3"},    // mixing separators is not supported, comma wins
			"feat1, feat2, feat3":   {"feat1", "feat2", "feat3"}, // trim whitespaces
			"feat1 , feat2 , feat3": {"feat1", "feat2", "feat3"}, // trim whitespaces
		}
		for in, expected := range cases {
			assert(in, expected)
		}
	})

	env = "DD_INSTRUMENTATION_INSTALL_ID"
	t.Run(env, func(t *testing.T) {
		t.Setenv(env, `install_id_foo_bar`)

		c := buildConfigComponent(t, fx.Replace(corecomp.MockParams{
			Params: corecomp.Params{ConfFilePath: "./testdata/full.yaml"},
		}))
		cfg := c.Object()
		assert.NotNil(t, cfg)
		assert.Equal(t, "install_id_foo_bar", pkgconfigsetup.Datadog().GetString("apm_config.install_id"))
		assert.Equal(t, "install_id_foo_bar", cfg.InstallSignature.InstallID)
		assert.True(t, cfg.InstallSignature.Found)
	})

	env = "DD_INSTRUMENTATION_INSTALL_TYPE"
	t.Run(env, func(t *testing.T) {
		t.Setenv(env, `host_injection`)

		c := buildConfigComponent(t, fx.Replace(corecomp.MockParams{
			Params: corecomp.Params{ConfFilePath: "./testdata/full.yaml"},
		}))
		cfg := c.Object()
		assert.NotNil(t, cfg)
		assert.Equal(t, "host_injection", pkgconfigsetup.Datadog().GetString("apm_config.install_type"))
		assert.Equal(t, "host_injection", cfg.InstallSignature.InstallType)
		assert.True(t, cfg.InstallSignature.Found)
	})

	env = "DD_INSTRUMENTATION_INSTALL_TIME"
	t.Run(env, func(t *testing.T) {
		t.Setenv(env, `1699621675`)

		c := buildConfigComponent(t, fx.Replace(corecomp.MockParams{
			Params: corecomp.Params{ConfFilePath: "./testdata/full.yaml"},
		}))
		cfg := c.Object()
		assert.NotNil(t, cfg)
		assert.Equal(t, int64(1699621675), pkgconfigsetup.Datadog().GetInt64("apm_config.install_time"))
		assert.Equal(t, int64(1699621675), cfg.InstallSignature.InstallTime)
		assert.True(t, cfg.InstallSignature.Found)
	})
}

func TestFargateConfig(t *testing.T) {
	type testData struct {
		features             []env.Feature
		expectedOrchestrator traceconfig.FargateOrchestratorName
	}
	for _, data := range []testData{
		{
			features:             []env.Feature{env.ECSFargate},
			expectedOrchestrator: traceconfig.OrchestratorECS,
		},
		{
			features:             []env.Feature{env.EKSFargate},
			expectedOrchestrator: traceconfig.OrchestratorEKS,
		},
		{
			features:             []env.Feature{},
			expectedOrchestrator: traceconfig.OrchestratorUnknown,
		},
	} {
		t.Run("", func(t *testing.T) {
			env.SetFeatures(t, data.features...)
			c := buildConfigComponent(t, fx.Replace(corecomp.MockParams{
				Params: corecomp.Params{ConfFilePath: "./testdata/no_apm_config.yaml"},
			}))

			cfg := c.Object()
			assert.NotNil(t, cfg)
			assert.Equal(t, data.expectedOrchestrator, cfg.FargateOrchestrator)

		})
	}
}

func TestSetMaxMemCPU(t *testing.T) {
	t.Run("default, non-containerized", func(t *testing.T) {
		config := buildConfigComponent(t)
		cfg := config.Object()

		require.NotNil(t, cfg)

		config.SetMaxMemCPU(false)
		assert.Equal(t, 0.5, cfg.MaxCPU)
		assert.Equal(t, 5e8, cfg.MaxMemory)
	})

	t.Run("default, containerized", func(t *testing.T) {
		config := buildConfigComponent(t)
		cfg := config.Object()

		require.NotNil(t, cfg)

		config.SetMaxMemCPU(true)
		assert.Equal(t, 0.0, cfg.MaxCPU)
		assert.Equal(t, 0.0, cfg.MaxMemory)
	})

	t.Run("limits set, non-containerized", func(t *testing.T) {
		overrides := map[string]interface{}{
			"apm_config.max_cpu_percent": "20",
			"apm_config.max_memory":      "200",
		}

		config := buildConfigComponent(t, fx.Replace(corecomp.MockParams{Overrides: overrides}))
		cfg := config.Object()
		require.NotNil(t, cfg)

		config.SetMaxMemCPU(false)
		assert.Equal(t, 0.2, cfg.MaxCPU)
		assert.Equal(t, 200.0, cfg.MaxMemory)
	})

	t.Run("limits set, containerized", func(t *testing.T) {
		overrides := map[string]interface{}{
			"apm_config.max_cpu_percent": "30",
			"apm_config.max_memory":      "300",
		}

		config := buildConfigComponent(t, fx.Replace(corecomp.MockParams{Overrides: overrides}))
		cfg := config.Object()
		require.NotNil(t, cfg)

		config.SetMaxMemCPU(true)
		assert.Equal(t, 0.3, cfg.MaxCPU)
		assert.Equal(t, 300.0, cfg.MaxMemory)
	})
}

func TestPeerTagsAggregation(t *testing.T) {

	t.Run("default-enabled", func(t *testing.T) {
		config := buildConfigComponent(t)
		cfg := config.Object()
		assert.True(t, cfg.PeerTagsAggregation)
		assert.Nil(t, cfg.PeerTags)

		assert.Contains(t, cfg.ConfiguredPeerTags(), "_dd.base_service") // global base peer tag precursor
		assert.Contains(t, cfg.ConfiguredPeerTags(), "db.hostname")      // known peer tag precursors that should be loaded from peer_tags.ini
		assert.Contains(t, cfg.ConfiguredPeerTags(), "db.system")        // known peer tag precursors that should be loaded from peer_tags.ini
		assert.Contains(t, cfg.ConfiguredPeerTags(), "peer.hostname")    // known peer tag precursors that should be loaded from peer_tags.ini
		assert.Contains(t, cfg.ConfiguredPeerTags(), "peer.service")     // known peer tag precursors that should be loaded from peer_tags.ini
	})

	t.Run("disabled", func(t *testing.T) {
		overrides := map[string]interface{}{
			"apm_config.peer_service_aggregation": true,
			"apm_config.peer_tags_aggregation":    false,
		}
		config := buildConfigComponent(t, fx.Replace(corecomp.MockParams{Overrides: overrides}))
		cfg := config.Object()
		require.NotNil(t, cfg)
		assert.False(t, cfg.PeerTagsAggregation)
		assert.Nil(t, cfg.PeerTags)
		assert.Nil(t, cfg.ConfiguredPeerTags())
	})

	t.Run("deprecated-enabled", func(t *testing.T) {
		overrides := map[string]interface{}{
			"apm_config.peer_service_aggregation": true,
		}

		config := buildConfigComponent(t, fx.Replace(corecomp.MockParams{Overrides: overrides}))
		cfg := config.Object()
		require.NotNil(t, cfg)
		assert.True(t, cfg.PeerTagsAggregation)
		assert.Nil(t, cfg.PeerTags)
		assert.Contains(t, cfg.ConfiguredPeerTags(), "db.hostname")   // known peer tag precursors that should be loaded from peer_tags.ini
		assert.Contains(t, cfg.ConfiguredPeerTags(), "db.system")     // known peer tag precursors that should be loaded from peer_tags.ini
		assert.Contains(t, cfg.ConfiguredPeerTags(), "peer.hostname") // known peer tag precursors that should be loaded from peer_tags.ini
		assert.Contains(t, cfg.ConfiguredPeerTags(), "peer.service")  // known peer tag precursors that should be loaded from peer_tags.ini
	})

	t.Run("deprecated-disabled", func(t *testing.T) {
		overrides := map[string]interface{}{
			// Setting peer_service_aggregation to false has no effect. Nobody should be using this flag, though some previous beta customers might still need to migrate to peer_tags_aggregation.
			"apm_config.peer_service_aggregation": false,
		}

		config := buildConfigComponent(t, fx.Replace(corecomp.MockParams{Overrides: overrides}))
		cfg := config.Object()
		require.NotNil(t, cfg)
		assert.True(t, cfg.PeerTagsAggregation)
		assert.Nil(t, cfg.PeerTags)
		assert.NotNil(t, cfg.ConfiguredPeerTags())
		assert.Contains(t, cfg.ConfiguredPeerTags(), "db.hostname")   // known peer tag precursors that should be loaded from peer_tags.ini
		assert.Contains(t, cfg.ConfiguredPeerTags(), "db.system")     // known peer tag precursors that should be loaded from peer_tags.ini
		assert.Contains(t, cfg.ConfiguredPeerTags(), "peer.hostname") // known peer tag precursors that should be loaded from peer_tags.ini
		assert.Contains(t, cfg.ConfiguredPeerTags(), "peer.service")  // known peer tag precursors that should be loaded from peer_tags.ini
	})

	t.Run("both-enabled", func(t *testing.T) {
		overrides := map[string]interface{}{
			"apm_config.peer_service_aggregation": true,
			"apm_config.peer_tags_aggregation":    true,
		}

		config := buildConfigComponent(t, fx.Replace(corecomp.MockParams{Overrides: overrides}))
		cfg := config.Object()
		require.NotNil(t, cfg)
		assert.True(t, cfg.PeerTagsAggregation)
		assert.Nil(t, cfg.PeerTags)
		assert.NotNil(t, cfg.ConfiguredPeerTags())
		assert.Contains(t, cfg.ConfiguredPeerTags(), "db.hostname")   // known peer tag precursors that should be loaded from peer_tags.ini
		assert.Contains(t, cfg.ConfiguredPeerTags(), "db.system")     // known peer tag precursors that should be loaded from peer_tags.ini
		assert.Contains(t, cfg.ConfiguredPeerTags(), "peer.hostname") // known peer tag precursors that should be loaded from peer_tags.ini
		assert.Contains(t, cfg.ConfiguredPeerTags(), "peer.service")  // known peer tag precursors that should be loaded from peer_tags.ini
	})

	t.Run("both-disabled", func(t *testing.T) {
		overrides := map[string]interface{}{
			"apm_config.peer_service_aggregation": false,
			"apm_config.peer_tags_aggregation":    false,
		}

		config := buildConfigComponent(t, fx.Replace(corecomp.MockParams{Overrides: overrides}))
		cfg := config.Object()
		require.NotNil(t, cfg)
		assert.False(t, cfg.PeerTagsAggregation)
		assert.Nil(t, cfg.PeerTags)
		assert.Nil(t, cfg.ConfiguredPeerTags())
	})
	t.Run("disabled-user-tags", func(t *testing.T) {
		overrides := map[string]interface{}{
			"apm_config.peer_tags_aggregation": false,
			"apm_config.peer_tags":             []string{"user_peer_tag"},
		}

		config := buildConfigComponent(t, fx.Replace(corecomp.MockParams{Overrides: overrides}))
		cfg := config.Object()
		require.NotNil(t, cfg)
		assert.False(t, cfg.PeerTagsAggregation)
		assert.Equal(t, []string{"user_peer_tag"}, cfg.PeerTags)
		assert.Nil(t, cfg.ConfiguredPeerTags())
	})
	t.Run("default-enabled-user-tags", func(t *testing.T) {
		overrides := map[string]interface{}{
			"apm_config.peer_tags": []string{"user_peer_tag"},
		}

		config := buildConfigComponent(t, fx.Replace(corecomp.MockParams{Overrides: overrides}))
		cfg := config.Object()
		assert.True(t, cfg.PeerTagsAggregation)
		assert.Equal(t, []string{"user_peer_tag"}, cfg.PeerTags)
		assert.Contains(t, cfg.ConfiguredPeerTags(), "user_peer_tag")
		assert.Contains(t, cfg.ConfiguredPeerTags(), "db.hostname")   // known peer tag precursors that should be loaded from peer_tags.ini
		assert.Contains(t, cfg.ConfiguredPeerTags(), "db.system")     // known peer tag precursors that should be loaded from peer_tags.ini
		assert.Contains(t, cfg.ConfiguredPeerTags(), "peer.hostname") // known peer tag precursors that should be loaded from peer_tags.ini
		assert.Contains(t, cfg.ConfiguredPeerTags(), "peer.service")  // known peer tag precursors that should be loaded from peer_tags.ini
	})
	t.Run("enabled-user-tags", func(t *testing.T) {
		overrides := map[string]interface{}{
			"apm_config.peer_tags":             []string{"user_peer_tag"},
			"apm_config.peer_tags_aggregation": true,
		}

		config := buildConfigComponent(t, fx.Replace(corecomp.MockParams{Overrides: overrides}))
		cfg := config.Object()
		require.NotNil(t, cfg)
		assert.True(t, cfg.PeerTagsAggregation)
		assert.Equal(t, []string{"user_peer_tag"}, cfg.PeerTags)
	})
	t.Run("both-enabled-user-tags", func(t *testing.T) {
		overrides := map[string]interface{}{
			"apm_config.peer_tags":                []string{"user_peer_tag"},
			"apm_config.peer_tags_aggregation":    true,
			"apm_config.peer_service_aggregation": true,
		}

		config := buildConfigComponent(t, fx.Replace(corecomp.MockParams{Overrides: overrides}))
		cfg := config.Object()
		require.NotNil(t, cfg)
		assert.True(t, cfg.PeerTagsAggregation)
		assert.Equal(t, []string{"user_peer_tag"}, cfg.PeerTags)
		assert.Contains(t, cfg.ConfiguredPeerTags(), "user_peer_tag")
		assert.Contains(t, cfg.ConfiguredPeerTags(), "db.hostname")   // known peer tag precursors that should be loaded from peer_tags.ini
		assert.Contains(t, cfg.ConfiguredPeerTags(), "db.system")     // known peer tag precursors that should be loaded from peer_tags.ini
		assert.Contains(t, cfg.ConfiguredPeerTags(), "peer.hostname") // known peer tag precursors that should be loaded from peer_tags.ini
		assert.Contains(t, cfg.ConfiguredPeerTags(), "peer.service")  // known peer tag precursors that should be loaded from peer_tags.ini
	})
}

func TestComputeStatsBySpanKind(t *testing.T) {

	t.Run("default-enabled", func(t *testing.T) {
		config := buildConfigComponent(t)

		cfg := config.Object()

		require.NotNil(t, cfg)
		assert.True(t, cfg.ComputeStatsBySpanKind)
	})

	t.Run("disabled", func(t *testing.T) {
		overrides := map[string]interface{}{
			"apm_config.compute_stats_by_span_kind": false,
		}
		config := buildConfigComponent(t, fx.Replace(corecomp.MockParams{Overrides: overrides}))
		cfg := config.Object()

		require.NotNil(t, cfg)
		assert.False(t, cfg.ComputeStatsBySpanKind)
	})

	t.Run("enabled", func(t *testing.T) {
		overrides := map[string]interface{}{
			"apm_config.compute_stats_by_span_kind": true,
		}

		config := buildConfigComponent(t, fx.Replace(corecomp.MockParams{Overrides: overrides}))
		cfg := config.Object()

		require.NotNil(t, cfg)
		assert.True(t, cfg.ComputeStatsBySpanKind)
	})
}

func TestGenerateInstallSignature(t *testing.T) {
	cfgDir := t.TempDir()
	cfgContent, err := os.ReadFile("./testdata/full.yaml")
	assert.NoError(t, err)
	cfgFile := filepath.Join(cfgDir, "full.yaml")
	err = os.WriteFile(cfgFile, cfgContent, 0644)
	assert.NoError(t, err)

	c := buildConfigComponent(t, fx.Replace(corecomp.MockParams{
		Params: corecomp.Params{ConfFilePath: cfgFile},
	}))

	cfg := c.Object()
	assert.NotNil(t, cfg)

	assert.False(t, pkgconfigsetup.Datadog().IsSet("apm_config.install_id"))
	assert.False(t, pkgconfigsetup.Datadog().IsSet("apm_config.install_type"))
	assert.False(t, pkgconfigsetup.Datadog().IsSet("apm_config.install_time"))

	assert.True(t, cfg.InstallSignature.Found)
	installFilePath := filepath.Join(cfgDir, "install.json")
	assert.FileExists(t, installFilePath)

	installFileContent, err := os.ReadFile(installFilePath)
	assert.NoError(t, err)

	fileSignature := traceconfig.InstallSignatureConfig{}
	err = json.Unmarshal(installFileContent, &fileSignature)
	assert.NoError(t, err)

	assert.Equal(t, fileSignature.InstallID, cfg.InstallSignature.InstallID)
	assert.Equal(t, fileSignature.InstallTime, cfg.InstallSignature.InstallTime)
	assert.Equal(t, fileSignature.InstallType, cfg.InstallSignature.InstallType)
}

func TestMockConfig(t *testing.T) {
	t.Setenv("DD_SITE", "datadoghq.eu")
	config := buildConfigComponent(t, fx.Supply(corecomp.Params{}))
	cfg := config.Object()
	require.NotNil(t, cfg)

	assert.Equal(t, true, cfg.Enabled)
	assert.Equal(t, "datadoghq.eu", cfg.Site)
}

func TestMockDefaultConfig(t *testing.T) {
	config := buildConfigComponent(t, fx.Supply(corecomp.Params{}))
	cfg := config.Object()
	require.NotNil(t, cfg)

	assert.Equal(t, true, cfg.Enabled)
	assert.Equal(t, "datadoghq.com", cfg.Site)
}

func TestGetCoreConfigHandler(t *testing.T) {
	config := buildConfigComponent(t, fx.Supply(corecomp.Params{}))

	handler := config.GetConfigHandler().(http.HandlerFunc)

	// Refuse non Get query
	resp := httptest.NewRecorder()
	req := httptest.NewRequest("POST", "/config", nil)
	handler(resp, req)
	assert.Equal(t, http.StatusMethodNotAllowed, resp.Code)

	// Refuse missing auth token
	resp = httptest.NewRecorder()
	req = httptest.NewRequest("GET", "/config", nil)
	handler(resp, req)
	assert.Equal(t, http.StatusUnauthorized, resp.Code)

	// Refuse invalid auth token
	resp = httptest.NewRecorder()
	req = httptest.NewRequest("GET", "/config", nil)
	req.Header.Set("Authorization", "Bearer ABCDE")
	handler(resp, req)
	assert.Equal(t, http.StatusForbidden, resp.Code)

	// Accept valid auth token and returning a valid YAML conf
	resp = httptest.NewRecorder()
	req = httptest.NewRequest("GET", "/config", nil)
	req.Header.Set("Authorization", "Bearer "+apiutil.GetAuthToken())
	handler(resp, req)
	assert.Equal(t, http.StatusOK, resp.Code)

	conf := map[string]interface{}{}
	err := yaml.Unmarshal(resp.Body.Bytes(), &conf)
	assert.NoError(t, err, "Error loading YAML configuration from the API")
	assert.Contains(t, conf, "apm_config")
}

func TestDisableReceiverConfig(t *testing.T) {
	config := buildConfigComponent(t, fx.Replace(corecomp.MockParams{
		Params: corecomp.Params{ConfFilePath: "./testdata/disable_receiver.yaml"},
	}))
	cfg := config.Object()

	require.NotNil(t, cfg)

	assert.False(t, cfg.ReceiverEnabled)
}

func TestOnUpdateAPIKeyCallback(t *testing.T) {
	var n int
	callback := func(_, _ string) {
		n++
	}

	config := buildConfigComponent(t)

	config.OnUpdateAPIKey(callback)

	configC := config.(*cfg)
	configC.updateAPIKey("foo", "bar")
	assert.Equal(t, 1, n)
}

func buildConfigComponent(t *testing.T, coreConfigOptions ...fx.Option) Component {
	t.Helper()

	coreConfig := fxutil.Test[corecomp.Component](t,
		corecomp.MockModule(),
		fx.Options(coreConfigOptions...),
	)

	taggerComponent := fxutil.Test[tagger.Mock](t,
		fx.Replace(coreConfig),
		taggerimpl.MockModule(),
	)

	c := fxutil.Test[Component](t, fx.Options(
		fx.Provide(func() tagger.Component {
			return taggerComponent
		}),
		fx.Provide(func() corecomp.Component {
			return coreConfig
		}),
		MockModule(),
	))
	return c
}
