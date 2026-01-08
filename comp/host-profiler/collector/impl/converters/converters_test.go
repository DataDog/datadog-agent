// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

//go:build linux && test

package converters

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
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

// loadTestData loads a confMap from a YAML file in the testdata directory
func loadTestData(t *testing.T, filename string) confMap {
	t.Helper()
	path := filepath.Join("testdata", filename)
	data, err := os.ReadFile(path)
	require.NoError(t, err, "failed to read test data file: %s", filename)

	// Parse YAML using confmap's YAML parser
	retrieved, err := confmap.NewRetrievedFromYAML(data)
	require.NoError(t, err, "failed to parse YAML from: %s", filename)

	conf, err := retrieved.AsConf()
	require.NoError(t, err, "failed to convert to confmap from: %s", filename)

	return conf.ToStringMap()
}

// newTestConfig creates a mock config for testing
func newTestConfig(t *testing.T) config.Component {
	t.Helper()
	return config.NewMock(t)
}

// Removed - duplicate of TestCheckProcessorsAddsDefaultWhenNoInfraattributes

func TestCheckProcessorsAddsDefaultWhenNoInfraattributes(t *testing.T) {
	cm := loadTestData(t, "adds_default_when_no_infraattributes.yaml")
	conf := confmap.NewFromStringMap(cm)
	converter := &converterWithAgent{config: newTestConfig(t)}
	err := converter.Convert(context.Background(), conf)
	require.NoError(t, err)

	result := conf.ToStringMap()

	// Check that infraattributes/default was added
	allowHostnameOverride, ok := Get[bool](result, "processors::infraattributes/default::allow_hostname_override")
	require.True(t, ok)
	require.Equal(t, true, allowHostnameOverride)

	// Check that batch still exists
	timeout, ok := Get[string](result, "processors::batch::timeout")
	require.True(t, ok)
	require.Equal(t, "10s", timeout)

	// Check both are in the pipeline
	processors, ok := Get[[]any](result, "service::pipelines::profiles::processors")
	require.True(t, ok)
	require.Contains(t, processors, "infraattributes/default")
	require.Contains(t, processors, "batch")
}

func TestCheckProcessorsEnsuresInfraattributesConfig(t *testing.T) {
	cm := loadTestData(t, "ensures_infraattributes_config.yaml")
	conf := confmap.NewFromStringMap(cm)
	converter := &converterWithAgent{config: newTestConfig(t)}
	err := converter.Convert(context.Background(), conf)
	require.NoError(t, err)

	result := conf.ToStringMap()

	// Check that allow_hostname_override was set correctly
	allowHostnameOverride, ok := Get[bool](result, "processors::infraattributes::allow_hostname_override")
	require.True(t, ok)
	require.Equal(t, true, allowHostnameOverride)

	// Check that existing config was preserved
	someOtherConfig, ok := Get[string](result, "processors::infraattributes::some_other_config")
	require.True(t, ok)
	require.Equal(t, "value", someOtherConfig)
}

func TestCheckProcessorsRemovesResourcedetection(t *testing.T) {
	cm := loadTestData(t, "removes_resourcedetection.yaml")
	conf := confmap.NewFromStringMap(cm)
	converter := &converterWithAgent{config: newTestConfig(t)}
	err := converter.Convert(context.Background(), conf)
	require.NoError(t, err)

	result := conf.ToStringMap()

	// Check that resourcedetection was removed
	_, ok := Get[confMap](result, "processors::resourcedetection")
	require.False(t, ok)

	// Check that batch still exists
	_, ok = Get[confMap](result, "processors::batch")
	require.True(t, ok)

	// Check pipeline
	processors, ok := Get[[]any](result, "service::pipelines::profiles::processors")
	require.True(t, ok)
	require.NotContains(t, processors, "resourcedetection")
	require.Contains(t, processors, "batch")
}

