// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package setup

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	secretsmock "github.com/DataDog/datadog-agent/comp/core/secrets/mock"
	pkgconfigmodel "github.com/DataDog/datadog-agent/pkg/config/model"
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
		setup func(t *testing.T, config pkgconfigmodel.Config, configPath string, resolver *secretsmock.Mock)
		tests func(t *testing.T, config pkgconfigmodel.Config)
	}

	cases := []testCase{
		{
			name: "secrets from configuration for proxy",
			setup: func(_ *testing.T, config pkgconfigmodel.Config, _ string, resolver *secretsmock.Mock) {
				resolver.SetSecrets(map[string]string{
					"http_handle":       "http_url",
					"https_handle":      "https_url",
					"no_proxy_1_handle": "no_proxy_1",
					"no_proxy_2_handle": "no_proxy_2",
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
			setup: func(t *testing.T, config pkgconfigmodel.Config, _ string, resolver *secretsmock.Mock) {
				resolver.SetSecrets(map[string]string{
					"http_handle":       "http_url",
					"https_handle":      "https_url",
					"no_proxy_1_handle": "no_proxy_1",
					"no_proxy_2_handle": "no_proxy_2",
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
			setup: func(t *testing.T, config pkgconfigmodel.Config, _ string, resolver *secretsmock.Mock) {
				resolver.SetSecrets(map[string]string{
					"http_handle":       "http_url",
					"https_handle":      "https_url",
					"no_proxy_1_handle": "no_proxy_1",
					"no_proxy_2_handle": "no_proxy_2",
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
			setup: func(_ *testing.T, _ pkgconfigmodel.Config, configPath string, resolver *secretsmock.Mock) {
				resolver.SetSecrets(map[string]string{
					"api_key_1": "resolved_api_key_1",
					"api_key_2": "resolved_api_key_2",
					"api_key_3": "resolved_api_key_3",
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

			config := newTestConf(t)
			config.SetWithoutSource("use_proxy_for_cloud_metadata", true)

			path := t.TempDir()
			configPath := filepath.Join(path, "empty_conf.yaml")
			os.WriteFile(configPath, nil, 0600)
			config.SetConfigFile(configPath)

			resolver := secretsmock.New(t)
			if c.setup != nil {
				c.setup(t, config, configPath, resolver)
			}

			err := LoadDatadog(config, resolver, nil)
			require.NoError(t, err)

			c.tests(t, config)
		})
	}
}

func TestAllFlattenedExcludesDottedAdditionalEndpointsChildrenAfterSecretResolution(t *testing.T) {
	config := newTestConf(t)

	path := t.TempDir()
	configPath := filepath.Join(path, "datadog.yaml")
	require.NoError(t, os.WriteFile(configPath, testAdditionalEndpointsConf, 0o600))
	config.SetConfigFile(configPath)

	resolver := secretsmock.New(t)
	resolver.SetSecrets(map[string]string{
		"api_key_1": "resolved_api_key_1",
		"api_key_2": "resolved_api_key_2",
		"api_key_3": "resolved_api_key_3",
	})

	require.NoError(t, LoadDatadog(config, resolver, nil))

	flattened, _ := config.AllFlattenedSettingsWithSequenceID()

	_, hasTopLevel := flattened["additional_endpoints"]
	_, hasURL1 := flattened["additional_endpoints.https://url1.com"]
	_, hasURL2 := flattened["additional_endpoints.https://url2.eu"]

	assert.True(t, hasTopLevel, "expected top-level additional_endpoints key in flattened map")
	assert.False(t, hasURL1, "did not expect dotted child key additional_endpoints.https://url1.com in flattened map")
	assert.False(t, hasURL2, "did not expect dotted child key additional_endpoints.https://url2.eu in flattened map")
}
