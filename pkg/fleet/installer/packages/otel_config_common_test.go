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
			ctx := HookContext{Context: context.Background()}
			require.NoError(t, writeOTelConfigCommon(ctx, datadogYamlPath, templatePath, outPath, false, 0o644))

			content, err := os.ReadFile(outPath)
			require.NoError(t, err)

			assert.Contains(t, string(content), tc.expectedSite)
			assert.NotContains(t, string(content), "${env:DD_SITE}")
		})
	}
}