func TestCheckProcessorsRemovesResourcedetectionCustomName(t *testing.T) {
	cm := loadTestData(t, "removes_resourcedetection_custom_name.yaml")
	conf := confmap.NewFromStringMap(cm)
	converter := &converterWithAgent{config: newTestConfig(t)}
	err := converter.Convert(context.Background(), conf)
	require.NoError(t, err)

	result := conf.ToStringMap()

	// Check that resourcedetection/custom was removed
	_, ok := Get[confMap](result, "processors::resourcedetection/custom")
	require.False(t, ok)

	// Check that infraattributes still exists
	allowHostnameOverride, ok := Get[bool](result, "processors::infraattributes::allow_hostname_override")
	require.True(t, ok)
	require.Equal(t, true, allowHostnameOverride)

	// Check pipeline
	processors, ok := Get[[]any](result, "service::pipelines::profiles::processors")
	require.True(t, ok)
	require.NotContains(t, processors, "resourcedetection/custom")
	require.Contains(t, processors, "infraattributes")
}

func TestCheckProcessorsHandlesInfraattributesCustomName(t *testing.T) {
	cm := loadTestData(t, "handles_infraattributes_custom_name.yaml")
	conf := confmap.NewFromStringMap(cm)
	converter := &converterWithAgent{config: newTestConfig(t)}
	err := converter.Convert(context.Background(), conf)
	require.NoError(t, err)

	result := conf.ToStringMap()

	// Check that allow_hostname_override was set on custom infraattributes
	allowHostnameOverride, ok := Get[bool](result, "processors::infraattributes/custom::allow_hostname_override")
	require.True(t, ok)
	require.Equal(t, true, allowHostnameOverride)

	// Check that default infraattributes was not added
	_, ok = Get[confMap](result, "processors::infraattributes/default")
	require.False(t, ok)

	// Check pipeline
	processors, ok := Get[[]any](result, "service::pipelines::profiles::processors")
	require.True(t, ok)
	require.NotContains(t, processors, "infraattributes/default")
	require.Contains(t, processors, "infraattributes/custom")
}

func TestCheckReceiversAddsHostprofilerWhenMissing(t *testing.T) {
	cm := loadTestData(t, "adds_hostprofiler_when_missing.yaml")
	conf := confmap.NewFromStringMap(cm)
	converter := &converterWithAgent{config: newTestConfig(t)}
	err := converter.Convert(context.Background(), conf)
	require.NoError(t, err)

	result := conf.ToStringMap()

	// Check that hostprofiler was added with symbol_uploader disabled
	enabled, ok := Get[bool](result, "receivers::hostprofiler::symbol_uploader::enabled")
	require.True(t, ok)
	require.Equal(t, false, enabled)

	// Check that hostprofiler was added to pipeline
	receivers, ok := Get[[]any](result, "service::pipelines::profiles::receivers")
	require.True(t, ok)
	require.Contains(t, receivers, "hostprofiler")
}

func TestCheckReceiversPreservesOtlpProtocols(t *testing.T) {
	cm := loadTestData(t, "preserves_otlp_protocols.yaml")
	conf := confmap.NewFromStringMap(cm)
	converter := &converterWithAgent{config: newTestConfig(t)}
	err := converter.Convert(context.Background(), conf)
	require.NoError(t, err)

	result := conf.ToStringMap()

	// Check that existing OTLP protocol config is preserved
	endpoint, ok := Get[string](result, "receivers::otlp::protocols::grpc::endpoint")
	require.True(t, ok)
	require.Equal(t, "0.0.0.0:4317", endpoint)
}

func TestCheckReceiversCreatesDefaultHostprofiler(t *testing.T) {
	cm := loadTestData(t, "creates_default_hostprofiler.yaml")
	conf := confmap.NewFromStringMap(cm)
	converter := &converterWithAgent{config: newTestConfig(t)}
	err := converter.Convert(context.Background(), conf)
	require.NoError(t, err)

	result := conf.ToStringMap()

	// Check that hostprofiler was created with symbol_uploader disabled
	enabled, ok := Get[bool](result, "receivers::hostprofiler::symbol_uploader::enabled")
	require.True(t, ok)
	require.Equal(t, false, enabled)
}

func TestCheckReceiversSymbolUploaderDisabled(t *testing.T) {
	cm := loadTestData(t, "symbol_uploader_disabled.yaml")
	conf := confmap.NewFromStringMap(cm)
	converter := &converterWithAgent{config: newTestConfig(t)}
	err := converter.Convert(context.Background(), conf)
	require.NoError(t, err)

	result := conf.ToStringMap()

	// Check that symbol_uploader remains disabled
	enabled, ok := Get[bool](result, "receivers::hostprofiler::symbol_uploader::enabled")
	require.True(t, ok)
	require.Equal(t, false, enabled)
}

