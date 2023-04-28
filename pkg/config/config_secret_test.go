// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build secrets
// +build secrets

package config

import (
	"io/ioutil"
	"path/filepath"
	"testing"

	"github.com/DataDog/datadog-agent/pkg/secrets"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestProxyWithSecret(t *testing.T) {
	type testCase struct {
		name  string
		setup func(t *testing.T, config Config)
		tests func(t *testing.T, config Config)
	}

	cases := []testCase{
		{
			name: "secrets fron configuration for proxy",
			setup: func(t *testing.T, config Config) {
				secrets.InjectSecrets(t, "http_handle", "http_url")
				secrets.InjectSecrets(t, "https_handle", "https_url")
				secrets.InjectSecrets(t, "no_proxy_1_handle", "no_proxy_1")
				secrets.InjectSecrets(t, "no_proxy_2_handle", "no_proxy_2")

				config.Set("secret_backend_command", "some_command")
				config.Set("proxy.http", "ENC[http_handle]")
				config.Set("proxy.https", "ENC[https_handle]")
				config.Set("proxy.no_proxy", []string{"ENC[no_proxy_1_handle]", "ENC[no_proxy_2_handle]"})
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
			setup: func(t *testing.T, config Config) {
				secrets.InjectSecrets(t, "http_handle", "http_url")
				secrets.InjectSecrets(t, "https_handle", "https_url")
				secrets.InjectSecrets(t, "no_proxy_1_handle", "no_proxy_1")
				secrets.InjectSecrets(t, "no_proxy_2_handle", "no_proxy_2")

				config.Set("secret_backend_command", "some_command")
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
			setup: func(t *testing.T, config Config) {
				secrets.InjectSecrets(t, "http_handle", "http_url")
				secrets.InjectSecrets(t, "https_handle", "https_url")
				secrets.InjectSecrets(t, "no_proxy_1_handle", "no_proxy_1")
				secrets.InjectSecrets(t, "no_proxy_2_handle", "no_proxy_2")

				config.Set("secret_backend_command", "some_command")
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
			config.Set("use_proxy_for_cloud_metadata", true)

			// Viper.MergeConfigOverride, which is used when secrets is enabled, will silently fail if a
			// config file is never set.
			path := t.TempDir()
			configPath := filepath.Join(path, "empty_conf.yaml")
			ioutil.WriteFile(configPath, nil, 0600)
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
