// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package config

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/DataDog/datadog-agent/comp/core/secrets"
	"github.com/DataDog/datadog-agent/comp/core/secrets/secretsimpl"
	"github.com/DataDog/datadog-agent/pkg/util/optional"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestProxyWithSecret(t *testing.T) {
	type testCase struct {
		name  string
		setup func(t *testing.T, config Config, resolver *secretsimpl.MockSecretResolver)
		tests func(t *testing.T, config Config)
	}

	cases := []testCase{
		{
			name: "secrets from configuration for proxy",
			setup: func(t *testing.T, config Config, resolver *secretsimpl.MockSecretResolver) {
				resolver.Inject("http_handle", "http_url")
				resolver.Inject("https_handle", "https_url")
				resolver.Inject("no_proxy_1_handle", "no_proxy_1")
				resolver.Inject("no_proxy_2_handle", "no_proxy_2")

				config.SetWithoutSource("secret_backend_command", "some_command")
				config.SetWithoutSource("proxy.http", "ENC[http_handle]")
				config.SetWithoutSource("proxy.https", "ENC[https_handle]")
				config.SetWithoutSource("proxy.no_proxy", []string{"ENC[no_proxy_1_handle]", "ENC[no_proxy_2_handle]"})
			},
			tests: func(t *testing.T, config Config) {
				assert.Equal(t,
					&Proxy{
						HTTP:    "http_url",
						HTTPS:   "https_url",
						NoProxy: []string{"no_proxy_1", "no_proxy_2"},
					},
					config.GetProxies())
			},
		},
		{
			name: "secrets fron DD env vars for proxy",
			setup: func(t *testing.T, config Config, resolver *secretsimpl.MockSecretResolver) {
				resolver.Inject("http_handle", "http_url")
				resolver.Inject("https_handle", "https_url")
				resolver.Inject("no_proxy_1_handle", "no_proxy_1")
				resolver.Inject("no_proxy_2_handle", "no_proxy_2")

				config.SetWithoutSource("secret_backend_command", "some_command")
				t.Setenv("DD_PROXY_HTTP", "ENC[http_handle]")
				t.Setenv("DD_PROXY_HTTPS", "ENC[https_handle]")
				t.Setenv("DD_PROXY_NO_PROXY", "ENC[no_proxy_1_handle] ENC[no_proxy_2_handle]")
			},
			tests: func(t *testing.T, config Config) {
				assert.Equal(t,
					&Proxy{
						HTTP:    "http_url",
						HTTPS:   "https_url",
						NoProxy: []string{"no_proxy_1", "no_proxy_2"},
					},
					config.GetProxies())
			},
		},
		{
			name: "secrets fron UNIX env vars for proxy",
			setup: func(t *testing.T, config Config, resolver *secretsimpl.MockSecretResolver) {
				resolver.Inject("http_handle", "http_url")
				resolver.Inject("https_handle", "https_url")
				resolver.Inject("no_proxy_1_handle", "no_proxy_1")
				resolver.Inject("no_proxy_2_handle", "no_proxy_2")

				config.SetWithoutSource("secret_backend_command", "some_command")
				t.Setenv("HTTP_PROXY", "ENC[http_handle]")
				t.Setenv("HTTPS_PROXY", "ENC[https_handle]")
				t.Setenv("NO_PROXY", "ENC[no_proxy_1_handle],ENC[no_proxy_2_handle]")
			},
			tests: func(t *testing.T, config Config) {
				assert.Equal(t,
					&Proxy{
						HTTP:    "http_url",
						HTTPS:   "https_url",
						NoProxy: []string{"no_proxy_1", "no_proxy_2"},
					},
					config.GetProxies())
			},
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {

			// CircleCI sets NO_PROXY, so unset it for this test
			unsetEnvForTest(t, "NO_PROXY")

			config := SetupConf()
			config.SetWithoutSource("use_proxy_for_cloud_metadata", true)

			// Viper.MergeConfigOverride, which is used when secrets is enabled, will silently fail if a
			// config file is never set.
			path := t.TempDir()
			configPath := filepath.Join(path, "empty_conf.yaml")
			os.WriteFile(configPath, nil, 0600)
			config.SetConfigFile(configPath)

			resolver := secretsimpl.NewMockSecretResolver()
			if c.setup != nil {
				c.setup(t, config, resolver)
			}

			_, err := LoadCustom(config, "unit_test", optional.NewOption[secrets.Component](resolver), nil)
			require.NoError(t, err)

			c.tests(t, config)
		})
	}
}