func TestCheckReceiversSymbolUploaderWithStringKeys(t *testing.T) {
	cm := loadTestData(t, "symbol_uploader_with_string_keys.yaml")
	conf := confmap.NewFromStringMap(cm)
	converter := &converterWithAgent{config: newTestConfig(t)}
	err := converter.Convert(context.Background(), conf)
	require.NoError(t, err)

	result := conf.ToStringMap()

	// Get symbol endpoints and check the first endpoint
	endpoints, ok := Get[[]any](result, "receivers::hostprofiler::symbol_uploader::symbol_endpoints")
	require.True(t, ok)
	require.Len(t, endpoints, 1)

	endpoint := endpoints[0].(confMap)
	require.Equal(t, "test-key", endpoint["api_key"])
	require.Equal(t, "test-app-key", endpoint["app_key"])
}

func TestCheckReceiversConvertsNonStringApiKey(t *testing.T) {
	cm := loadTestData(t, "converts_non_string_api_key.yaml")
	conf := confmap.NewFromStringMap(cm)
	converter := &converterWithAgent{config: newTestConfig(t)}
	err := converter.Convert(context.Background(), conf)
	require.NoError(t, err)

	result := conf.ToStringMap()

	// Check that api_key was converted to string
	endpoints, ok := Get[[]any](result, "receivers::hostprofiler::symbol_uploader::symbol_endpoints")
	require.True(t, ok)
	endpoint := endpoints[0].(confMap)
	require.Equal(t, "12345", endpoint["api_key"])
}

func TestCheckReceiversConvertsNonStringAppKey(t *testing.T) {
	cm := loadTestData(t, "converts_non_string_app_key.yaml")
	conf := confmap.NewFromStringMap(cm)
	converter := &converterWithAgent{config: newTestConfig(t)}
	err := converter.Convert(context.Background(), conf)
	require.NoError(t, err)

	result := conf.ToStringMap()

	// Check that app_key was converted to string
	endpoints, ok := Get[[]any](result, "receivers::hostprofiler::symbol_uploader::symbol_endpoints")
	require.True(t, ok)
	endpoint := endpoints[0].(confMap)
	require.Equal(t, "67890", endpoint["app_key"])
}

func TestCheckReceiversAddsHostprofilerToPipeline(t *testing.T) {
	cm := loadTestData(t, "adds_hostprofiler_to_pipeline.yaml")
	conf := confmap.NewFromStringMap(cm)
	converter := &converterWithAgent{config: newTestConfig(t)}
	err := converter.Convert(context.Background(), conf)
	require.NoError(t, err)

	result := conf.ToStringMap()

	// Verify hostprofiler was added to pipeline
	receivers, ok := Get[[]any](result, "service::pipelines::profiles::receivers")
	require.True(t, ok)
	require.Contains(t, receivers, "hostprofiler")
	require.Contains(t, receivers, "otlp")

	// Verify hostprofiler config was created
	enabled, ok := Get[bool](result, "receivers::hostprofiler::symbol_uploader::enabled")
	require.True(t, ok)
	require.Equal(t, false, enabled)
}

func TestCheckReceiversMultipleSymbolEndpoints(t *testing.T) {
	cm := loadTestData(t, "multiple_symbol_endpoints.yaml")
	conf := confmap.NewFromStringMap(cm)
	converter := &converterWithAgent{config: newTestConfig(t)}
	err := converter.Convert(context.Background(), conf)
	require.NoError(t, err)

	result := conf.ToStringMap()

	// Check that both endpoints were processed correctly
	endpoints, ok := Get[[]any](result, "receivers::hostprofiler::symbol_uploader::symbol_endpoints")
	require.True(t, ok)
	require.Len(t, endpoints, 2)

	endpoint1 := endpoints[0].(confMap)
	require.Equal(t, "key1", endpoint1["api_key"])
	require.Equal(t, "app1", endpoint1["app_key"])

	endpoint2 := endpoints[1].(confMap)
	require.Equal(t, "123", endpoint2["api_key"])
	require.Equal(t, "456", endpoint2["app_key"])
}

func TestCheckOtlpHttpExporterEnsuresHeaders(t *testing.T) {
	cm := loadTestData(t, "ensures_headers.yaml")
	conf := confmap.NewFromStringMap(cm)
	converter := &converterWithAgent{config: newTestConfig(t)}
	err := converter.Convert(context.Background(), conf)
	require.NoError(t, err)

	result := conf.ToStringMap()

	// Check that headers was created
	_, ok := Get[confMap](result, "exporters::otlphttp::headers")
	require.True(t, ok)
}

