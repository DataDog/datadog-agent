// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

package configstreambootstrap

import (
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"

	pkgconfigmodel "github.com/DataDog/datadog-agent/pkg/config/model"
	pkgconfigsetup "github.com/DataDog/datadog-agent/pkg/config/setup"
)

func TestSeedGlobalBuilderResolvesIPCArtifactsNextToDatadogYaml(t *testing.T) {
	dir := t.TempDir()
	yamlPath := filepath.Join(dir, "datadog.yaml")
	pkgconfigsetup.InitConfigObjects()
	SeedGlobalBuilder(Settings{CmdHost: "localhost", CmdPort: 5001}, yamlPath)
	require.Equal(t, filepath.Join(dir, "auth_token"), AuthTokenFilepath())
}

func envVarSettings(t *testing.T, cfg pkgconfigmodel.Reader) map[string]string {
	t.Helper()
	control, ok := cfg.(pkgconfigmodel.EnvVarControl)
	require.True(t, ok, "the global config must implement EnvVarControl")
	return control.EnvVarSettings()
}

func TestEnvVarSettingsNamesOnlyTheVarsSetOnThisProcess(t *testing.T) {
	t.Setenv("DD_SITE", "datadoghq.eu")
	t.Setenv("DD_API_KEY", "some-secret-value")
	pkgconfigsetup.InitConfigObjects()
	cfg := pkgconfigsetup.Datadog()

	// A higher-precedence source is irrelevant: the env var was still set, and streaming ignores it.
	cfg.Set("api_key", "from-cli", pkgconfigmodel.SourceCLI)

	settings := envVarSettings(t, cfg)
	require.Equal(t, "DD_SITE", settings["site"])
	require.Equal(t, "DD_API_KEY", settings["api_key"])
	require.NotContains(t, settings, "log_level", "DD_LOG_LEVEL is not set on this process")
}

func TestDescribeEnvSettingsNamesSettingsAndTheirVars(t *testing.T) {
	require.Equal(t,
		[]string{"api_key (DD_API_KEY)", "site (DD_SITE)"},
		describeEnvSettings(map[string]string{"site": "DD_SITE", "api_key": "DD_API_KEY"}))
	require.Empty(t, describeEnvSettings(nil))
}
