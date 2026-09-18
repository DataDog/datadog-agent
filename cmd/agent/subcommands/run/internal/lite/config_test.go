// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build test

package lite

import (
	"os"
	"path/filepath"
	"testing"

	configutils "github.com/DataDog/datadog-agent/pkg/config/utils"
	"github.com/stretchr/testify/require"
)

func configFile(t *testing.T, raw string) string {
	t.Helper()
	p := filepath.Join(t.TempDir(), "datadog.yaml")
	require.NoError(t, os.WriteFile(p, []byte(raw), 0600))
	return p
}

func cleanEnv(t *testing.T) {
	t.Helper()
	for _, key := range []string{"DD_API_KEY", "DD_SITE", "DD_DD_URL", "DD_URL", "DD_HEALTH_PLATFORM_ENABLED", "DD_FLEET_POLICIES_DIR", "DD_PROXY_HTTP", "DD_PROXY_HTTPS", "DD_PROXY_NO_PROXY", "HTTP_PROXY", "HTTPS_PROXY", "NO_PROXY", "http_proxy", "https_proxy", "no_proxy"} {
		t.Setenv(key, "")
		require.NoError(t, os.Unsetenv(key))
	}
}

func TestRecoverReportingSettings(t *testing.T) {
	for _, tc := range []struct {
		name, raw, envKey, wantKey string
		wantErr                    bool
	}{
		{"malformed unrelated YAML", "api_key: yaml-key\nsite: datadoghq.eu\nlogs_config: [broken\n", "", "yaml-key", false},
		{"environment wins", "api_key: yaml-key\nsite: datadoghq.eu\nlogs_config: [broken\n", "env-key", "env-key", false},
		{"no fuzzy keys", "api_kye: candidate\nsite: datadoghq.eu\n", "", "", false},
		{"no nested flattening", "logs_config:\n  api_key: nested-key\nsite: datadoghq.eu\n", "", "", false},
		{"ambiguous key", "api_key: one\napi_key: two\nsite: datadoghq.eu\n", "", "", true},
		{"broken destination", "api_key: yaml-key\nsite: [broken\n", "", "", true},
		{"broken proxy", "api_key: yaml-key\nproxy: [broken\n", "", "", true},
		{"broken opt out", "api_key: yaml-key\nhealth_platform: [broken\n", "", "", true},
		{"quoted continuation is ambiguous", "logs_config: \"broken\napi_key: fake-key\nsite: datadoghq.eu\n", "", "", true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			cleanEnv(t)
			t.Setenv("DD_API_KEY", tc.envKey)
			cfg, _, err := recoverConfig(Params{ConfigPath: configFile(t, tc.raw)})
			if tc.wantErr {
				require.Error(t, err)
				return
			}
			require.NoError(t, err)
			require.Equal(t, tc.wantKey, cfg.GetString("api_key"))
			require.Equal(t, "https://agenthealth-intake.datadoghq.eu.", configutils.GetMainEndpoint(cfg, "https://agenthealth-intake.", "dd_url"))
		})
	}
}

func TestSelectedSources(t *testing.T) {
	cleanEnv(t)
	base := configFile(t, "api_key: base-key\n")
	extra := configFile(t, "api_key: extra-key\n")
	fleet := configFile(t, "api_key: fleet-key\n")
	p := Params{ConfigPath: base, ExtraConfigPaths: []string{extra}, FleetPoliciesDir: filepath.Dir(fleet)}
	t.Setenv("DD_API_KEY", "env-key")
	cfg, _, err := recoverConfig(p)
	require.NoError(t, err)
	require.Equal(t, "fleet-key", cfg.GetString("api_key"))
	p.FleetPoliciesDir = ""
	cfg, _, err = recoverConfig(p)
	require.NoError(t, err)
	require.Equal(t, "env-key", cfg.GetString("api_key"))
	t.Setenv("DD_API_KEY", "")
	cfg, _, err = recoverConfig(p)
	require.NoError(t, err)
	require.Equal(t, "extra-key", cfg.GetString("api_key"))
	p.ExtraConfigPaths = []string{extra + ".missing"}
	_, _, err = recoverConfig(p)
	require.Error(t, err)
	p = Params{ConfigPath: base + ".missing.yaml", DefaultConfigPath: filepath.Dir(base)}
	_, _, err = recoverConfig(p)
	require.Error(t, err, "an explicit missing path must not fall back")
}