func TestCheckOtlpHttpExporterWithStringApiKey(t *testing.T) {
	cm := loadTestData(t, "otlphttp_with_string_api_key.yaml")
	conf := confmap.NewFromStringMap(cm)
	converter := &converterWithAgent{config: newTestConfig(t)}
	err := converter.Convert(context.Background(), conf)
	require.NoError(t, err)

	result := conf.ToStringMap()

	// Check that dd-api-key is preserved as string
	apiKey, ok := Get[string](result, "exporters::otlphttp::headers::dd-api-key")
	require.True(t, ok)
	require.Equal(t, "test-api-key", apiKey)
}

func TestCheckOtlpHttpExporterConvertsNonStringApiKey(t *testing.T) {
	cm := loadTestData(t, "otlphttp_converts_non_string_api_key.yaml")
	conf := confmap.NewFromStringMap(cm)
	converter := &converterWithAgent{config: newTestConfig(t)}
	err := converter.Convert(context.Background(), conf)
	require.NoError(t, err)

	result := conf.ToStringMap()

	// Check that dd-api-key was converted to string
	apiKey, ok := Get[string](result, "exporters::otlphttp::headers::dd-api-key")
	require.True(t, ok)
	require.Equal(t, "12345", apiKey)
}

func TestCheckOtlpHttpExporterMultipleExporters(t *testing.T) {
	cm := loadTestData(t, "multiple_otlphttp_exporters.yaml")
	conf := confmap.NewFromStringMap(cm)
	converter := &converterWithAgent{config: newTestConfig(t)}
	err := converter.Convert(context.Background(), conf)
	require.NoError(t, err)

	result := conf.ToStringMap()

	// Check prod exporter api key was converted to string
	prodApiKey, ok := Get[string](result, "exporters::otlphttp/prod::headers::dd-api-key")
	require.True(t, ok)
	require.Equal(t, "11111", prodApiKey)

	// Check staging exporter api key is preserved as string
	stagingApiKey, ok := Get[string](result, "exporters::otlphttp/staging::headers::dd-api-key")
	require.True(t, ok)
	require.Equal(t, "staging-key", stagingApiKey)

	// Check that logging exporter still exists
	_, ok = Get[confMap](result, "exporters::logging")
	require.True(t, ok)
}

func TestCheckOtlpHttpExporterIgnoresNonOtlpHttp(t *testing.T) {
	cm := loadTestData(t, "ignores_non_otlphttp.yaml")
	conf := confmap.NewFromStringMap(cm)
	converter := &converterWithAgent{config: newTestConfig(t)}
	err := converter.Convert(context.Background(), conf)
	require.NoError(t, err)

	result := conf.ToStringMap()

	// Check that non-otlphttp exporters are preserved
	_, ok := Get[confMap](result, "exporters::logging")
	require.True(t, ok)

	_, ok = Get[confMap](result, "exporters::debug")
	require.True(t, ok)
}

func TestCheckExportersErrorsWhenNoOtlpHttp(t *testing.T) {
	cm := loadTestData(t, "errors_when_no_otlphttp.yaml")
	conf := confmap.NewFromStringMap(cm)
	converter := &converterWithAgent{config: newTestConfig(t)}
	err := converter.Convert(context.Background(), conf)
	require.Error(t, err)
	require.Contains(t, err.Error(), "no otlphttp exporter configured")
}

// ============================================================================
// Edge Cases & Tricky Scenarios
// ============================================================================

func TestProcessorsOverridesAllowHostnameOverrideToTrue(t *testing.T) {
	// Test that even if allow_hostname_override is explicitly set to false, we override it to true
	cm := loadTestData(t, "overrides_allow_hostname_override_to_true.yaml")
	conf := confmap.NewFromStringMap(cm)
	converter := &converterWithAgent{config: newTestConfig(t)}
	err := converter.Convert(context.Background(), conf)
	require.NoError(t, err)

	result := conf.ToStringMap()

	// Should be overridden to true
	allowHostnameOverride, ok := Get[bool](result, "processors::infraattributes::allow_hostname_override")
	require.True(t, ok)
	require.Equal(t, true, allowHostnameOverride)

	// Other config should be preserved
	someConfig, ok := Get[string](result, "processors::infraattributes::some_config")
	require.True(t, ok)
	require.Equal(t, "value", someConfig)
}

