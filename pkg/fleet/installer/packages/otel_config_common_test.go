// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package packages

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestEnableOTelCollectorConfigInDatadogYAML(t *testing.T) {
	tests := []struct {
		name          string
		datadogYAML   string
		expectContent []string
		isDatadogYAML bool
	}{
		{
			name:          "adds otelcollector.enabled and agent_ipc defaults when datadog.yaml is present",
			datadogYAML:   "initial_configuration: present\n",
			expectContent: []string{"otelcollector:\n    enabled: true", "agent_ipc:\n", "    port: 5009\n", "    config_refresh_interval: 60\n"},
			isDatadogYAML: true,
		},
		{
			name:          "Skip when datadog.yaml is not present",
			isDatadogYAML: false,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			dir := t.TempDir()

			datadogYamlPath := filepath.Join(dir, "datadog.yaml")
			if tc.isDatadogYAML {
				require.NoError(t, os.WriteFile(datadogYamlPath, []byte(tc.datadogYAML), 0o644))
			}

			ctx := HookContext{Context: t.Context()}
			require.NoError(t, enableOTelCollectorConfigInDatadogYAML(ctx, datadogYamlPath))

			if tc.isDatadogYAML {
				content, err := os.ReadFile(datadogYamlPath)
				require.NoError(t, err)

				for _, expected := range tc.expectContent {
					assert.Contains(t, string(content), expected)
				}
			}
		})
	}
}

func TestWriteOTelConfigCommonSiteSubstitution(t *testing.T) {
	const template = "endpoint: ${env:DD_SITE}\napi_key: ${env:DD_API_KEY}\n"

	tests := []struct {
		name           string
		datadogYAML    string
		expectedAPIKey string
		expectedSite   string
		isDatadogYAML  bool
	}{
		{
			name:           "defaults to datadoghq.com when site is not set",
			datadogYAML:    "api_key: testapikey\n",
			expectedAPIKey: "testapikey",
			expectedSite:   "datadoghq.com",
			isDatadogYAML:  true,
		},
		{
			name:           "uses explicit site when set",
			datadogYAML:    "api_key: testapikey\nsite: datadoghq.eu\n",
			expectedAPIKey: "testapikey",
			expectedSite:   "datadoghq.eu",
			isDatadogYAML:  true,
		},
		{
			name:           "Fallback when datadog.yaml is not present",
			expectedAPIKey: "${env:DD_API_KEY}",
			expectedSite:   "datadoghq.com",
			isDatadogYAML:  false,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			dir := t.TempDir()

			datadogYamlPath := filepath.Join(dir, "datadog.yaml")
			if tc.isDatadogYAML {
				require.NoError(t, os.WriteFile(datadogYamlPath, []byte(tc.datadogYAML), 0o644))
			}

			templatePath := filepath.Join(dir, "otel-config.yaml.tmpl")
			require.NoError(t, os.WriteFile(templatePath, []byte(template), 0o644))

			outPath := filepath.Join(dir, "otel-config.yaml")
			ctx := HookContext{Context: t.Context()}
			require.NoError(t, writeOTelConfigCommon(ctx, datadogYamlPath, templatePath, outPath, false, 0o644))

			content, err := os.ReadFile(outPath)
			require.NoError(t, err)

			assert.Contains(t, string(content), tc.expectedSite)
			assert.NotContains(t, string(content), "${env:DD_SITE}")
		})
	}
}

// The DDOT configuration directory is owned by the unprivileged dd-agent user, so these
// root-run hooks must not read or write through a symlink that leaves the directory.

// symlinkVictimFixture lays out a configuration directory next to a file that the hooks must
// never reach, and returns both paths.
func symlinkVictimFixture(t *testing.T, victimContent string) (cfgDir string, victim string) {
	t.Helper()
	base := t.TempDir()
	cfgDir = filepath.Join(base, "cfg")
	require.NoError(t, os.MkdirAll(cfgDir, 0o755))
	victim = filepath.Join(base, "victim")
	require.NoError(t, os.WriteFile(victim, []byte(victimContent), 0o600))
	return cfgDir, victim
}

func TestWriteOTelConfigCommonPreservesExistingFile(t *testing.T) {
	cfgDir := t.TempDir()

	templatePath := filepath.Join(cfgDir, "otel-config.yaml.example")
	require.NoError(t, os.WriteFile(templatePath, []byte("site: ${env:DD_SITE}\n"), 0o644))

	outPath := filepath.Join(cfgDir, "otel-config.yaml")
	require.NoError(t, os.WriteFile(outPath, []byte("tuned: by-the-user\n"), 0o640))

	ctx := HookContext{Context: t.Context()}
	require.NoError(t, writeOTelConfigCommon(ctx, filepath.Join(cfgDir, "datadog.yaml"), templatePath, outPath, true, 0o640))

	content, err := os.ReadFile(outPath)
	require.NoError(t, err)
	assert.Equal(t, "tuned: by-the-user\n", string(content),
		"an existing configuration must survive an upgrade untouched")
}

