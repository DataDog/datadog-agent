// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package packages

import (
	"context"
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
			expectContent: []string{"otelcollector:\n  enabled: true", "agent_ipc:\n", "  port: 5009\n", "  config_refresh_interval: 60\n"},
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

			ctx := HookContext{Context: context.Background()}
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
		name         string
		envSite      string
		expectedSite string
	}{
		{
			name:         "defaults to datadoghq.com when DD_SITE is not set",
			envSite:      "",
			expectedSite: "datadoghq.com",
		},
		{
			name:         "uses DD_SITE when set",
			envSite:      "datadoghq.eu",
			expectedSite: "datadoghq.eu",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			dir := t.TempDir()
			if tc.envSite != "" {
				t.Setenv("DD_SITE", tc.envSite)
			} else {
				t.Setenv("DD_SITE", "")
			}

			templatePath := filepath.Join(dir, "otel-config.yaml.tmpl")
			require.NoError(t, os.WriteFile(templatePath, []byte(template), 0o644))

			outPath := filepath.Join(dir, "otel-config.yaml")
			ctx := HookContext{Context: context.Background()}
			// writeOTelConfigCommon no longer reads datadog.yaml; the first
			// arg is kept for call-site symmetry but is ignored.
			require.NoError(t, writeOTelConfigCommon(ctx, "", templatePath, outPath, false, 0o644))

			content, err := os.ReadFile(outPath)
			require.NoError(t, err)

			assert.Contains(t, string(content), tc.expectedSite)
			assert.NotContains(t, string(content), "${env:DD_SITE}")
		})
	}
}