func TestProcessorsWithBothDefaultAndCustomInfraattributes(t *testing.T) {
	// Edge case: both infraattributes and infraattributes/custom in pipeline
	cm := loadTestData(t, "both_default_and_custom_infraattributes.yaml")
	conf := confmap.NewFromStringMap(cm)
	converter := &converterWithAgent{config: newTestConfig(t)}
	err := converter.Convert(context.Background(), conf)
	require.NoError(t, err)

	result := conf.ToStringMap()

	// Both should have allow_hostname_override set to true
	allowHostnameOverride1, ok := Get[bool](result, "processors::infraattributes::allow_hostname_override")
	require.True(t, ok)
	require.Equal(t, true, allowHostnameOverride1)

	allowHostnameOverride2, ok := Get[bool](result, "processors::infraattributes/custom::allow_hostname_override")
	require.True(t, ok)
	require.Equal(t, true, allowHostnameOverride2)
}

func TestProcessorsWithMultipleResourcedetectionProcessors(t *testing.T) {
	// Multiple resourcedetection processors with different names - all should be removed
	cm := loadTestData(t, "multiple_resourcedetection_processors.yaml")
	conf := confmap.NewFromStringMap(cm)
	converter := &converterWithAgent{config: newTestConfig(t)}
	err := converter.Convert(context.Background(), conf)
	require.NoError(t, err)

	result := conf.ToStringMap()

	// All resourcedetection processors should be removed
	_, ok := Get[confMap](result, "processors::resourcedetection")
	require.False(t, ok)
	_, ok = Get[confMap](result, "processors::resourcedetection/system")
	require.False(t, ok)
	_, ok = Get[confMap](result, "processors::resourcedetection/cloud")
	require.False(t, ok)

	// Batch should remain
	_, ok = Get[confMap](result, "processors::batch")
	require.True(t, ok)

	// Pipeline should only have batch and infraattributes/default
	processors, ok := Get[[]any](result, "service::pipelines::profiles::processors")
	require.True(t, ok)
	require.NotContains(t, processors, "resourcedetection")
	require.NotContains(t, processors, "resourcedetection/system")
	require.NotContains(t, processors, "resourcedetection/cloud")
	require.Contains(t, processors, "batch")
	require.Contains(t, processors, "infraattributes/default")
}

func TestReceiversSymbolUploaderEnabledWithEmptyEndpoints(t *testing.T) {
	// Edge case: symbol_uploader enabled but endpoints list is empty
	cm := loadTestData(t, "symbol_uploader_enabled_with_empty_endpoints.yaml")
	conf := confmap.NewFromStringMap(cm)
	converter := &converterWithAgent{config: newTestConfig(t)}
	err := converter.Convert(context.Background(), conf)
	require.NoError(t, err)

	result := conf.ToStringMap()

	// Should preserve empty endpoints list
	endpoints, ok := Get[[]any](result, "receivers::hostprofiler::symbol_uploader::symbol_endpoints")
	require.True(t, ok)
	require.Empty(t, endpoints)
}

func TestReceiversSymbolUploaderWithMixedEndpointTypes(t *testing.T) {
	// Edge case: Some endpoints have string keys, some have numeric keys
	cm := loadTestData(t, "symbol_uploader_with_mixed_endpoint_types.yaml")
	conf := confmap.NewFromStringMap(cm)
	converter := &converterWithAgent{config: newTestConfig(t)}
	err := converter.Convert(context.Background(), conf)
	require.NoError(t, err)

	result := conf.ToStringMap()

	endpoints, ok := Get[[]any](result, "receivers::hostprofiler::symbol_uploader::symbol_endpoints")
	require.True(t, ok)
	require.Len(t, endpoints, 3)

	// First endpoint - mixed types
	ep1 := endpoints[0].(confMap)
	require.Equal(t, "string-key", ep1["api_key"])
	require.Equal(t, "12345", ep1["app_key"])

	// Second endpoint - mixed types
	ep2 := endpoints[1].(confMap)
	require.Equal(t, "67890", ep2["api_key"])
	require.Equal(t, "string-app", ep2["app_key"])

	// Third endpoint - missing keys get filled from config
	ep3 := endpoints[2].(confMap)
	require.Equal(t, "http://example.com", ep3["url"])
	// The converter fills in api_key and app_key from config defaults
	_, hasApiKey := ep3["api_key"]
	require.True(t, hasApiKey) // Now filled from config
	_, hasAppKey := ep3["app_key"]
	require.True(t, hasAppKey) // Now filled from config
}

