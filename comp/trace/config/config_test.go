// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package config

import (
	"context"
	"errors"
	"html/template"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"regexp"
	"strings"
	"time"

	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/fx"

	corecomp "github.com/DataDog/datadog-agent/comp/core/config"
	coreconfig "github.com/DataDog/datadog-agent/pkg/config"
	"github.com/DataDog/datadog-agent/pkg/trace/config"
	traceconfig "github.com/DataDog/datadog-agent/pkg/trace/config"
	"github.com/DataDog/datadog-agent/pkg/util/fxutil"
)

// team: agent-apm

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

// TestSplitTag tests various split-tagging scenarios
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
		config := fxutil.Test[Component](t, fx.Options(
			corecomp.MockModule,
			MockModule,
		))
		// underlying config
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

		config := fxutil.Test[Component](t, fx.Options(
			corecomp.MockModule,
			fx.Replace(corecomp.MockParams{Overrides: overrides}),
			MockModule,
		))
		// underlying config
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

		config := fxutil.Test[Component](t, fx.Options(
			corecomp.MockModule,
			fx.Replace(corecomp.MockParams{Overrides: overrides}),
			MockModule,
		))

		// underlying config
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

		config := fxutil.Test[Component](t, fx.Options(
			corecomp.MockModule,
			fx.Replace(corecomp.MockParams{Overrides: overrides}),
			MockModule,
		))
		// underlying config
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

		config := fxutil.Test[Component](t, fx.Options(
			corecomp.MockModule,
			fx.Replace(corecomp.MockParams{Overrides: overrides}),
			MockModule,
		))
		// underlying config
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

		config := fxutil.Test[Component](t, fx.Options(
			corecomp.MockModule,
			fx.Replace(corecomp.MockParams{Overrides: overrides}),
			MockModule,
		))
		// underlying config
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

		fxutil.TestStart(t, fx.Options(
			corecomp.MockModule,
			fx.Replace(corecomp.MockParams{
				Params:      corecomp.Params{ConfFilePath: "./testdata/site_override.yaml"},
				SetupConfig: true,
				Overrides:   overrides,
			}),
			MockModule,
		), func(t testing.TB, app *fx.App) {

			require.NotNil(t, app)

			ctx := context.Background()
			err := app.Start(ctx)
			defer app.Stop(ctx)

			require.NotNil(t, err)
			assert.Contains(t, err.Error(), "nor from OS")

		}, func(config Component) {
			// nothing
		})
	})

	t.Run("fallback", func(t *testing.T) {

		host, err := os.Hostname()
		if err != nil || host == "" {
			// can't say
			t.Skip()
		}

		config := fxutil.Test[Component](t, fx.Options(
			corecomp.MockModule,
			fx.Replace(corecomp.MockParams{
				Params:      corecomp.Params{ConfFilePath: "./testdata/site_override.yaml"},
				SetupConfig: true,
			}),
			MockModule,
		))
		cfg := config.Object()

		require.NotNil(t, cfg)
		assert.Equal(t, host, cfg.Hostname)
	})

	t.Run("file", func(t *testing.T) {

		config := fxutil.Test[Component](t, fx.Options(
			corecomp.MockModule,
			fx.Replace(corecomp.MockParams{
				Params:      corecomp.Params{ConfFilePath: "./testdata/full.yaml"},
				SetupConfig: true,
			}),
			MockModule,
		))

		cfg := config.Object()

		require.NotNil(t, cfg)
		assert.Equal(t, "mymachine", cfg.Hostname)
	})

	t.Run("env", func(t *testing.T) {
		t.Setenv("XXXX_HOSTNAME", "onlyenv")

		config := fxutil.Test[Component](t, fx.Options(
			corecomp.MockModule,
			fx.Replace(corecomp.MockParams{
				Params:      corecomp.Params{ConfFilePath: "./testdata/site_override.yaml"},
				SetupConfig: true,
			}),
			MockModule,
		))
		cfg := config.Object()

		require.NotNil(t, cfg)
		assert.Equal(t, "onlyenv", cfg.Hostname)
	})

	t.Run("file+env", func(t *testing.T) {
		t.Setenv("XXXX_HOSTNAME", "envoverride")

		config := fxutil.Test[Component](t, fx.Options(
			corecomp.MockModule,
			fx.Replace(corecomp.MockParams{
				Params:      corecomp.Params{ConfFilePath: "./testdata/full.yaml"},
				SetupConfig: true,
			}),
			MockModule,
		))
		cfg := config.Object()

		require.NotNil(t, cfg)
		assert.Equal(t, "envoverride", cfg.Hostname)
	})

	t.Run("serverless", func(t *testing.T) {
		overrides := map[string]interface{}{
			"serverless.enabled": true,
		}

		config := fxutil.Test[Component](t, fx.Options(
			corecomp.MockModule,
			fx.Replace(corecomp.MockParams{
				Params:      corecomp.Params{ConfFilePath: "./testdata/site_default.yaml"},
				SetupConfig: true,
				Overrides:   overrides,
			}),
			MockModule,
		))
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

			config := fxutil.Test[Component](t, fx.Options(
				corecomp.MockModule,
				MockModule,
			))
			// underlying config
			cfg := config.Object()
			require.NotNil(t, cfg)

			cfg.DDAgentBin = bin
			assert.NoError(t, acquireHostnameFallback(cfg))
			assert.Equal(t, cfg.Hostname, "host.name")

		})

		t.Run("empty", func(t *testing.T) {
			bin := makeProgram("", 0)
			defer os.Remove(bin)

			config := fxutil.Test[Component](t, fx.Options(
				corecomp.MockModule,
				MockModule,
			))
			// underlying config
			cfg := config.Object()
			require.NotNil(t, cfg)

			cfg.DDAgentBin = bin
			assert.NoError(t, acquireHostnameFallback(cfg))
			assert.Empty(t, cfg.Hostname)
		})

		t.Run("empty+disallowed", func(t *testing.T) {
			bin := makeProgram("", 0)
			defer os.Remove(bin)

			config := fxutil.Test[Component](t, fx.Options(
				corecomp.MockModule,
				MockModule,
			))

			// underlying config
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

			config := fxutil.Test[Component](t, fx.Options(
				corecomp.MockModule,
				MockModule,
			))
			// underlying config
			cfg := config.Object()
			require.NotNil(t, cfg)

			cfg.DDAgentBin = bin
			assert.NoError(t, acquireHostnameFallback(cfg))
			assert.Equal(t, cfg.Hostname, "fallback.host")
		})

		t.Run("fallback2", func(t *testing.T) {
			bin := makeProgram("some text", 1)
			defer os.Remove(bin)

			config := fxutil.Test[Component](t, fx.Options(
				corecomp.MockModule,
				MockModule,
			))
			// underlying config
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

			config := fxutil.Test[Component](t, fx.Options(
				corecomp.MockModule,
				fx.Replace(corecomp.MockParams{
					Params:      corecomp.Params{ConfFilePath: tt.file},
					SetupConfig: true,
				}),
				MockModule,
			))
			cfg := config.Object()

			require.NotNil(t, cfg)
			assert.Equal(t, tt.url, cfg.Endpoints[0].Host)
		})
	}
}

