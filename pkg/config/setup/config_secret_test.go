// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package setup

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/DataDog/datadog-agent/comp/core/secrets"
	"github.com/DataDog/datadog-agent/comp/core/secrets/secretsimpl"
	pkgconfigmodel "github.com/DataDog/datadog-agent/pkg/config/model"
	"github.com/DataDog/datadog-agent/pkg/util/optional"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

var testAdditionalEndpointsConf = []byte(`
secret_backend_command: some command
additional_endpoints:
  https://url1.com:
    - ENC[api_key_1]
    - ENC[api_key_2]
  https://url2.eu:
    - ENC[api_key_3]
process_config:
  additional_endpoints:
    https://url1.com:
      - ENC[api_key_1]
      - ENC[api_key_2]
    https://url2.eu:
      - ENC[api_key_3]
`)

func TestProxyWithSecret(t *testing.T) {
	type testCase struct {
		name  string
		setup func(t *testing.T, config pkgconfigmodel.Config, configPath string, resolver secrets.Mock)
		tests func(t *testing.T, config pkgconfigmodel.Config)
	}

	cases := []testCase{
		{
			name: "secrets from configuration for proxy",
			setup: func(t *testing.T, config pkgconfigmodel.Config, configPath string, resolver secrets.Mock) {
				resolver.SetFetchHookFunc(func(_ []string) (map[string]string, error) {
					return map[string]string{
						"http_handle":       "http_url",
						"https_handle":      "https_url",
						"no_proxy_1_handle": "no_proxy_1",
						"no_proxy_2_handle": "no_proxy_2",
					}, nil
				})

				config.SetWithoutSource("secret_backend_command", "some_command")
				config.SetWithoutSource("proxy.http", "ENC[http_handle]")
				config.SetWithoutSource("proxy.https", "ENC[https_handle]")
				config.SetWithoutSource("proxy.no_proxy", []string{"ENC[no_proxy_1_handle]", "ENC[no_proxy_2_handle]"})
			},
			tests: func(t *testing.T, config pkgconfigmodel.Config) {
				assert.Equal(t,
					&pkgconfigmodel.Proxy{
						HTTP:    "http_url",
						HTTPS:   "https_url",
						NoProxy: []string{"no_proxy_1", "no_proxy_2"},
					},
					config.GetProxies())
			},
		},
		{
			name: "secrets fron DD env vars for proxy",
			setup: func(t *testing.T, config pkgconfigmodel.Config, configPath string, resolver secrets.Mock) {
				resolver.SetFetchHookFunc(func(_ []string) (map[string]string, error) {
					return map[string]string{
						"http_handle":       "http_url",
						"https_handle":      "https_url",
						"no_proxy_1_handle": "no_proxy_1",
						"no_proxy_2_handle": "no_proxy_2",
					}, nil
				})

				config.SetWithoutSource("secret_backend_command", "some_command")
				t.Setenv("DD_PROXY_HTTP", "ENC[http_handle]")
				t.Setenv("DD_PROXY_HTTPS", "ENC[https_handle]")
				t.Setenv("DD_PROXY_NO_PROXY", "ENC[no_proxy_1_handle] ENC[no_proxy_2_handle]")
			},
			tests: func(t *testing.T, config pkgconfigmodel.Config) {
				assert.Equal(t,
					&pkgconfigmodel.Proxy{
						HTTP:    "http_url",
						HTTPS:   "https_url",
						NoProxy: []string{"no_proxy_1", "no_proxy_2"},
					},
					config.GetProxies())
			},
		},
		{
			name: "secrets fron UNIX env vars for proxy",
			setup: func(t *testing.T, config pkgconfigmodel.Config, configPath string, resolver secrets.Mock) {
				resolver.SetFetchHookFunc(func(_ []string) (map[string]string, error) {
					return map[string]string{
						"http_handle":       "http_url",
						"https_handle":      "https_url",
						"no_proxy_1_handle": "no_proxy_1",
						"no_proxy_2_handle": "no_proxy_2",
					}, nil
				})

				config.SetWithoutSource("secret_backend_command", "some_command")
				t.Setenv("HTTP_PROXY", "ENC[http_handle]")
				t.Setenv("HTTPS_PROXY", "ENC[https_handle]")
				t.Setenv("NO_PROXY", "ENC[no_proxy_1_handle],ENC[no_proxy_2_handle]")
			},
			tests: func(t *testing.T, config pkgconfigmodel.Config) {
				assert.Equal(t,
					&pkgconfigmodel.Proxy{
						HTTP:    "http_url",
						HTTPS:   "https_url",
						NoProxy: []string{"no_proxy_1", "no_proxy_2"},
					},
					config.GetProxies())
			},
		},
		{
			name: "secrets from maps with keys containing dots (ie 'additional_endpoints')",
			setup: func(t *testing.T, config pkgconfigmodel.Config, configPath string, resolver secrets.Mock) {
				resolver.SetFetchHookFunc(func(_ []string) (map[string]string, error) {
					return map[string]string{
						"api_key_1": "resolved_api_key_1",
						"api_key_2": "resolved_api_key_2",
						"api_key_3": "resolved_api_key_3",
					}, nil
				})
				os.WriteFile(configPath, testAdditionalEndpointsConf, 0600)
			},
			tests: func(t *testing.T, config pkgconfigmodel.Config) {
				expected := map[string][]string{
					"https://url1.com": {
						"resolved_api_key_1",
						"resolved_api_key_2",
					},
					"https://url2.eu": {
						"resolved_api_key_3",
					},
				}
				assert.Equal(t, expected, config.GetStringMapStringSlice("additional_endpoints"))
				assert.Equal(t, expected, config.GetStringMapStringSlice("process_config.additional_endpoints"))
			},
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			// CircleCI sets NO_PROXY, so unset it for this test
			unsetEnvForTest(t, "NO_PROXY")

			config := Conf()
			config.SetWithoutSource("use_proxy_for_cloud_metadata", true)

			path := t.TempDir()
			configPath := filepath.Join(path, "empty_conf.yaml")
			os.WriteFile(configPath, nil, 0600)
			config.SetConfigFile(configPath)

			resolver := secretsimpl.NewMock()
			if c.setup != nil {
				c.setup(t, config, configPath, resolver)
			}

			_, err := LoadCustom(config, "unit_test", optional.NewOption[secrets.Component](resolver), nil)
			require.NoError(t, err)

			c.tests(t, config)
		})
	}
}