func TestExportersMultipleOtlpHttpWithMixedKeys(t *testing.T) {
	// Multiple otlphttp exporters with various key types
	cm := loadTestData(t, "multiple_otlphttp_with_mixed_keys.yaml")
	conf := confmap.NewFromStringMap(cm)
	converter := &converterWithAgent{config: newTestConfig(t)}
	err := converter.Convert(context.Background(), conf)
	require.NoError(t, err)

	result := conf.ToStringMap()

	// Prod: should convert numeric key
	prodApiKey, ok := Get[string](result, "exporters::otlphttp/prod::headers::dd-api-key")
	require.True(t, ok)
	require.Equal(t, "12345", prodApiKey)

	// Prod: custom header should be preserved
	customHeader, ok := Get[string](result, "exporters::otlphttp/prod::headers::custom")
	require.True(t, ok)
	require.Equal(t, "header", customHeader)

	// Staging: headers should be created
	_, ok = Get[confMap](result, "exporters::otlphttp/staging::headers")
	require.True(t, ok)

	// Dev: string key should be preserved
	devApiKey, ok := Get[string](result, "exporters::otlphttp/dev::headers::dd-api-key")
	require.True(t, ok)
	require.Equal(t, "already-string", devApiKey)
}

func TestEmptyPipeline(t *testing.T) {
	// Edge case: Empty everything in pipeline
	cm := loadTestData(t, "empty_pipeline.yaml")
	conf := confmap.NewFromStringMap(cm)
	converter := &converterWithAgent{config: newTestConfig(t)}
	err := converter.Convert(context.Background(), conf)

	// Should error - no otlphttp exporter
	require.Error(t, err)
	require.Contains(t, err.Error(), "no otlphttp exporter configured")
}

func TestMissingServiceSection(t *testing.T) {
	// Edge case: No service section at all
	cm := loadTestData(t, "missing_service_section.yaml")
	conf := confmap.NewFromStringMap(cm)
	converter := &converterWithAgent{config: newTestConfig(t)}
	err := converter.Convert(context.Background(), conf)

	// Should not fail because there are is an otlphttp component configured: we can infer profiles' exporter pipeline
	require.NoError(t, err)
}

func TestNonStringProcessorNameInPipeline(t *testing.T) {
	// Edge case: Non-string value in processors list (should be handled gracefully)
	cm := loadTestData(t, "non_string_processor_name_in_pipeline.yaml")
	conf := confmap.NewFromStringMap(cm)
	converter := &converterWithAgent{config: newTestConfig(t)}
	err := converter.Convert(context.Background(), conf)

	// Should error on the first non-string processor (123)
	require.Error(t, err)
	require.Contains(t, err.Error(), "processor name must be a string")
}

func TestReceiverConfigIsNotMap(t *testing.T) {
	// Tricky: hostprofiler receiver exists but config is not a map
	cm := loadTestData(t, "receiver_config_is_not_map.yaml")
	conf := confmap.NewFromStringMap(cm)
	converter := &converterWithAgent{config: newTestConfig(t)}
	err := converter.Convert(context.Background(), conf)

	// Should return an error with proper type checking
	require.Error(t, err)
	require.Contains(t, err.Error(), "hostprofiler config should be a map")
}

func TestSymbolEndpointsExistsButWrongType(t *testing.T) {
	// Tricky: symbol_uploader.enabled=true but endpoints is a string, not a list
	// Ensure silently replaces wrong-typed values with correct empty types
	cm := loadTestData(t, "symbol_endpoints_exists_but_wrong_type.yaml")
	conf := confmap.NewFromStringMap(cm)
	converter := &converterWithAgent{config: newTestConfig(t)}
	err := converter.Convert(context.Background(), conf)

	// Ensure[[]any] replaces the string with an empty list - no error
	require.NoError(t, err)

	result := conf.ToStringMap()

	// The invalid string should have been replaced with an empty list
	endpoints, ok := Get[[]any](result, "receivers::hostprofiler::symbol_uploader::symbol_endpoints")
	require.True(t, ok)
	require.Empty(t, endpoints)
}

