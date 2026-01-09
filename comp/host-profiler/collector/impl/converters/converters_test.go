// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

//go:build linux

package converters

import (
	"context"
	"fmt"
	"testing"

	"github.com/DataDog/datadog-agent/comp/core/config"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/collector/confmap"
)

func TestConverterInfraAttributes(t *testing.T) {
	yaml := fmt.Sprintf(`
processors:
  %s:
    enabled: true
  otherProcessor: {}
service:
  pipelines:
    profiles:
      processors:
        - %s
        - otherProcessor
`, infraAttributesName(), infraAttributesName())
	conf := readFromYamlFile(t, yaml)
	require.Equal(t, conf, map[string]any{
		"processors": map[string]any{
			"otherProcessor": map[string]any{},
		},
		"service": map[string]any{
			"pipelines": map[string]any{
				"profiles": map[string]any{
					"processors": []any{"otherProcessor"},
				},
			},
		},
	})
}

func TestConverterNoInfraAttributes(t *testing.T) {
	yaml := `
processors:
  otherProcessor: {}
service:
  pipelines:
    profiles:
      processors:
        - otherProcessor
`
	conf := readFromYamlFile(t, yaml)
	require.Equal(t, conf, map[string]any{
		"processors": map[string]any{
			"otherProcessor": map[string]any{},
		},
		"service": map[string]any{
			"pipelines": map[string]any{
				"profiles": map[string]any{
					"processors": []any{"otherProcessor"},
				},
			},
		},
	})
}

func TestConverterDDProfiling(t *testing.T) {
	yaml := fmt.Sprintf(`
extensions:
  %s: {}
service:
  extensions: [%s]
`, ddprofilingName(), ddprofilingName())

	conf := readFromYamlFile(t, yaml)
	require.Equal(t, conf, map[string]any{
		"extensions": map[string]any{},
		"service": map[string]any{
			"extensions": []any{},
		},
	})
}

func TestConverterHPFlare(t *testing.T) {
	yaml := fmt.Sprintf(`
extensions:
  %s: {}
service:
  extensions: [%s]
`, hpflareName(), hpflareName())

	conf := readFromYamlFile(t, yaml)
	require.Equal(t, conf, map[string]any{
		"extensions": map[string]any{},
		"service": map[string]any{
			"extensions": []any{},
		},
	})
}

func readFromYamlFile(t *testing.T, yamlContent string) map[string]any {
	confRetrieved, err := confmap.NewRetrievedFromYAML([]byte(yamlContent))
	require.NoError(t, err)
	conf, err := confRetrieved.AsConf()
	require.NoError(t, err)
	converter := &converterWithoutAgent{}
	err = converter.Convert(context.Background(), conf)
	require.NoError(t, err)
	return conf.ToStringMap()
}

func readFromYamlFileWithAgent(t *testing.T, yamlContent string, agentConfig config.Component) map[string]any {
	confRetrieved, err := confmap.NewRetrievedFromYAML([]byte(yamlContent))
	require.NoError(t, err, "Failed to create retrieved from YAML")
	conf, err := confRetrieved.AsConf()
	require.NoError(t, err, "Failed to convert to conf")
	converter := &converterWithAgent{config: agentConfig}
	err = converter.Convert(context.Background(), conf)
	require.NoError(t, err, "Failed to convert config")
	return conf.ToStringMap()
}

func TestAPIKeyInferenceAddsKeysWhenMissing(t *testing.T) {
	yaml := `
receivers:
  hostprofiler:
    symbol_uploader:
      enabled: true
      symbol_endpoints:
        - site: datadoghq.com
processors:
  cumulativetodelta: {}
exporters:
  otlphttp:
    metrics_endpoint: https://otlp.datad0g.com/v1/metrics
    profiles_endpoint: https://intake.profile.datad0g.com/v1development/profiles
    headers:
      dd-otel-metric-config: '{"resource_attributes_as_tags": true}'
`
	mockConfig := config.NewMockWithOverrides(t, map[string]interface{}{
		"api_key": "inferred-api-key",
		"app_key": "inferred-app-key",
	})

	conf := readFromYamlFileWithAgent(t, yaml, mockConfig)

	// Verify API keys were added to symbol endpoint
	receivers := conf["receivers"].(map[string]any)
	hostprofiler := receivers["hostprofiler"].(map[string]any)
	symbolUploader := hostprofiler["symbol_uploader"].(map[string]any)
	endpoints := symbolUploader["symbol_endpoints"].([]any)
	endpoint := endpoints[0].(map[string]any)
	require.Equal(t, "inferred-api-key", endpoint["api_key"])
	require.Equal(t, "inferred-app-key", endpoint["app_key"])

	// Verify API key was added to exporter headers
	exporters := conf["exporters"].(map[string]any)
	otlphttp := exporters["otlphttp"].(map[string]any)
	headers := otlphttp["headers"].(map[string]any)
	require.Equal(t, "inferred-api-key", headers["dd-api-key"])
}

func TestAPIKeyInferenceDoesNotOverrideExistingKeys(t *testing.T) {
	yaml := `
receivers:
  hostprofiler:
    symbol_uploader:
      enabled: true
      symbol_endpoints:
        - site: datadoghq.com
          api_key: existing-symbol-api-key
          app_key: existing-symbol-app-key
exporters:
  otlphttp:
    metrics_endpoint: https://otlp.datad0g.com/v1/metrics
    profiles_endpoint: https://intake.profile.datad0g.com/v1development/profiles
    headers:
      dd-api-key: existing-exporter-api-key
      dd-otel-metric-config: '{"resource_attributes_as_tags": true}'
`
	mockConfig := config.NewMockWithOverrides(t, map[string]interface{}{
		"api_key": "inferred-api-key",
		"app_key": "inferred-app-key",
	})

	conf := readFromYamlFileWithAgent(t, yaml, mockConfig)

	// Verify existing API keys in symbol endpoint were NOT overridden
	receivers := conf["receivers"].(map[string]any)
	hostprofiler := receivers["hostprofiler"].(map[string]any)
	symbolUploader := hostprofiler["symbol_uploader"].(map[string]any)
	endpoints := symbolUploader["symbol_endpoints"].([]any)
	endpoint := endpoints[0].(map[string]any)
	require.Equal(t, "existing-symbol-api-key", endpoint["api_key"])
	require.Equal(t, "existing-symbol-app-key", endpoint["app_key"])

	// Verify existing API key in exporter headers was NOT overridden
	exporters := conf["exporters"].(map[string]any)
	otlphttp := exporters["otlphttp"].(map[string]any)
	headers := otlphttp["headers"].(map[string]any)
	require.Equal(t, "existing-exporter-api-key", headers["dd-api-key"])
}
