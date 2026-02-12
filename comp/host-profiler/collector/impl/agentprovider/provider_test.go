// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

//go:build linux && test

package agentprovider

import (
	"context"
	"flag"
	"os"
	"path/filepath"
	"testing"

	"github.com/DataDog/datadog-agent/comp/core/config"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/collector/confmap"
	"gopkg.in/yaml.v3"
)

var updateGolden = flag.Bool("update", false, "update golden test files")

// loadConfig loads a YAML file and returns a mock config with explicit overrides
// to prevent environment variables from interfering with test values
func loadConfig(t *testing.T, path string) config.Component {
	t.Helper()
	yamlData, err := os.ReadFile(path)
	require.NoError(t, err)

	var data map[string]interface{}
	require.NoError(t, yaml.Unmarshal(yamlData, &data))

	// Create mock with explicit overrides to prevent env var interference
	return config.NewMockWithOverrides(t, data)
}

func TestProvider(t *testing.T) {
	// Unset environment variables that would override test config
	oldSite := os.Getenv("DD_SITE")
	oldAPIKey := os.Getenv("DD_API_KEY")
	os.Unsetenv("DD_SITE")
	os.Unsetenv("DD_API_KEY")
	t.Cleanup(func() {
		if oldSite != "" {
			os.Setenv("DD_SITE", oldSite)
		}
		if oldAPIKey != "" {
			os.Setenv("DD_API_KEY", oldAPIKey)
		}
	})

	tests := []struct {
		name         string
		agentConfig  string
		expectedOTel string
		shouldError  bool
	}{
		{
			name:         "basic-config",
			agentConfig:  "provider/basic-config/agent.yaml",
			expectedOTel: "provider/basic-config/otel.yaml",
		},
		{
			name:         "profiling-dd-url",
			agentConfig:  "provider/profiling-dd-url/agent.yaml",
			expectedOTel: "provider/profiling-dd-url/otel.yaml",
		},
		{
			name:         "multi-keys-per-endpoint",
			agentConfig:  "provider/multi-keys-per-endpoint/agent.yaml",
			expectedOTel: "provider/multi-keys-per-endpoint/otel.yaml",
		},
		{
			name:        "no-site",
			agentConfig: "provider/no-site/agent.yaml",
			shouldError: true,
		},
		{
			name:        "empty-api-key",
			agentConfig: "provider/empty-api-key/agent.yaml",
			shouldError: true,
		},
		{
			name:         "additional-endpoints-only",
			agentConfig:  "provider/additional-endpoints-only/agent.yaml",
			expectedOTel: "provider/additional-endpoints-only/otel.yaml",
		},
		{
			name:         "profiling-url-precedence",
			agentConfig:  "provider/profiling-url-precedence/agent.yaml",
			expectedOTel: "provider/profiling-url-precedence/otel.yaml",
		},
		{
			name:         "invalid-addl-endpoint",
			agentConfig:  "provider/invalid-addl-endpoint/agent.yaml",
			expectedOTel: "provider/invalid-addl-endpoint/otel.yaml",
		},
		{
			name:        "invalid-profiling-dd-url",
			agentConfig: "provider/invalid-profiling-dd-url/agent.yaml",
			shouldError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := loadConfig(t, filepath.Join("td", tt.agentConfig))
			provider := newProvider(cfg)(confmap.ProviderSettings{})

			retrieved, err := provider.Retrieve(context.Background(), "dd:", nil)

			if tt.shouldError {
				require.Error(t, err)
				require.Contains(t, err.Error(), "no valid endpoints configured")
				return
			}

			require.NoError(t, err)

			actual, err := retrieved.AsConf()
			require.NoError(t, err)

			if *updateGolden {
				path := filepath.Join("td", tt.expectedOTel)
				require.NoError(t, os.MkdirAll(filepath.Dir(path), 0755))
				data, err := yaml.Marshal(actual.ToStringMap())
				require.NoError(t, err)
				require.NoError(t, os.WriteFile(path, data, 0644))
				return
			}

			expectedData, err := os.ReadFile(filepath.Join("td", tt.expectedOTel))
			require.NoError(t, err)
			expectedRetrieved, err := confmap.NewRetrievedFromYAML(expectedData)
			require.NoError(t, err)
			expected, err := expectedRetrieved.AsConf()
			require.NoError(t, err)

			require.Equal(t, expected.ToStringMap(), actual.ToStringMap())
		})
	}
}

// TestProviderMultipleEndpoints tests the multiple-endpoints case with structure validation
// instead of golden file comparison due to non-deterministic map iteration order
func TestProviderMultipleEndpoints(t *testing.T) {
	cfg := loadConfig(t, filepath.Join("td", "provider/multiple-endpoints/agent.yaml"))
	provider := newProvider(cfg)(confmap.ProviderSettings{})

	retrieved, err := provider.Retrieve(context.Background(), "dd:", nil)
	require.NoError(t, err)

	actual, err := retrieved.AsConf()
	require.NoError(t, err)

	actualMap := actual.ToStringMap()

	// Validate exporters exist
	exporters := actualMap["exporters"].(map[string]interface{})
	require.NotEmpty(t, exporters, "should have exporters")

	// Validate symbol endpoints
	receivers := actualMap["receivers"].(map[string]interface{})
	hostprofiler := receivers["hostprofiler"].(map[string]interface{})
	symbolUploader := hostprofiler["symbol_uploader"].(map[string]interface{})
	symbolEndpoints := symbolUploader["symbol_endpoints"].([]interface{})
	require.Len(t, symbolEndpoints, 4, "should have 4 symbol endpoints (2 EU + 1 US3 + 1 main)")
}

func TestProviderMethods(t *testing.T) {
	provider := newProvider(config.NewMock(t))(confmap.ProviderSettings{})

	require.Equal(t, "dd", provider.Scheme())

	_, err := provider.Retrieve(context.Background(), "invalid:", nil)
	require.Error(t, err)
	require.Contains(t, err.Error(), "uri is not supported")

	require.NoError(t, provider.Shutdown(context.Background()))
}

func TestExtractSite(t *testing.T) {
	tests := []struct {
		url  string
		want string
	}{
		{"https://intake.profile.datadoghq.com/api/v2/profile", "datadoghq.com"},
		{"https://intake.profile.datadoghq.eu/api/v2/profile", "datadoghq.eu"},
		{"https://intake.profile.us3.datadoghq.com/api/v2/profile", "datadoghq.com"},
		{"not-a-valid-url", ""},
		{"", ""},
		{"file:///local/path", ""},
	}

	for _, tt := range tests {
		require.Equal(t, tt.want, extractSite(tt.url))
	}
}
