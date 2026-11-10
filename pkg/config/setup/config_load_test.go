// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package setup

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	delegatedauthmock "github.com/DataDog/datadog-agent/comp/core/delegatedauth/mock"
	secretsmock "github.com/DataDog/datadog-agent/comp/core/secrets/mock"
	pkgconfigmodel "github.com/DataDog/datadog-agent/pkg/config/model"
)

// untrimmedAPIKey shows whether LoadDatadog ran the post-load steps: sanitizeAPIKeyConfig trims it.
const untrimmedAPIKey = "abcdef0123456789\n"

func loadDatadogForTest(t *testing.T, config pkgconfigmodel.Config, resolver *secretsmock.Mock, options ...LoadOption) error {
	t.Helper()
	if resolver == nil {
		resolver = secretsmock.New(t)
	}
	return LoadDatadog(config, resolver, delegatedauthmock.New(t), noopSchemaValidator{}, options...)
}

func TestLoadDatadogMissingMainFile(t *testing.T) {
	missingPath := filepath.Join(t.TempDir(), "datadog.yaml")

	for _, tc := range []struct {
		name       string
		configFile string
	}{
		{name: "no file in the search paths", configFile: ""},
		{name: "SetConfigFile on a missing path", configFile: missingPath},
	} {
		for _, fullLoad := range []bool{false, true} {
			name := tc.name + "/default"
			if fullLoad {
				name = tc.name + "/full load"
			}
			t.Run(name, func(t *testing.T) {
				t.Setenv("DD_API_KEY", untrimmedAPIKey)
				config := newTestConf(t)
				config.SetConfigFile(tc.configFile)

				err := loadDatadogForTest(t, config, nil, WithFullLoadOnMissingFile(fullLoad))

				require.ErrorIs(t, err, pkgconfigmodel.ErrConfigFileNotFound)
				assert.ErrorIs(t, err, os.ErrNotExist)
				assert.Equal(t, tc.configFile, config.ConfigFileUsed())
				if fullLoad {
					assert.Equal(t, "abcdef0123456789", config.GetString("api_key"))
				} else {
					assert.Equal(t, untrimmedAPIKey, config.GetString("api_key"))
				}
			})
		}
	}
}

func TestLoadDatadogFullLoadKeepsEarlyReturn(t *testing.T) {
	for _, tc := range []struct {
		name     string
		setup    func(t *testing.T, config pkgconfigmodel.BuildableConfig)
		checkErr func(t *testing.T, err error)
	}{
		{
			name: "malformed main file",
			setup: func(t *testing.T, config pkgconfigmodel.BuildableConfig) {
				path := filepath.Join(t.TempDir(), "datadog.yaml")
				require.NoError(t, os.WriteFile(path, []byte("api_key: [unclosed"), 0o600))
				config.SetConfigFile(path)
			},
			checkErr: func(t *testing.T, err error) {
				assert.ErrorIs(t, err, pkgconfigmodel.ErrConfigFileNotFound)
				assert.NotErrorIs(t, err, os.ErrNotExist)
			},
		},
		{
			name: "missing extra file",
			setup: func(t *testing.T, config pkgconfigmodel.BuildableConfig) {
				dir := t.TempDir()
				path := filepath.Join(dir, "datadog.yaml")
				require.NoError(t, os.WriteFile(path, nil, 0o600))
				config.SetConfigFile(path)
				require.NoError(t, config.AddExtraConfigPaths([]string{filepath.Join(dir, "extra.yaml")}))
			},
			checkErr: func(t *testing.T, err error) {
				assert.ErrorIs(t, err, os.ErrNotExist)
			},
		},
		{
			name: "main file without read permission",
			setup: func(t *testing.T, config pkgconfigmodel.BuildableConfig) {
				if runtime.GOOS == "windows" {
					t.Skip("file mode bits do not control read access on Windows")
				}
				if os.Geteuid() == 0 {
					t.Skip("root can read files without read permission")
				}
				path := filepath.Join(t.TempDir(), "datadog.yaml")
				require.NoError(t, os.WriteFile(path, nil, 0o000))
				config.SetConfigFile(path)
			},
			checkErr: func(t *testing.T, err error) {
				assert.ErrorContains(t, err, "check config file permissions")
			},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Setenv("DD_API_KEY", untrimmedAPIKey)
			config := newTestConf(t)
			tc.setup(t, config)

			err := loadDatadogForTest(t, config, nil, WithFullLoadOnMissingFile(true))

			require.Error(t, err)
			tc.checkErr(t, err)
			assert.Equal(t, untrimmedAPIKey, config.GetString("api_key"), "post-load steps must not run")
		})
	}
}

func TestLoadDatadogFullLoadReturnsPostLoadErrorAlone(t *testing.T) {
	t.Setenv("DD_SECRET_BACKEND_COMMAND", "some_command")
	t.Setenv("DD_API_KEY", "ENC[unknown_handle]")
	config := newTestConf(t)

	err := loadDatadogForTest(t, config, nil, WithFullLoadOnMissingFile(true))

	require.ErrorContains(t, err, "unknown_handle")
	assert.NotErrorIs(t, err, pkgconfigmodel.ErrConfigFileNotFound)
}

// The tests below check values that LoadDatadog processes only with the option when no datadog.yaml exists.

func TestLoadDatadogFullLoadTrimsAPIKey(t *testing.T) {
	t.Setenv("DD_API_KEY", untrimmedAPIKey)
	config := newTestConf(t)

	err := loadDatadogForTest(t, config, nil, WithFullLoadOnMissingFile(true))

	require.ErrorIs(t, err, pkgconfigmodel.ErrConfigFileNotFound)
	assert.Equal(t, "abcdef0123456789", config.GetString("api_key"))
}

func TestLoadDatadogFullLoadResolvesSecretsFromEnv(t *testing.T) {
	t.Setenv("DD_SECRET_BACKEND_COMMAND", "some_command")
	t.Setenv("DD_API_KEY", "ENC[api_key_handle]")
	config := newTestConf(t)
	resolver := secretsmock.New(t)
	resolver.SetSecrets(map[string]string{"api_key_handle": "resolved_api_key"})

	err := loadDatadogForTest(t, config, resolver, WithFullLoadOnMissingFile(true))

	require.ErrorIs(t, err, pkgconfigmodel.ErrConfigFileNotFound)
	assert.Equal(t, "resolved_api_key", config.GetString("api_key"))
}

func TestLoadDatadogFullLoadReadsProxyFromEnv(t *testing.T) {
	unsetProxyEnvForTest(t)
	t.Setenv("HTTPS_PROXY", "https://proxy.example.com:3128")
	config := newTestConf(t)

	err := loadDatadogForTest(t, config, nil, WithFullLoadOnMissingFile(true))

	require.ErrorIs(t, err, pkgconfigmodel.ErrConfigFileNotFound)
	assert.Equal(t, "https://proxy.example.com:3128", config.GetString("proxy.https"))
}