func TestWriteOTelConfigCommonRefusesSymlinkOut(t *testing.T) {
	cfgDir, victim := symlinkVictimFixture(t, "victim content\n")

	templatePath := filepath.Join(cfgDir, "otel-config.yaml.example")
	require.NoError(t, os.WriteFile(templatePath, []byte("site: ${env:DD_SITE}\n"), 0o644))

	outPath := filepath.Join(cfgDir, "otel-config.yaml")
	require.NoError(t, os.Symlink(victim, outPath))

	ctx := HookContext{Context: t.Context()}
	err := writeOTelConfigCommon(ctx, filepath.Join(cfgDir, "datadog.yaml"), templatePath, outPath, false, 0o640)
	require.ErrorContains(t, err, "could not write "+outPath)

	content, err := os.ReadFile(victim)
	require.NoError(t, err)
	assert.Equal(t, "victim content\n", string(content), "the file the symlink points at must be untouched")
}

func TestWriteOTelConfigCommonRefusesSymlinkTemplate(t *testing.T) {
	cfgDir, victim := symlinkVictimFixture(t, "root-only-secret\n")

	templatePath := filepath.Join(cfgDir, "otel-config.yaml.example")
	require.NoError(t, os.Symlink(victim, templatePath))

	outPath := filepath.Join(cfgDir, "otel-config.yaml")
	ctx := HookContext{Context: t.Context()}
	err := writeOTelConfigCommon(ctx, filepath.Join(cfgDir, "datadog.yaml"), templatePath, outPath, false, 0o640)
	require.ErrorContains(t, err, "could not read "+templatePath)

	written, err := os.ReadFile(outPath)
	if err == nil {
		assert.NotContains(t, string(written), "root-only-secret",
			"a template symlink must not copy a file from outside the directory into the config")
	}
}

func TestWriteOTelConfigCommonRefusesSymlinkDatadogYAML(t *testing.T) {
	cfgDir, victim := symlinkVictimFixture(t, "api_key: leaked\n")

	datadogYamlPath := filepath.Join(cfgDir, "datadog.yaml")
	require.NoError(t, os.Symlink(victim, datadogYamlPath))

	templatePath := filepath.Join(cfgDir, "otel-config.yaml.example")
	require.NoError(t, os.WriteFile(templatePath, []byte("api_key: ${env:DD_API_KEY}\n"), 0o644))

	outPath := filepath.Join(cfgDir, "otel-config.yaml")
	ctx := HookContext{Context: t.Context()}
	err := writeOTelConfigCommon(ctx, datadogYamlPath, templatePath, outPath, false, 0o640)
	require.ErrorContains(t, err, "could not read "+datadogYamlPath)

	written, err := os.ReadFile(outPath)
	if err == nil {
		assert.NotContains(t, string(written), "leaked",
			"reading datadog.yaml through a symlink must not leak the target into otel-config.yaml")
	}
}

func TestWriteOTelConfigCommonRefusesSymlinkWhenPreserving(t *testing.T) {
	cfgDir, victim := symlinkVictimFixture(t, "victim content\n")

	templatePath := filepath.Join(cfgDir, "otel-config.yaml.example")
	require.NoError(t, os.WriteFile(templatePath, []byte("site: ${env:DD_SITE}\n"), 0o644))

	outPath := filepath.Join(cfgDir, "otel-config.yaml")
	require.NoError(t, os.Symlink(victim, outPath))

	ctx := HookContext{Context: t.Context()}
	err := writeOTelConfigCommon(ctx, filepath.Join(cfgDir, "datadog.yaml"), templatePath, outPath, true, 0o640)
	require.ErrorContains(t, err, "it is a symlink",
		"a planted symlink must be reported, not silently treated as an existing config")
}

func TestEnableOTelCollectorConfigInDatadogYAMLRefusesSymlink(t *testing.T) {
	cfgDir, victim := symlinkVictimFixture(t, "unrelated: true\n")

	datadogYamlPath := filepath.Join(cfgDir, "datadog.yaml")
	require.NoError(t, os.Symlink(victim, datadogYamlPath))

	ctx := HookContext{Context: t.Context()}
	require.ErrorContains(t, enableOTelCollectorConfigInDatadogYAML(ctx, datadogYamlPath),
		"could not read "+datadogYamlPath)

	content, err := os.ReadFile(victim)
	require.NoError(t, err)
	assert.Equal(t, "unrelated: true\n", string(content), "the file the symlink points at must be untouched")
}

func TestOTelCollectorConfigSkipsMissingDatadogYAML(t *testing.T) {
	// Both hooks must treat a missing datadog.yaml as "nothing to do": one runs on a fresh
	// install, the other on a host whose configuration was already purged.
	missing := filepath.Join(t.TempDir(), "datadog.yaml")

	ctx := HookContext{Context: t.Context()}
	require.NoError(t, enableOTelCollectorConfigInDatadogYAML(ctx, missing))
	require.NoError(t, disableOtelCollectorConfigCommon(missing))
}

func TestDisableOtelCollectorConfigRefusesSymlink(t *testing.T) {
	cfgDir, victim := symlinkVictimFixture(t, "unrelated: true\n")

	datadogYamlPath := filepath.Join(cfgDir, "datadog.yaml")
	require.NoError(t, os.Symlink(victim, datadogYamlPath))

	require.ErrorContains(t, disableOtelCollectorConfigCommon(datadogYamlPath),
		"could not read "+datadogYamlPath)

	content, err := os.ReadFile(victim)
	require.NoError(t, err)
	assert.Equal(t, "unrelated: true\n", string(content), "the file the symlink points at must be untouched")
}
