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
	"github.com/DataDog/datadog-agent/comp/host-profiler/collector/impl/extensions/hpflareextension"
	"github.com/DataDog/datadog-agent/comp/host-profiler/collector/impl/receiver"
	ddprofilingextensionimpl "github.com/DataDog/datadog-agent/comp/otelcol/ddprofilingextension/impl"
	"github.com/DataDog/datadog-agent/comp/otelcol/otlp/components/processor/infraattributesprocessor"
	"github.com/open-telemetry/opentelemetry-collector-contrib/processor/attributesprocessor"
	"github.com/open-telemetry/opentelemetry-collector-contrib/processor/cumulativetodeltaprocessor"
	"github.com/open-telemetry/opentelemetry-collector-contrib/processor/k8sattributesprocessor"
	"github.com/open-telemetry/opentelemetry-collector-contrib/processor/resourcedetectionprocessor"
	"github.com/open-telemetry/opentelemetry-collector-contrib/processor/resourceprocessor"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/collector/confmap"
	"go.opentelemetry.io/collector/exporter/debugexporter"
	"go.opentelemetry.io/collector/exporter/otlphttpexporter"
	"go.opentelemetry.io/collector/extension"
	"go.opentelemetry.io/collector/featuregate"
	"go.opentelemetry.io/collector/otelcol"
	"go.opentelemetry.io/collector/otelcol/otelcoltest"
	"go.opentelemetry.io/collector/receiver/otlpreceiver"
	"go.opentelemetry.io/collector/service/telemetry/otelconftelemetry"
	"gopkg.in/yaml.v3"
)

var updateGolden = flag.Bool("update", false, "update golden test files")

func init() {
	// Enable profiles support for all tests (required for profiles pipeline validation)
	_ = featuregate.GlobalRegistry().Set("service.profilesSupport", true)
}

// createTestFactories creates the OTEL factories needed for validation
func createTestFactories(t *testing.T) otelcol.Factories {
	t.Helper()

	receivers, err := otelcol.MakeFactoryMap(
		receiver.NewFactory(),
		otlpreceiver.NewFactory(),
	)
	require.NoError(t, err)

	exporters, err := otelcol.MakeFactoryMap(
		debugexporter.NewFactory(),
		otlphttpexporter.NewFactory(),
	)
	require.NoError(t, err)

	processors, err := otelcol.MakeFactoryMap(
		attributesprocessor.NewFactory(),
		cumulativetodeltaprocessor.NewFactory(),
		infraattributesprocessor.NewFactory(),
		k8sattributesprocessor.NewFactory(),
		resourcedetectionprocessor.NewFactory(),
		resourceprocessor.NewFactory(),
	)
	require.NoError(t, err)

	// Create standalone extensions (without Agent dependencies)
	// Note: hpflareextension is passed nil for ipc component since we're only validating config structure
	extensionFactories := []extension.Factory{
		ddprofilingextensionimpl.NewFactory(),
		hpflareextension.NewFactoryForAgent(nil),
	}
	extensions, err := otelcol.MakeFactoryMap(extensionFactories...)
	require.NoError(t, err)

	return otelcol.Factories{
		Receivers:  receivers,
		Exporters:  exporters,
		Processors: processors,
		Extensions: extensions,
		Telemetry:  otelconftelemetry.NewFactory(),
	}
}

// validateOTelConfig validates an OTEL collector configuration by unmarshaling it
// and calling the OTEL collector's Validate() method
func validateOTelConfig(t *testing.T, conf *confmap.Conf) error {
	t.Helper()

	factories := createTestFactories(t)

	// Write config to a temporary file
	tmpDir := t.TempDir()
	tmpFile := filepath.Join(tmpDir, "otel-config.yaml")
	data, err := yaml.Marshal(conf.ToStringMap())
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(tmpFile, data, 0644))

	// Load and validate the config using otelcoltest
	_, err = otelcoltest.LoadConfigAndValidate(tmpFile, factories)
	return err
}

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
			name:         "multiple-keys-per-endpoint",
			agentConfig:  "provider/multi-keys-per-ep/agent.yaml",
			expectedOTel: "provider/multi-keys-per-ep/otel.yaml",
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
			name:         "profiling-dd-url-precedence",
			agentConfig:  "provider/prof-dd-url-prec/agent.yaml",
			expectedOTel: "provider/prof-dd-url-prec/otel.yaml",
		},
		{
			name:         "invalid-additional-endpoint",
			agentConfig:  "provider/invalid-add-ep/agent.yaml",
			expectedOTel: "provider/invalid-add-ep/otel.yaml",
		},
		{
			name:        "invalid-profiling-dd-url",
			agentConfig: "provider/invalid-profiling-dd-url/agent.yaml",
			shouldError: true,
		},
		{
			name:         "duplicate-site",
			agentConfig:  "provider/duplicate-site/agent.yaml",
			expectedOTel: "provider/duplicate-site/otel.yaml",
		},
		{
			name:         "infer-dc-from-url",
			agentConfig:  "provider/infer-dc-url/agent.yaml",
			expectedOTel: "provider/infer-dc-url/otel.yaml",
		},
		{
			name:         "infer-dc-from-additional-ep",
			agentConfig:  "provider/infer-dc-add-ep/agent.yaml",
			expectedOTel: "provider/infer-dc-add-ep/otel.yaml",
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

			// Validate the OTEL config
			err = validateOTelConfig(t, actual)
			require.NoError(t, err, "OTEL config validation failed")

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

	// Validate the OTEL config
	err = validateOTelConfig(t, actual)
	require.NoError(t, err, "OTEL config validation failed")

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