func TestHeadersExistButWrongType(t *testing.T) {
	// Tricky: exporter headers exist but are a string, not a map
	// Ensure silently replaces wrong-typed values with correct empty types
	cm := loadTestData(t, "headers_exist_but_wrong_type.yaml")
	conf := confmap.NewFromStringMap(cm)
	converter := &converterWithAgent{config: newTestConfig(t)}
	err := converter.Convert(context.Background(), conf)

	// Ensure[confMap] replaces the string with an empty map - no error
	require.NoError(t, err)

	result := conf.ToStringMap()

	// The invalid string should have been replaced with a map
	headers, ok := Get[confMap](result, "exporters::otlphttp::headers")
	require.True(t, ok)
	require.NotNil(t, headers)

	// ensureStringKey fills in dd-api-key from config when it doesn't exist
	// So after replacement, the headers map will have the default api key from config
	require.NotEmpty(t, headers) // Now contains dd-api-key from config
	_, hasApiKey := headers["dd-api-key"]
	require.True(t, hasApiKey) // Filled from config
}

func TestEmptyStringProcessorName(t *testing.T) {
	// Tricky: processor name is an empty string
	cm := loadTestData(t, "empty_string_processor_name.yaml")
	conf := confmap.NewFromStringMap(cm)
	converter := &converterWithAgent{config: newTestConfig(t)}
	err := converter.Convert(context.Background(), conf)
	require.NoError(t, err)

	result := conf.ToStringMap()

	// Empty string should be preserved, infraattributes should be added
	processorNames, ok := Get[[]any](result, "service::pipelines::profiles::processors")
	require.True(t, ok)
	require.Contains(t, processorNames, "")
	require.Contains(t, processorNames, "infraattributes/default")
}

func TestProcessorNameSimilarButNotExactMatch(t *testing.T) {
	// TODO: Should use proper OTEL type/id parsing (e.g., strings.HasPrefix with "/" check)
	// In OTEL specs, components must use type/id format (e.g., infraattributes/custom)
	cm := loadTestData(t, "processor_name_similar_but_not_exact_match.yaml")
	conf := confmap.NewFromStringMap(cm)
	converter := &converterWithAgent{config: newTestConfig(t)}
	err := converter.Convert(context.Background(), conf)
	require.NoError(t, err)

	result := conf.ToStringMap()

	processorNames, ok := Get[[]any](result, "service::pipelines::profiles::processors")
	require.True(t, ok)

	// Current behavior: myresourcedetection gets removed (contains "resourcedetection")
	// TODO: This is wrong - should only match "resourcedetection" or "resourcedetection/*"
	require.NotContains(t, processorNames, "myresourcedetection")

	// Current behavior: infraattributes_custom is treated as infraattributes type (contains "infraattributes")
	// TODO: This is wrong - should only match "infraattributes" or "infraattributes/*"
	require.Contains(t, processorNames, "infraattributes_custom")

	// Verify it was incorrectly treated as infraattributes (allow_hostname_override was added)
	allowHostnameOverride, ok := Get[bool](result, "processors::infraattributes_custom::allow_hostname_override")
	require.True(t, ok)
	require.Equal(t, true, allowHostnameOverride)

	// batch should remain
	require.Contains(t, processorNames, "batch")

	// Since converter thinks it found infraattributes, default is NOT added
	require.NotContains(t, processorNames, "infraattributes/default")
}

func TestGlobalProcessorsSectionIsNotMap(t *testing.T) {
	// Tricky: processors section exists but is a string, not a map
	// Ensure silently replaces wrong-typed values with correct empty types
	cm := loadTestData(t, "global_processors_section_is_not_map.yaml")
	conf := confmap.NewFromStringMap(cm)
	converter := &converterWithAgent{config: newTestConfig(t)}
	err := converter.Convert(context.Background(), conf)

	// Ensure[confMap] replaces the string with an empty map - no error
	require.NoError(t, err)

	result := conf.ToStringMap()

	// The invalid string should have been replaced with a valid map
	processors, ok := Get[confMap](result, "processors")
	require.True(t, ok)
	require.NotNil(t, processors)

	// infraattributes/default should have been added
	_, exists := processors["infraattributes/default"]
	require.True(t, exists)
}
