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

func TestWriteOTelConfigCommonSiteSubstitution(t *testing.T) {
	const template = "endpoint: ${env:DD_SITE}\napi_key: ${env:DD_API_KEY}\n"

	tests := []struct {
		name         string
		datadogYAML  string
		expectedSite string
	}{
		{
			name:         "defaults to datadoghq.com when site is not set",
			datadogYAML:  "api_key: testapikey\n",
			expectedSite: "datadoghq.com",
		},
		{
			name:         "uses explicit site when set",
			datadogYAML:  "api_key: testapikey\nsite: datadoghq.eu\n",
			expectedSite: "datadoghq.eu",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			dir := t.TempDir()

			datadogYamlPath := filepath.Join(dir, "datadog.yaml")
			require.NoError(t, os.WriteFile(datadogYamlPath, []byte(tc.datadogYAML), 0o644))

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