func TestDefaultConfig(t *testing.T) {

	config := fxutil.Test[Component](t, fx.Options(
		corecomp.MockModule,
		MockModule,
	))
	cfg := config.Object()

	require.NotNil(t, cfg)

	// assert that some sane defaults are set
	assert.Equal(t, "localhost", cfg.ReceiverHost)
	assert.Equal(t, 8126, cfg.ReceiverPort)

	assert.Equal(t, "localhost", cfg.StatsdHost)
	assert.Equal(t, 8125, cfg.StatsdPort)
	assert.Equal(t, true, cfg.StatsdEnabled)

	assert.Equal(t, true, cfg.Enabled)

}

func TestNoAPMConfig(t *testing.T) {

	config := fxutil.Test[Component](t, fx.Options(
		corecomp.MockModule,
		fx.Replace(corecomp.MockParams{
			Params:      corecomp.Params{ConfFilePath: "./testdata/no_apm_config.yaml"},
			SetupConfig: true,
		}),
		MockModule,
	))
	cfg := config.Object()

	require.NotNil(t, cfg)

	assert.Equal(t, "thing", cfg.Hostname)
	assert.Equal(t, "apikey_12", cfg.Endpoints[0].APIKey)
	assert.Equal(t, "0.0.0.0", cfg.ReceiverHost)
	assert.Equal(t, 28125, cfg.StatsdPort)
}

func TestDisableLoggingConfig(t *testing.T) {

	config := fxutil.Test[Component](t, fx.Options(
		corecomp.MockModule,
		fx.Replace(corecomp.MockParams{
			Params:      corecomp.Params{ConfFilePath: "./testdata/disable_file_logging.yaml"},
			SetupConfig: true,
		}),
		MockModule,
	))
	cfg := config.Object()

	require.NotNil(t, cfg)

	assert.Equal(t, "", cfg.LogFilePath)
}

