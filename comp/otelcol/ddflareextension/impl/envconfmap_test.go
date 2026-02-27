// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

package ddflareextensionimpl

import (
	"context"
	"os"
	"path"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/collector/confmap"
	"go.opentelemetry.io/collector/confmap/provider/envprovider"
	"go.opentelemetry.io/collector/confmap/provider/fileprovider"
	"go.opentelemetry.io/collector/confmap/provider/yamlprovider"
	"go.opentelemetry.io/collector/otelcol"
	"go.yaml.in/yaml/v2"
)

const yamlStr = `exporters:
  datadog:
    api:
      key: ${env:ENV1}
      site: ${env:ENV2}
    hostname: otelcol-docker
    traces:
      span_name_as_resource_name: true`

func TestEnvConfMap_useEnvVarNames(t *testing.T) {
	envConfMap := newEnvConfMapFromYAML(t, yamlStr)

	provided := yamlToMap(t, `exporters:
  datadog:
    api:
      key: REDACTED
      site: datadoghq.com
    hostname: otelcol-docker
    traces:
      span_name_as_resource_name: true`)

	provided = envConfMap.useEnvVarNames(provided)
	expected := `exporters:
  datadog:
    api:
      key: ${env:ENV1}
      site: ${env:ENV2}
    hostname: otelcol-docker
    traces:
      span_name_as_resource_name: true`

	require.Equal(t, expected, mapToYAML(t, provided))
}

func TestEnvConfMap_useEnvVarValues(t *testing.T) {
	envConfMap := newEnvConfMapFromYAML(t, yamlStr)

	provided := yamlToMap(t, `exporters:
  datadog:
    api:
      key: REDACTED
      site: datadoghq.com
    hostname: otelcol-docker
    traces:
      span_name_as_resource_name: true`)

	results := envConfMap.useEnvVarValues(provided)
	expected := `exporters:
  datadog:
    api:
      key: REDACTED
      site: datadoghq.com`

	require.Equal(t, expected, mapToYAML(t, results))

}

func newEnvConfMapFromYAML(t *testing.T, yamlStr string) *envConfMap {
	path := path.Join(t.TempDir(), "otel-config.yaml")

	err := os.WriteFile(path, []byte(yamlStr), 0644)
	require.NoError(t, err)

	ctx := context.Background()
	configProviderSettings := otelcol.ConfigProviderSettings{
		ResolverSettings: confmap.ResolverSettings{
			URIs: []string{path},
			ProviderFactories: []confmap.ProviderFactory{
				fileprovider.NewFactory(),
				envprovider.NewFactory(),
				yamlprovider.NewFactory(),
			},
			ProviderSettings: confmap.ProviderSettings{},
		},
	}

	envConfMap, err := newEnvConfMap(ctx, configProviderSettings)
	require.NoError(t, err)
	return envConfMap
}

func yamlToMap(t *testing.T, yamlStr string) map[string]any {
	var result map[any]any
	err := yaml.Unmarshal([]byte(yamlStr), &result)

	require.NoError(t, err)
	return convertMapKeyAnyToStringAny(result)
}

func convertMapKeyAnyToStringAny(m map[any]any) map[string]any {
	result := make(map[string]any)
	for k, v := range m {
		m, ok := v.(map[any]any)
		if ok {
			result[k.(string)] = convertMapKeyAnyToStringAny(m)
		} else {
			result[k.(string)] = v
		}
	}
	return result
}

func mapToYAML(t *testing.T, data map[string]any) string {
	yamlBytes, err := yaml.Marshal(data)
	require.NoError(t, err)
	return strings.TrimSpace(string(yamlBytes))
}
