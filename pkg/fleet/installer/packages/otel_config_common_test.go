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
		name           string
		apiKey         string
		site           string
		expectedAPIKey string
		expectedSite   string
	}{
		{
			name:           "defaults to datadoghq.com when site is not set",
			apiKey:         "testapikey",
			site:           "",
			expectedAPIKey: "testapikey",
			expectedSite:   "datadoghq.com",
		},
		{
			name:           "uses explicit site when set",
			apiKey:         "testapikey",
			site:           "datadoghq.eu",
			expectedAPIKey: "testapikey",
			expectedSite:   "datadoghq.eu",
		},
		{
			name:           "empty api key leaves placeholder intact",
			apiKey:         "",
			site:           "",
			expectedAPIKey: "${env:DD_API_KEY}",
			expectedSite:   "datadoghq.com",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			dir := t.TempDir()

			templatePath := filepath.Join(dir, "otel-config.yaml.tmpl")
			require.NoError(t, os.WriteFile(templatePath, []byte(template), 0o644))

			outPath := filepath.Join(dir, "otel-config.yaml")
			ctx := HookContext{Context: context.Background()}
			require.NoError(t, writeOTelConfigCommon(ctx, tc.apiKey, tc.site, templatePath, outPath, false, 0o644))

			content, err := os.ReadFile(outPath)
			require.NoError(t, err)

			assert.Contains(t, string(content), tc.expectedSite)
			assert.NotContains(t, string(content), "${env:DD_SITE}")
			if tc.apiKey != "" {
				assert.Contains(t, string(content), tc.expectedAPIKey)
			}
		})
	}
}
