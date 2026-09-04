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

// configEnvVars reaches the accessor the same way bootstrap.go does.
func configEnvVars(t *testing.T, cfg pkgconfigmodel.Reader) map[string][]string {
	t.Helper()
	lister, ok := cfg.(interface{ ConfigEnvVars() map[string][]string })
	require.True(t, ok, "the global config must expose ConfigEnvVars")
	return lister.ConfigEnvVars()
}

func capturedByKey(captured []envOverride) map[string]envOverride {
	byKey := make(map[string]envOverride, len(captured))
	for _, o := range captured {
		byKey[o.key] = o
	}
	return byKey
}

func TestCaptureEnvOverridesOnlyTakesKeysTheEnvLayerDecides(t *testing.T) {
	t.Setenv("DD_SITE", "datadoghq.eu")
	t.Setenv("DD_LOG_LEVEL", "debug")
	t.Setenv("DD_API_KEY", "some-secret-value")
	pkgconfigsetup.InitConfigObjects()
	cfg := pkgconfigsetup.Datadog()

	// SourceRC (10) outranks SourceEnvVar (4), so the env var was never deciding this value.
	cfg.Set("log_level", "info", pkgconfigmodel.SourceRC)
	cfg.Set("api_key", "from-cli", pkgconfigmodel.SourceCLI)

	captured := capturedByKey(captureEnvOverrides(cfg, configEnvVars(t, cfg)))
	require.Equal(t, envOverride{key: "site", envVar: "DD_SITE", value: "datadoghq.eu"}, captured["site"])
	require.NotContains(t, captured, "log_level", "a higher-precedence source was already overriding the env var")
	require.NotContains(t, captured, "api_key", "a higher-precedence source was already overriding the env var")
}

func TestDiffEnvOverridesNamesOnlyDifferingSettings(t *testing.T) {
	pkgconfigsetup.InitConfigObjects()
	cfg := pkgconfigsetup.Datadog()
	cfg.Set("site", "datadoghq.eu", pkgconfigmodel.SourceFile)

	captured := []envOverride{
		{key: "site", envVar: "DD_SITE", value: "datadoghq.eu"},
		// Never streamed, so it reads back as the default: the incident-60263 shape.
		{key: "runtime_security_config.enabled", envVar: "DD_RUNTIME_SECURITY_CONFIG_ENABLED", value: true},
	}

	require.Equal(t,
		[]string{"runtime_security_config.enabled (DD_RUNTIME_SECURITY_CONFIG_ENABLED)"},
		diffEnvOverrides(cfg, captured),
		"a byte-identical streamed value must not warn")
}

func TestDiffEnvOverridesComparesMapsAndSlices(t *testing.T) {
	pkgconfigsetup.InitConfigObjects()
	cfg := pkgconfigsetup.Datadog()
	cfg.Set("tags", []string{"env:prod", "team:agent"}, pkgconfigmodel.SourceFile)
	cfg.Set("docker_labels_as_tags", map[string]string{"app": "kube_app"}, pkgconfigmodel.SourceFile)

	same := []envOverride{
		{key: "tags", envVar: "DD_TAGS", value: cfg.Get("tags")},
		{key: "docker_labels_as_tags", envVar: "DD_DOCKER_LABELS_AS_TAGS", value: cfg.Get("docker_labels_as_tags")},
	}
	require.Empty(t, diffEnvOverrides(cfg, same))

	differing := []envOverride{
		{key: "tags", envVar: "DD_TAGS", value: []string{"env:staging"}},
		{key: "docker_labels_as_tags", envVar: "DD_DOCKER_LABELS_AS_TAGS", value: map[string]string{"app": "other"}},
	}
	require.Equal(t,
		[]string{"tags (DD_TAGS)", "docker_labels_as_tags (DD_DOCKER_LABELS_AS_TAGS)"},
		diffEnvOverrides(cfg, differing))
}