func TestFullYamlConfig(t *testing.T) {

	config := fxutil.Test[Component](t, fx.Options(
		corecomp.MockModule,
		fx.Replace(corecomp.MockParams{
			Params:      corecomp.Params{ConfFilePath: "./testdata/full.yaml"},
			SetupConfig: true,
		}),
		MockModule,
	))
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
	assert.True(t, cfg.LogThrottling)
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
	assert.ElementsMatch(t, []*traceconfig.Tag{{K: "outcome", V: "success"}}, cfg.RejectTags)

	assert.ElementsMatch(t, []*traceconfig.ReplaceRule{
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
	assert.True(t, o.CreditCards.Enabled)
	assert.True(t, o.CreditCards.Luhn)

}

func TestFileLoggingDisabled(t *testing.T) {

	config := fxutil.Test[Component](t, fx.Options(
		corecomp.MockModule,
		fx.Replace(corecomp.MockParams{
			Params:      corecomp.Params{ConfFilePath: "./testdata/disable_file_logging.yaml"},
			SetupConfig: true,
		}),
		MockModule,
	))
	cfg := config.Object()

	require.NotNil(t, cfg)
	assert.Equal(t, "", cfg.LogFilePath)
}

func TestUndocumentedYamlConfig(t *testing.T) {

	config := fxutil.Test[Component](t, fx.Options(
		corecomp.MockModule,
		fx.Replace(corecomp.MockParams{
			Params:      corecomp.Params{ConfFilePath: "./testdata/undocumented.yaml"},
			SetupConfig: true,
		}),
		MockModule,
	))
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
	c := config.New()
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
			t.Setenv("XXXX_ENV", in)

			config := fxutil.Test[Component](t, fx.Options(
				corecomp.MockModule,
				fx.Replace(corecomp.MockParams{
					Params:      corecomp.Params{ConfFilePath: "./testdata/no_apm_config.yaml"},
					SetupConfig: true,
				}),
				MockModule,
			))
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
			t.Setenv("XXXX_TAGS", in)

			config := fxutil.Test[Component](t, fx.Options(
				corecomp.MockModule,
				fx.Replace(corecomp.MockParams{
					Params:      corecomp.Params{ConfFilePath: "./testdata/no_apm_config.yaml"},
					SetupConfig: true,
				}),
				MockModule,
			))
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

			config := fxutil.Test[Component](t, fx.Options(
				corecomp.MockModule,
				fx.Replace(corecomp.MockParams{
					Params:      corecomp.Params{ConfFilePath: cfgFile},
					SetupConfig: true,
				}),
				MockModule,
			))

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
			{"DD_IGNORE_RESOURCE", "DD_APM_IGNORE_RESOURCES", "apm_config.ignore_resources"},
		} {
			t.Setenv(tt.envOld, "1,2,3")
			t.Setenv(tt.envNew, "4,5,6")

			config := fxutil.Test[Component](t, fx.Options(
				corecomp.MockModule,
				fx.Replace(corecomp.MockParams{
					Params:      corecomp.Params{ConfFilePath: "./testdata/full.yaml"},
					SetupConfig: true,
				}),
				MockModule,
			))

			cfg := config.Object()

			assert.NotNil(t, cfg)
			if tt.envNew == "DD_APM_IGNORE_RESOURCES" {
				assert.Equal(t, []string{"4", "5", "6"}, coreconfig.Datadog.GetStringSlice(tt.key))
			} else {
				assert.Equal(t, "4,5,6", coreconfig.Datadog.GetString(tt.key))
			}
		}
	})

	env := "XXXX_API_KEY"
	t.Run(env, func(t *testing.T) {
		t.Setenv(env, "123")

		config := fxutil.Test[Component](t, fx.Options(
			corecomp.MockModule,
			fx.Replace(corecomp.MockParams{
				Params:      corecomp.Params{ConfFilePath: "./testdata/full.yaml"},
				SetupConfig: true,
			}),
			MockModule,
		))
		cfg := config.Object()

		assert.NotNil(t, cfg)
		assert.Equal(t, "123", cfg.Endpoints[0].APIKey)
	})

	env = "XXXX_SITE"
	t.Run(env, func(t *testing.T) {
		os.Setenv(env, "my-site.com")
		defer os.Unsetenv(env)

		config := fxutil.Test[Component](t, fx.Options(
			corecomp.MockModule,
			fx.Replace(corecomp.MockParams{
				Params:      corecomp.Params{ConfFilePath: "./testdata/site_default.yaml"},
				SetupConfig: true,
			}),
			MockModule,
		))
		cfg := config.Object()

		assert.NotNil(t, cfg)
		assert.Equal(t, apiEndpointPrefix+"my-site.com", cfg.Endpoints[0].Host)
	})

	env = "DD_APM_ENABLED"
	t.Run(env, func(t *testing.T) {
		t.Setenv(env, "true")
		config := fxutil.Test[Component](t, fx.Options(
			corecomp.MockModule,
			fx.Replace(corecomp.MockParams{
				Params:      corecomp.Params{ConfFilePath: "./testdata/full.yaml"},
				SetupConfig: true,
			}),
			MockModule,
		))
		cfg := config.Object()

		assert.NotNil(t, cfg)
		assert.True(t, cfg.Enabled)
	})

	env = "DD_APM_DD_URL"
	t.Run(env, func(t *testing.T) {
		t.Setenv(env, "my-site.com")
		config := fxutil.Test[Component](t, fx.Options(
			corecomp.MockModule,
			fx.Replace(corecomp.MockParams{
				Params:      corecomp.Params{ConfFilePath: "./testdata/full.yaml"},
				SetupConfig: true,
			}),
			MockModule,
		))
		cfg := config.Object()

		assert.NotNil(t, cfg)
		assert.Equal(t, "my-site.com", cfg.Endpoints[0].Host)
	})

	env = "HTTPS_PROXY"
	t.Run(env, func(t *testing.T) {
		t.Setenv(env, "my-proxy.url")
		config := fxutil.Test[Component](t, fx.Options(
			corecomp.MockModule,
			fx.Replace(corecomp.MockParams{
				Params:      corecomp.Params{ConfFilePath: "./testdata/full.yaml"},
				SetupConfig: true,
			}),
			MockModule,
		))
		cfg := config.Object()

		assert.NotNil(t, cfg)
		assert.Equal(t, "my-proxy.url", cfg.ProxyURL.String())
	})

	env = "DD_PROXY_HTTPS"
	t.Run(env, func(t *testing.T) {
		t.Setenv(env, "my-proxy.url")
		config := fxutil.Test[Component](t, fx.Options(
			corecomp.MockModule,
			fx.Replace(corecomp.MockParams{
				Params:      corecomp.Params{ConfFilePath: "./testdata/full.yaml"},
				SetupConfig: true,
			}),
			MockModule,
		))
		cfg := config.Object()

		assert.NotNil(t, cfg)
		assert.Equal(t, "my-proxy.url", cfg.ProxyURL.String())
	})

	env = "XXXX_HOSTNAME"
	t.Run(env, func(t *testing.T) {
		t.Setenv(env, "local.host")
		config := fxutil.Test[Component](t, fx.Options(
			corecomp.MockModule,
			fx.Replace(corecomp.MockParams{
				Params:      corecomp.Params{ConfFilePath: "./testdata/full.yaml"},
				SetupConfig: true,
			}),
			MockModule,
		))
		cfg := config.Object()

		assert.NotNil(t, cfg)
		assert.Equal(t, "local.host", cfg.Hostname)
	})

	env = "XXXX_BIND_HOST"
	t.Run(env, func(t *testing.T) {
		t.Setenv(env, "bindhost.com")
		config := fxutil.Test[Component](t, fx.Options(
			corecomp.MockModule,
			fx.Replace(corecomp.MockParams{
				Params:      corecomp.Params{ConfFilePath: "./testdata/full.yaml"},
				SetupConfig: true,
			}),
			MockModule,
		))
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

			config := fxutil.Test[Component](t, fx.Options(
				corecomp.MockModule,
				fx.Replace(corecomp.MockParams{
					Params:      corecomp.Params{ConfFilePath: "./testdata/full.yaml"},
					SetupConfig: true,
				}),
				MockModule,
			))
			cfg := config.Object()

			assert.NotNil(t, cfg)
			assert.Equal(t, 1234, cfg.ReceiverPort)
		})
	}

	env = "XXXX_DOGSTATSD_PORT"
	t.Run(env, func(t *testing.T) {
		t.Setenv(env, "4321")

		config := fxutil.Test[Component](t, fx.Options(
			corecomp.MockModule,
			fx.Replace(corecomp.MockParams{
				Params:      corecomp.Params{ConfFilePath: "./testdata/full.yaml"},
				SetupConfig: true,
			}),
			MockModule,
		))
		cfg := config.Object()

		assert.NotNil(t, cfg)
		assert.Equal(t, 4321, cfg.StatsdPort)
	})

	env = "DD_APM_NON_LOCAL_TRAFFIC"
	t.Run(env, func(t *testing.T) {
		t.Setenv(env, "true")

		config := fxutil.Test[Component](t, fx.Options(
			corecomp.MockModule,
			fx.Replace(corecomp.MockParams{
				Params:      corecomp.Params{ConfFilePath: "./testdata/undocumented.yaml"},
				SetupConfig: true,
			}),
			MockModule,
		))
		cfg := config.Object()

		assert.NotNil(t, cfg)
		assert.Equal(t, "0.0.0.0", cfg.ReceiverHost)
	})

	env = "DD_OTLP_CONFIG_TRACES_PROBABILISTIC_SAMPLER_SAMPLING_PERCENTAGE"
	t.Run(env, func(t *testing.T) {
		t.Setenv(env, "12.3")

		config := fxutil.Test[Component](t, fx.Options(
			corecomp.MockModule,
			fx.Replace(corecomp.MockParams{
				Params:      corecomp.Params{ConfFilePath: "./testdata/undocumented.yaml"},
				SetupConfig: true,
			}),
			MockModule,
		))
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

			config := fxutil.Test[Component](t, fx.Options(
				corecomp.MockModule,
				fx.Replace(corecomp.MockParams{
					Params:      corecomp.Params{ConfFilePath: "./testdata/full.yaml"},
					SetupConfig: true,
				}),
				MockModule,
			))
			cfg := config.Object()

			assert.NotNil(t, cfg)
			assert.Equal(t, []string{"1", "2", "3"}, cfg.Ignore["resource"])
		})
	}

	env = "DD_APM_ANALYZED_SPANS"
	t.Run(env, func(t *testing.T) {

		t.Setenv(env, "web|http.request=1,db|sql.query=0.5")

		config := fxutil.Test[Component](t, fx.Options(
			corecomp.MockModule,
			fx.Replace(corecomp.MockParams{
				Params:      corecomp.Params{ConfFilePath: "./testdata/full.yaml"},
				SetupConfig: true,
			}),
			MockModule,
		))

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

		c := fxutil.Test[Component](t, fx.Options(
			corecomp.MockModule,
			fx.Replace(corecomp.MockParams{
				Params:      corecomp.Params{ConfFilePath: "./testdata/full.yaml"},
				SetupConfig: true,
			}),
			MockModule,
		))

		cfg := c.Object()

		assert.NotNil(t, cfg)

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
		assert.Contains(t, cfg.ReplaceTags, rule1)
		assert.Contains(t, cfg.ReplaceTags, rule2)
	})

	env = "DD_APM_FILTER_TAGS_REQUIRE"
	t.Run(env, func(t *testing.T) {
		t.Setenv(env, `important1 important2:value1`)

		c := fxutil.Test[Component](t, fx.Options(
			corecomp.MockModule,
			fx.Replace(corecomp.MockParams{
				Params:      corecomp.Params{ConfFilePath: "./testdata/full.yaml"},
				SetupConfig: true,
			}),
			MockModule,
		))

		cfg := c.Object()

		assert.NotNil(t, cfg)

		assert.Equal(t, cfg.RequireTags, []*config.Tag{{K: "important1", V: ""}, {K: "important2", V: "value1"}})
	})

	t.Run(env, func(t *testing.T) {
		t.Setenv(env, `["important1:value with a space"]`)

		c := fxutil.Test[Component](t, fx.Options(
			corecomp.MockModule,
			fx.Replace(corecomp.MockParams{
				Params:      corecomp.Params{ConfFilePath: "./testdata/full.yaml"},
				SetupConfig: true,
			}),
			MockModule,
		))
		cfg := c.Object()

		assert.NotNil(t, cfg)

		assert.Equal(t, cfg.RequireTags, []*config.Tag{{K: "important1", V: "value with a space"}})
	})

	env = "DD_APM_FILTER_TAGS_REJECT"
	t.Run(env, func(t *testing.T) {
		t.Setenv(env, `bad1:value1`)

		c := fxutil.Test[Component](t, fx.Options(
			corecomp.MockModule,
			fx.Replace(corecomp.MockParams{
				Params:      corecomp.Params{ConfFilePath: "./testdata/full.yaml"},
				SetupConfig: true,
			}),
			MockModule,
		))
		cfg := c.Object()

		assert.NotNil(t, cfg)

		assert.Equal(t, cfg.RejectTags, []*config.Tag{{K: "bad1", V: "value1"}})
	})

	t.Run(env, func(t *testing.T) {
		t.Setenv(env, `["bad1:value with a space"]`)

		c := fxutil.Test[Component](t, fx.Options(
			corecomp.MockModule,
			fx.Replace(corecomp.MockParams{
				Params:      corecomp.Params{ConfFilePath: "./testdata/full.yaml"},
				SetupConfig: true,
			}),
			MockModule,
		))
		cfg := c.Object()

		assert.NotNil(t, cfg)

		assert.Equal(t, cfg.RejectTags, []*config.Tag{{K: "bad1", V: "value with a space"}})
	})

	t.Run(env, func(t *testing.T) {
		t.Setenv(env, `["bad1:value with a space","bad2:value with spaces"]`)

		c := fxutil.Test[Component](t, fx.Options(
			corecomp.MockModule,
			fx.Replace(corecomp.MockParams{
				Params:      corecomp.Params{ConfFilePath: "./testdata/full.yaml"},
				SetupConfig: true,
			}),
			MockModule,
		))
		cfg := c.Object()

		assert.NotNil(t, cfg)

		assert.Equal(t, cfg.RejectTags, []*config.Tag{
			{K: "bad1", V: "value with a space"},
			{K: "bad2", V: "value with spaces"},
		})
	})

	for _, envKey := range []string{
		"DD_CONNECTION_LIMIT", // deprecated
		"DD_APM_CONNECTION_LIMIT",
	} {
		t.Run(envKey, func(t *testing.T) {
			t.Setenv(envKey, "50")

			c := fxutil.Test[Component](t, fx.Options(
				corecomp.MockModule,
				fx.Replace(corecomp.MockParams{
					Params:      corecomp.Params{ConfFilePath: "./testdata/full.yaml"},
					SetupConfig: true,
				}),
				MockModule,
			))
			cfg := c.Object()

			assert.NotNil(t, cfg)
			assert.Equal(t, 50, cfg.ConnectionLimit)
		})
	}

	for _, envKey := range []string{
		"DD_MAX_TPS", // deprecated
		"DD_APM_MAX_TPS",
	} {
		t.Run(envKey, func(t *testing.T) {
			t.Setenv(envKey, "6")

			c := fxutil.Test[Component](t, fx.Options(
				corecomp.MockModule,
				fx.Replace(corecomp.MockParams{
					Params:      corecomp.Params{ConfFilePath: "./testdata/full.yaml"},
					SetupConfig: true,
				}),
				MockModule,
			))
			cfg := c.Object()

			assert.NotNil(t, cfg)
			assert.Equal(t, 6., cfg.TargetTPS)
		})
	}

	for _, envKey := range []string{
		"DD_APM_ERROR_TPS",
	} {
		t.Run(envKey, func(t *testing.T) {
			t.Setenv(envKey, "12")

			c := fxutil.Test[Component](t, fx.Options(
				corecomp.MockModule,
				fx.Replace(corecomp.MockParams{
					Params:      corecomp.Params{ConfFilePath: "./testdata/full.yaml"},
					SetupConfig: true,
				}),
				MockModule,
			))
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

			c := fxutil.Test[Component](t, fx.Options(
				corecomp.MockModule,
				fx.Replace(corecomp.MockParams{
					Params:      corecomp.Params{ConfFilePath: "./testdata/full.yaml"},
					SetupConfig: true,
				}),
				MockModule,
			))
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
			c := fxutil.Test[Component](t, fx.Options(
				corecomp.MockModule,
				fx.Replace(corecomp.MockParams{
					Params:      corecomp.Params{ConfFilePath: "./testdata/full.yaml"},
					SetupConfig: true,
				}),
				MockModule,
			))
			cfg := c.Object()

			assert.NotNil(t, cfg)
			assert.Equal(t, 7., cfg.MaxEPS)
		})
	}

	env = "DD_APM_MAX_REMOTE_TPS"
	t.Run(env, func(t *testing.T) {
		t.Setenv(env, "337.41")

		c := fxutil.Test[Component](t, fx.Options(
			corecomp.MockModule,
			fx.Replace(corecomp.MockParams{
				Params:      corecomp.Params{ConfFilePath: "./testdata/full.yaml"},
				SetupConfig: true,
			}),
			MockModule,
		))
		cfg := c.Object()

		assert.NotNil(t, cfg)
		assert.Equal(t, 337.41, cfg.MaxRemoteTPS)
	})

	env = "DD_APM_ADDITIONAL_ENDPOINTS"
	t.Run(env, func(t *testing.T) {
		t.Setenv(env, `{"url1": ["key1", "key2"], "url2": ["key3"]}`)

		c := fxutil.Test[Component](t, fx.Options(
			corecomp.MockModule,
			fx.Replace(corecomp.MockParams{
				Params:      corecomp.Params{ConfFilePath: "./testdata/full.yaml"},
				SetupConfig: true,
			}),
			MockModule,
		))
		cfg := c.Object()

		assert.NotNil(t, cfg)

		assert.Contains(t, cfg.Endpoints, &config.Endpoint{APIKey: "key1", Host: "url1"})
		assert.Contains(t, cfg.Endpoints, &config.Endpoint{APIKey: "key2", Host: "url1"})
		assert.Contains(t, cfg.Endpoints, &config.Endpoint{APIKey: "key3", Host: "url2"})
	})

	env = "DD_APM_PROFILING_DD_URL"
	t.Run(env, func(t *testing.T) {
		t.Setenv(env, "my-site.com")

		c := fxutil.Test[Component](t, fx.Options(
			corecomp.MockModule,
			fx.Replace(corecomp.MockParams{
				Params:      corecomp.Params{ConfFilePath: "./testdata/full.yaml"},
				SetupConfig: true,
			}),
			MockModule,
		))
		cfg := c.Object()

		assert.NotNil(t, cfg)

		assert.Equal(t, "my-site.com", coreconfig.Datadog.GetString("apm_config.profiling_dd_url"))
	})

	env = "DD_APM_DEBUGGER_DD_URL"
	t.Run(env, func(t *testing.T) {
		t.Setenv(env, "my-site.com")

		c := fxutil.Test[Component](t, fx.Options(
			corecomp.MockModule,
			fx.Replace(corecomp.MockParams{
				Params:      corecomp.Params{ConfFilePath: "./testdata/full.yaml"},
				SetupConfig: true,
			}),
			MockModule,
		))
		cfg := c.Object()

		assert.NotNil(t, cfg)

		assert.Equal(t, "my-site.com", coreconfig.Datadog.GetString("apm_config.debugger_dd_url"))
	})

	env = "DD_APM_DEBUGGER_API_KEY"
	t.Run(env, func(t *testing.T) {
		t.Setenv(env, "my-key")

		c := fxutil.Test[Component](t, fx.Options(
			corecomp.MockModule,
			fx.Replace(corecomp.MockParams{
				Params:      corecomp.Params{ConfFilePath: "./testdata/full.yaml"},
				SetupConfig: true,
			}),
			MockModule,
		))
		cfg := c.Object()

		assert.NotNil(t, cfg)

		assert.Equal(t, "my-key", coreconfig.Datadog.GetString("apm_config.debugger_api_key"))
	})

	env = "DD_APM_DEBUGGER_ADDITIONAL_ENDPOINTS"
	t.Run(env, func(t *testing.T) {
		t.Setenv(env, `{"url1": ["key1", "key2"], "url2": ["key3"]}`)

		c := fxutil.Test[Component](t, fx.Options(
			corecomp.MockModule,
			fx.Replace(corecomp.MockParams{
				Params:      corecomp.Params{ConfFilePath: "./testdata/full.yaml"},
				SetupConfig: true,
			}),
			MockModule,
		))
		cfg := c.Object()

		assert.NotNil(t, cfg)
		expected := map[string][]string{
			"url1": {"key1", "key2"},
			"url2": {"key3"},
		}

		actual := coreconfig.Datadog.GetStringMapStringSlice("apm_config.debugger_additional_endpoints")
		if !reflect.DeepEqual(actual, expected) {
			t.Fatalf("Failed to process env var %s, expected %v and got %v", env, expected, actual)
		}
	})

	env = "DD_APM_SYMDB_DD_URL"
	t.Run(env, func(t *testing.T) {
		t.Setenv(env, "my-site.com")

		c := fxutil.Test[Component](t, fx.Options(
			corecomp.MockModule,
			fx.Replace(corecomp.MockParams{
				Params:      corecomp.Params{ConfFilePath: "./testdata/full.yaml"},
				SetupConfig: true,
			}),
			MockModule,
		))
		cfg := c.Object()

		assert.NotNil(t, cfg)
		assert.Equal(t, "my-site.com", coreconfig.Datadog.GetString("apm_config.symdb_dd_url"))
	})

	env = "DD_APM_SYMDB_API_KEY"
	t.Run(env, func(t *testing.T) {
		t.Setenv(env, "my-key")

		c := fxutil.Test[Component](t, fx.Options(
			corecomp.MockModule,
			fx.Replace(corecomp.MockParams{
				Params:      corecomp.Params{ConfFilePath: "./testdata/full.yaml"},
				SetupConfig: true,
			}),
			MockModule,
		))
		cfg := c.Object()

		assert.NotNil(t, cfg)
		assert.Equal(t, "my-key", coreconfig.Datadog.GetString("apm_config.symdb_api_key"))
	})

	env = "DD_APM_SYMDB_ADDITIONAL_ENDPOINTS"
	t.Run(env, func(t *testing.T) {
		t.Setenv(env, `{"url1": ["key1", "key2"], "url2": ["key3"]}`)

		c := fxutil.Test[Component](t, fx.Options(
			corecomp.MockModule,
			fx.Replace(corecomp.MockParams{
				Params:      corecomp.Params{ConfFilePath: "./testdata/full.yaml"},
				SetupConfig: true,
			}),
			MockModule,
		))
		cfg := c.Object()

		assert.NotNil(t, cfg)
		expected := map[string][]string{
			"url1": {"key1", "key2"},
			"url2": {"key3"},
		}

		actual := coreconfig.Datadog.GetStringMapStringSlice("apm_config.symdb_additional_endpoints")
		if !reflect.DeepEqual(actual, expected) {
			t.Fatalf("Failed to process env var %s, expected %v and got %v", env, expected, actual)
		}
	})

	env = "DD_APM_OBFUSCATION_CREDIT_CARDS_ENABLED"
	t.Run(env, func(t *testing.T) {
		t.Setenv(env, "false")

		c := fxutil.Test[Component](t, fx.Options(
			corecomp.MockModule,
			fx.Replace(corecomp.MockParams{
				Params:      corecomp.Params{ConfFilePath: "./testdata/full.yaml"},
				SetupConfig: true,
			}),
			MockModule,
		))
		cfg := c.Object()

		assert.NotNil(t, cfg)

		assert.False(t, coreconfig.Datadog.GetBool("apm_config.obfuscation.credit_cards.enabled"))
		assert.False(t, cfg.Obfuscation.CreditCards.Enabled)
	})

	env = "DD_APM_OBFUSCATION_CREDIT_CARDS_LUHN"
	t.Run(env, func(t *testing.T) {
		t.Setenv(env, "false")

		c := fxutil.Test[Component](t, fx.Options(
			corecomp.MockModule,
			fx.Replace(corecomp.MockParams{
				Params:      corecomp.Params{ConfFilePath: "./testdata/full.yaml"},
				SetupConfig: true,
			}),
			MockModule,
		))
		cfg := c.Object()

		assert.NotNil(t, cfg)
		assert.False(t, coreconfig.Datadog.GetBool("apm_config.obfuscation.credit_cards.luhn"))
	})

	env = "DD_APM_OBFUSCATION_ELASTICSEARCH_ENABLED"
	t.Run(env, func(t *testing.T) {
		t.Setenv(env, "true")

		c := fxutil.Test[Component](t, fx.Options(
			corecomp.MockModule,
			fx.Replace(corecomp.MockParams{
				Params:      corecomp.Params{ConfFilePath: "./testdata/full.yaml"},
				SetupConfig: true,
			}),
			MockModule,
		))
		cfg := c.Object()

		assert.NotNil(t, cfg)
		assert.True(t, coreconfig.Datadog.GetBool("apm_config.obfuscation.elasticsearch.enabled"))
		assert.True(t, cfg.Obfuscation.ES.Enabled)
	})

	env = "DD_APM_OBFUSCATION_ELASTICSEARCH_KEEP_VALUES"
	t.Run(env, func(t *testing.T) {
		t.Setenv(env, `["client_id", "product_id"]`)

		c := fxutil.Test[Component](t, fx.Options(
			corecomp.MockModule,
			fx.Replace(corecomp.MockParams{
				Params:      corecomp.Params{ConfFilePath: "./testdata/full.yaml"},
				SetupConfig: true,
			}),
			MockModule,
		))
		cfg := c.Object()

		assert.NotNil(t, cfg)
		expected := []string{"client_id", "product_id"}
		actualConfig := coreconfig.Datadog.GetStringSlice("apm_config.obfuscation.elasticsearch.keep_values")
		actualParsed := cfg.Obfuscation.ES.KeepValues
		assert.Equal(t, expected, actualConfig)
		assert.Equal(t, expected, actualParsed)
	})

	env = "DD_APM_OBFUSCATION_ELASTICSEARCH_OBFUSCATE_SQL_VALUES"
	t.Run(env, func(t *testing.T) {
		t.Setenv(env, `["key1", "key2"]`)

		c := fxutil.Test[Component](t, fx.Options(
			corecomp.MockModule,
			fx.Replace(corecomp.MockParams{
				Params:      corecomp.Params{ConfFilePath: "./testdata/full.yaml"},
				SetupConfig: true,
			}),
			MockModule,
		))
		cfg := c.Object()

		assert.NotNil(t, cfg)
		expected := []string{"key1", "key2"}
		actualConfig := coreconfig.Datadog.GetStringSlice("apm_config.obfuscation.elasticsearch.obfuscate_sql_values")
		actualParsed := cfg.Obfuscation.ES.ObfuscateSQLValues
		assert.Equal(t, expected, actualConfig)
		assert.Equal(t, expected, actualParsed)
	})

	env = "DD_APM_OBFUSCATION_HTTP_REMOVE_QUERY_STRING"
	t.Run(env, func(t *testing.T) {
		t.Setenv(env, "true")

		c := fxutil.Test[Component](t, fx.Options(
			corecomp.MockModule,
			fx.Replace(corecomp.MockParams{
				Params:      corecomp.Params{ConfFilePath: "./testdata/full.yaml"},
				SetupConfig: true,
			}),
			MockModule,
		))
		cfg := c.Object()

		assert.NotNil(t, cfg)
		assert.True(t, coreconfig.Datadog.GetBool("apm_config.obfuscation.http.remove_query_string"))
		assert.True(t, cfg.Obfuscation.HTTP.RemoveQueryString)
	})

	env = "DD_APM_OBFUSCATION_HTTP_REMOVE_PATHS_WITH_DIGITS"
	t.Run(env, func(t *testing.T) {
		t.Setenv(env, "true")

		c := fxutil.Test[Component](t, fx.Options(
			corecomp.MockModule,
			fx.Replace(corecomp.MockParams{
				Params:      corecomp.Params{ConfFilePath: "./testdata/full.yaml"},
				SetupConfig: true,
			}),
			MockModule,
		))
		cfg := c.Object()

		assert.NotNil(t, cfg)
		assert.True(t, coreconfig.Datadog.GetBool("apm_config.obfuscation.memcached.enabled"))
		assert.True(t, cfg.Obfuscation.Memcached.Enabled)
	})

	env = "DD_APM_OBFUSCATION_MEMCACHED_ENABLED"
	t.Run(env, func(t *testing.T) {
		t.Setenv(env, "true")

		c := fxutil.Test[Component](t, fx.Options(
			corecomp.MockModule,
			fx.Replace(corecomp.MockParams{
				Params:      corecomp.Params{ConfFilePath: "./testdata/full.yaml"},
				SetupConfig: true,
			}),
			MockModule,
		))
		cfg := c.Object()

		assert.NotNil(t, cfg)
		assert.True(t, coreconfig.Datadog.GetBool("apm_config.obfuscation.memcached.enabled"))
		assert.True(t, cfg.Obfuscation.Memcached.Enabled)
	})

	env = "DD_APM_OBFUSCATION_MONGODB_ENABLED"
	t.Run(env, func(t *testing.T) {
		t.Setenv(env, "true")

		c := fxutil.Test[Component](t, fx.Options(
			corecomp.MockModule,
			fx.Replace(corecomp.MockParams{
				Params:      corecomp.Params{ConfFilePath: "./testdata/full.yaml"},
				SetupConfig: true,
			}),
			MockModule,
		))
		cfg := c.Object()

		assert.NotNil(t, cfg)
		assert.True(t, coreconfig.Datadog.GetBool("apm_config.obfuscation.mongodb.enabled"))
		assert.True(t, cfg.Obfuscation.Mongo.Enabled)
	})

	env = "DD_APM_OBFUSCATION_MONGODB_KEEP_VALUES"
	t.Run(env, func(t *testing.T) {
		t.Setenv(env, `["document_id", "template_id"]`)

		c := fxutil.Test[Component](t, fx.Options(
			corecomp.MockModule,
			fx.Replace(corecomp.MockParams{
				Params:      corecomp.Params{ConfFilePath: "./testdata/full.yaml"},
				SetupConfig: true,
			}),
			MockModule,
		))
		cfg := c.Object()

		assert.NotNil(t, cfg)
		expected := []string{"document_id", "template_id"}
		actualConfig := coreconfig.Datadog.GetStringSlice("apm_config.obfuscation.mongodb.keep_values")
		actualParsed := cfg.Obfuscation.Mongo.KeepValues
		assert.Equal(t, expected, actualConfig)
		assert.Equal(t, expected, actualParsed)
	})

	env = "DD_APM_OBFUSCATION_MONGODB_OBFUSCATE_SQL_VALUES"
	t.Run(env, func(t *testing.T) {
		t.Setenv(env, `["key1", "key2"]`)

		c := fxutil.Test[Component](t, fx.Options(
			corecomp.MockModule,
			fx.Replace(corecomp.MockParams{
				Params:      corecomp.Params{ConfFilePath: "./testdata/full.yaml"},
				SetupConfig: true,
			}),
			MockModule,
		))
		cfg := c.Object()

		assert.NotNil(t, cfg)
		expected := []string{"key1", "key2"}
		actualConfig := coreconfig.Datadog.GetStringSlice("apm_config.obfuscation.mongodb.obfuscate_sql_values")
		actualParsed := cfg.Obfuscation.Mongo.ObfuscateSQLValues
		assert.Equal(t, expected, actualConfig)
		assert.Equal(t, expected, actualParsed)
	})

	env = "DD_APM_OBFUSCATION_REDIS_ENABLED"
	t.Run(env, func(t *testing.T) {
		t.Setenv(env, "true")

		c := fxutil.Test[Component](t, fx.Options(
			corecomp.MockModule,
			fx.Replace(corecomp.MockParams{
				Params:      corecomp.Params{ConfFilePath: "./testdata/full.yaml"},
				SetupConfig: true,
			}),
			MockModule,
		))
		cfg := c.Object()

		assert.NotNil(t, cfg)
		assert.True(t, coreconfig.Datadog.GetBool("apm_config.obfuscation.redis.enabled"))
		assert.True(t, cfg.Obfuscation.Redis.Enabled)
	})

	env = "DD_APM_OBFUSCATION_REDIS_REMOVE_ALL_ARGS"
	t.Run(env, func(t *testing.T) {
		t.Setenv(env, "true")

		c := fxutil.Test[Component](t, fx.Options(
			corecomp.MockModule,
			fx.Replace(corecomp.MockParams{
				Params:      corecomp.Params{ConfFilePath: "./testdata/full.yaml"},
				SetupConfig: true,
			}),
			MockModule,
		))
		cfg := c.Object()

		assert.NotNil(t, cfg)
		assert.True(t, coreconfig.Datadog.GetBool("apm_config.obfuscation.redis.remove_all_args"))
		assert.True(t, cfg.Obfuscation.Redis.RemoveAllArgs)
	})

	env = "DD_APM_OBFUSCATION_REMOVE_STACK_TRACES"
	t.Run(env, func(t *testing.T) {
		t.Setenv(env, "true")

		c := fxutil.Test[Component](t, fx.Options(
			corecomp.MockModule,
			fx.Replace(corecomp.MockParams{
				Params:      corecomp.Params{ConfFilePath: "./testdata/full.yaml"},
				SetupConfig: true,
			}),
			MockModule,
		))
		cfg := c.Object()

		assert.NotNil(t, cfg)
		assert.True(t, coreconfig.Datadog.GetBool("apm_config.obfuscation.remove_stack_traces"))
		assert.True(t, cfg.Obfuscation.RemoveStackTraces)
	})

	env = "DD_APM_OBFUSCATION_SQL_EXEC_PLAN_ENABLED"
	t.Run(env, func(t *testing.T) {
		t.Setenv(env, "true")

		c := fxutil.Test[Component](t, fx.Options(
			corecomp.MockModule,
			fx.Replace(corecomp.MockParams{
				Params:      corecomp.Params{ConfFilePath: "./testdata/full.yaml"},
				SetupConfig: true,
			}),
			MockModule,
		))
		cfg := c.Object()

		assert.NotNil(t, cfg)
		assert.True(t, coreconfig.Datadog.GetBool("apm_config.obfuscation.sql_exec_plan.enabled"))
		assert.True(t, cfg.Obfuscation.SQLExecPlan.Enabled)
	})

	env = "DD_APM_OBFUSCATION_SQL_EXEC_PLAN_KEEP_VALUES"
	t.Run(env, func(t *testing.T) {
		t.Setenv(env, `["id1", "id2"]`)

		c := fxutil.Test[Component](t, fx.Options(
			corecomp.MockModule,
			fx.Replace(corecomp.MockParams{
				Params:      corecomp.Params{ConfFilePath: "./testdata/full.yaml"},
				SetupConfig: true,
			}),
			MockModule,
		))
		cfg := c.Object()

		assert.NotNil(t, cfg)
		expected := []string{"id1", "id2"}
		actualConfig := coreconfig.Datadog.GetStringSlice("apm_config.obfuscation.sql_exec_plan.keep_values")
		actualParsed := cfg.Obfuscation.SQLExecPlan.KeepValues
		assert.Equal(t, expected, actualConfig)
		assert.Equal(t, expected, actualParsed)
	})

	env = "DD_APM_OBFUSCATION_SQL_EXEC_PLAN_OBFUSCATE_SQL_VALUES"
	t.Run(env, func(t *testing.T) {
		t.Setenv(env, `["key1", "key2"]`)

		c := fxutil.Test[Component](t, fx.Options(
			corecomp.MockModule,
			fx.Replace(corecomp.MockParams{
				Params:      corecomp.Params{ConfFilePath: "./testdata/full.yaml"},
				SetupConfig: true,
			}),
			MockModule,
		))
		cfg := c.Object()

		assert.NotNil(t, cfg)
		expected := []string{"key1", "key2"}
		actualConfig := coreconfig.Datadog.GetStringSlice("apm_config.obfuscation.sql_exec_plan.obfuscate_sql_values")
		actualParsed := cfg.Obfuscation.SQLExecPlan.ObfuscateSQLValues
		assert.Equal(t, expected, actualConfig)
		assert.Equal(t, expected, actualParsed)
	})

	env = "DD_APM_OBFUSCATION_SQL_EXEC_PLAN_NORMALIZE_ENABLED"
	t.Run(env, func(t *testing.T) {
		t.Setenv(env, "true")

		c := fxutil.Test[Component](t, fx.Options(
			corecomp.MockModule,
			fx.Replace(corecomp.MockParams{
				Params:      corecomp.Params{ConfFilePath: "./testdata/full.yaml"},
				SetupConfig: true,
			}),
			MockModule,
		))
		cfg := c.Object()

		assert.NotNil(t, cfg)
		assert.True(t, coreconfig.Datadog.GetBool("apm_config.obfuscation.sql_exec_plan_normalize.enabled"))
		assert.True(t, cfg.Obfuscation.SQLExecPlanNormalize.Enabled)
	})

	env = "DD_APM_OBFUSCATION_SQL_EXEC_PLAN_NORMALIZE_KEEP_VALUES"
	t.Run(env, func(t *testing.T) {
		t.Setenv(env, `["id1", "id2"]`)

		c := fxutil.Test[Component](t, fx.Options(
			corecomp.MockModule,
			fx.Replace(corecomp.MockParams{
				Params:      corecomp.Params{ConfFilePath: "./testdata/full.yaml"},
				SetupConfig: true,
			}),
			MockModule,
		))
		cfg := c.Object()

		assert.NotNil(t, cfg)
		expected := []string{"id1", "id2"}
		actualConfig := coreconfig.Datadog.GetStringSlice("apm_config.obfuscation.sql_exec_plan_normalize.keep_values")
		actualParsed := cfg.Obfuscation.SQLExecPlanNormalize.KeepValues
		assert.Equal(t, expected, actualConfig)
		assert.Equal(t, expected, actualParsed)
	})

	env = "DD_APM_OBFUSCATION_SQL_EXEC_PLAN_NORMALIZE_OBFUSCATE_SQL_VALUES"
	t.Run(env, func(t *testing.T) {
		t.Setenv(env, `["key1", "key2"]`)

		c := fxutil.Test[Component](t, fx.Options(
			corecomp.MockModule,
			fx.Replace(corecomp.MockParams{
				Params:      corecomp.Params{ConfFilePath: "./testdata/full.yaml"},
				SetupConfig: true,
			}),
			MockModule,
		))
		cfg := c.Object()

		assert.NotNil(t, cfg)
		expected := []string{"key1", "key2"}
		actualConfig := coreconfig.Datadog.GetStringSlice("apm_config.obfuscation.sql_exec_plan_normalize.obfuscate_sql_values")
		actualParsed := cfg.Obfuscation.SQLExecPlanNormalize.ObfuscateSQLValues
		assert.Equal(t, expected, actualConfig)
		assert.Equal(t, expected, actualParsed)
	})

	env = "DD_APM_PROFILING_ADDITIONAL_ENDPOINTS"
	t.Run(env, func(t *testing.T) {
		t.Setenv(env, `{"url1": ["key1", "key2"], "url2": ["key3"]}`)

		c := fxutil.Test[Component](t, fx.Options(
			corecomp.MockModule,
			fx.Replace(corecomp.MockParams{
				Params:      corecomp.Params{ConfFilePath: "./testdata/full.yaml"},
				SetupConfig: true,
			}),
			MockModule,
		))
		cfg := c.Object()

		assert.NotNil(t, cfg)

		expected := map[string][]string{
			"url1": {"key1", "key2"},
			"url2": {"key3"},
		}
		actual := coreconfig.Datadog.GetStringMapStringSlice("apm_config.profiling_additional_endpoints")
		if !reflect.DeepEqual(actual, expected) {
			t.Fatalf("Failed to process env var %s, expected %v and got %v", env, expected, actual)
		}
	})

	env = "DD_APM_FEATURES"
	t.Run(env, func(t *testing.T) {
		assert := func(in string, expected []string) {
			t.Setenv(env, in)
			c := fxutil.Test[Component](t, fx.Options(
				corecomp.MockModule,
				fx.Replace(corecomp.MockParams{
					Params:      corecomp.Params{ConfFilePath: "./testdata/full.yaml"},
					SetupConfig: true,
				}),
				MockModule,
			))
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
}

func TestFargateConfig(t *testing.T) {
	type testData struct {
		features             []coreconfig.Feature
		expectedOrchestrator config.FargateOrchestratorName
	}
	for _, data := range []testData{
		{
			features:             []coreconfig.Feature{coreconfig.ECSFargate},
			expectedOrchestrator: config.OrchestratorECS,
		},
		{
			features:             []coreconfig.Feature{coreconfig.EKSFargate},
			expectedOrchestrator: config.OrchestratorEKS,
		},
		{
			features:             []coreconfig.Feature{},
			expectedOrchestrator: config.OrchestratorUnknown,
		},
	} {
		t.Run("", func(t *testing.T) {

			c := fxutil.Test[Component](t, fx.Options(
				corecomp.MockModule,
				fx.Replace(corecomp.MockParams{
					Params:      corecomp.Params{ConfFilePath: "./testdata/no_apm_config.yaml"},
					Features:    data.features,
					SetupConfig: true,
				}),
				MockModule,
			))

			cfg := c.Object()
			assert.NotNil(t, cfg)
			assert.Equal(t, data.expectedOrchestrator, cfg.FargateOrchestrator)

		})
	}
}

func TestSetMaxMemCPU(t *testing.T) {
	t.Run("default, non-containerized", func(t *testing.T) {

		config := fxutil.Test[Component](t, fx.Options(
			corecomp.MockModule,
			MockModule,
		))
		cfg := config.Object()

		require.NotNil(t, cfg)

		config.SetMaxMemCPU(false)
		assert.Equal(t, 0.5, cfg.MaxCPU)
		assert.Equal(t, 5e8, cfg.MaxMemory)
	})

	t.Run("default, containerized", func(t *testing.T) {

		config := fxutil.Test[Component](t, fx.Options(
			corecomp.MockModule,
			MockModule,
		))
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

		config := fxutil.Test[Component](t, fx.Options(
			corecomp.MockModule,
			fx.Replace(corecomp.MockParams{Overrides: overrides}),
			MockModule,
		))
		// underlying config
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

		config := fxutil.Test[Component](t, fx.Options(
			corecomp.MockModule,
			fx.Replace(corecomp.MockParams{Overrides: overrides}),
			MockModule,
		))
		// underlying config
		cfg := config.Object()
		require.NotNil(t, cfg)

		config.SetMaxMemCPU(true)
		assert.Equal(t, 0.3, cfg.MaxCPU)
		assert.Equal(t, 300.0, cfg.MaxMemory)
	})
}

func TestPeerServiceAggregation(t *testing.T) {
	t.Run("disabled", func(t *testing.T) {
		config := fxutil.Test[Component](t, fx.Options(
			corecomp.MockModule,
			MockModule,
		))
		// underlying config
		cfg := config.Object()

		require.NotNil(t, cfg)
		assert.False(t, cfg.PeerServiceAggregation)
	})

	t.Run("enabled", func(t *testing.T) {
		overrides := map[string]interface{}{
			"apm_config.peer_service_aggregation": true,
		}

		config := fxutil.Test[Component](t, fx.Options(
			corecomp.MockModule,
			fx.Replace(corecomp.MockParams{Overrides: overrides}),
			MockModule,
		))
		// underlying config
		cfg := config.Object()

		require.NotNil(t, cfg)
		assert.True(t, cfg.PeerServiceAggregation)
	})
}

func TestComputeStatsBySpanKind(t *testing.T) {
	t.Run("disabled", func(t *testing.T) {
		config := fxutil.Test[Component](t, fx.Options(
			corecomp.MockModule,
			MockModule,
		))
		// underlying config
		cfg := config.Object()

		require.NotNil(t, cfg)
		assert.False(t, cfg.ComputeStatsBySpanKind)
	})

	t.Run("enabled", func(t *testing.T) {
		overrides := map[string]interface{}{
			"apm_config.compute_stats_by_span_kind": true,
		}

		config := fxutil.Test[Component](t, fx.Options(
			corecomp.MockModule,
			fx.Replace(corecomp.MockParams{Overrides: overrides}),
			MockModule,
		))
		// underlying config
		cfg := config.Object()

		require.NotNil(t, cfg)
		assert.True(t, cfg.ComputeStatsBySpanKind)
	})
}

func TestMockConfig(t *testing.T) {
	os.Setenv("DD_HOSTNAME", "foo")
	defer func() { os.Unsetenv("DD_HOSTNAME") }()

	os.Setenv("DD_SITE", "datadoghq.eu")
	defer func() { os.Unsetenv("DD_SITE") }()

	config := fxutil.Test[Component](t, fx.Options(
		fx.Supply(corecomp.Params{}),
		corecomp.MockModule,
		MockModule,
	))
	// underlying config
	cfg := config.Object()
	require.NotNil(t, cfg)

	// values aren't set from env..
	assert.NotEqual(t, "foo", cfg.Hostname)
	assert.NotEqual(t, "datadoghq.eu", cfg.Site)

	// but defaults are set
	assert.Equal(t, true, cfg.Enabled)
	assert.Equal(t, "datadoghq.com", cfg.Site)

	// but can be set by the mock
	// config.(Mock).Set("app_key", "newvalue")
	// require.Equal(t, "newvalue", config.GetString("app_key"))
}
