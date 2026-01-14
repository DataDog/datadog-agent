// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

//go:build linux && test

package converters

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/collector/confmap"
)

func TestWithoutAgentProcessorNameSimilarButNotExactMatch(t *testing.T) {
	// Tests that similar names don't match - uses proper OTEL type/id parsing
	result := loadAsStandaloneMode(t, "wo_agent_proc_name_similar_not_exact.yaml")

	processorNames, ok := Get[[]any](result, "service::pipelines::profiles::processors")
	require.True(t, ok)

	// Correct behavior: myinfraattributes stays (not "infraattributes" or "infraattributes/*")
	require.Contains(t, processorNames, "myinfraattributes")

	// Correct behavior: infraattributes_custom stays unchanged (not "infraattributes" or "infraattributes/*")
	require.Contains(t, processorNames, "infraattributes_custom")

	// batch should remain
	require.Contains(t, processorNames, "batch")

	// Since no valid resourcedetection found, default SHOULD be added
	require.Contains(t, processorNames, "resourcedetection/default")

	// Verify resourcedetection/default was configured correctly
	detectors, ok := Get[[]any](result, "processors::resourcedetection/default::detectors")
	require.True(t, ok)
	require.Contains(t, detectors, "system")
}

func TestWithoutAgentRemovesInfraattributesFromMetricsPipeline(t *testing.T) {
	// Test that infraattributes is removed from metrics pipeline, not just profiles pipeline
	result := loadAsStandaloneMode(t, "wo_agent_removes_infraattrs_metrics.yaml")

	// Check that infraattributes processors were removed from global config
	_, ok := Get[confMap](result, "processors::infraattributes")
	require.False(t, ok)
	_, ok = Get[confMap](result, "processors::infraattributes/custom")
	require.False(t, ok)

	// Check that batch still exists
	_, ok = Get[confMap](result, "processors::batch")
	require.True(t, ok)

	// Check that resourcedetection/default was added to profiles pipeline
	profilesProcessors, ok := Get[[]any](result, "service::pipelines::profiles::processors")
	require.True(t, ok)
	require.Contains(t, profilesProcessors, "resourcedetection/default")
	require.Contains(t, profilesProcessors, "batch")

	// Check that infraattributes processors were ALSO removed from metrics pipeline
	metricsProcessors, ok := Get[[]any](result, "service::pipelines::metrics::processors")
	require.True(t, ok)
	require.NotContains(t, metricsProcessors, "infraattributes")
	require.NotContains(t, metricsProcessors, "infraattributes/custom")
	require.Contains(t, metricsProcessors, "batch")
}

func TestWithoutAgentCheckReceiversAddsHostprofilerWhenMissing(t *testing.T) {
	result := loadAsStandaloneMode(t, "adds_hostprofiler_when_missing.yaml")

	// Check that hostprofiler was added with symbol_uploader disabled
	enabled, ok := Get[bool](result, "receivers::hostprofiler::symbol_uploader::enabled")
	require.True(t, ok)
	require.Equal(t, false, enabled)

	// Check that hostprofiler was added to pipeline
	receivers, ok := Get[[]any](result, "service::pipelines::profiles::receivers")
	require.True(t, ok)
	require.Contains(t, receivers, "hostprofiler")
}

func TestWithoutAgentCheckReceiversPreservesOtlpProtocols(t *testing.T) {
	result := loadAsStandaloneMode(t, "preserves_otlp_protocols.yaml")

	// Check that existing OTLP protocol config is preserved
	endpoint, ok := Get[string](result, "receivers::otlp::protocols::grpc::endpoint")
	require.True(t, ok)
	require.Equal(t, "0.0.0.0:4317", endpoint)
}

func TestWithoutAgentCheckReceiversCreatesDefaultHostprofiler(t *testing.T) {
	result := loadAsStandaloneMode(t, "creates_default_hostprofiler.yaml")

	// Check that hostprofiler was created with symbol_uploader disabled
	enabled, ok := Get[bool](result, "receivers::hostprofiler::symbol_uploader::enabled")
	require.True(t, ok)
	require.Equal(t, false, enabled)
}

func TestWithoutAgentCheckReceiversSymbolUploaderDisabled(t *testing.T) {
	result := loadAsStandaloneMode(t, "symbol_uploader_disabled.yaml")

	// Check that symbol_uploader remains disabled
	enabled, ok := Get[bool](result, "receivers::hostprofiler::symbol_uploader::enabled")
	require.True(t, ok)
	require.Equal(t, false, enabled)
}

func TestWithoutAgentCheckReceiversSymbolUploaderWithStringKeys(t *testing.T) {
	result := loadAsStandaloneMode(t, "symbol_uploader_with_string_keys.yaml")

	// Get symbol endpoints and check the first endpoint
	endpoints, ok := Get[[]any](result, "receivers::hostprofiler::symbol_uploader::symbol_endpoints")
	require.True(t, ok)
	require.Len(t, endpoints, 1)

	endpoint := endpoints[0].(confMap)
	require.Equal(t, "test-key", endpoint["api_key"])
	require.Equal(t, "test-app-key", endpoint["app_key"])
}

func TestWithoutAgentCheckReceiversConvertsNonStringApiKey(t *testing.T) {
	result := loadAsStandaloneMode(t, "converts_non_string_api_key.yaml")

	// Check that api_key was converted to string
	endpoints, ok := Get[[]any](result, "receivers::hostprofiler::symbol_uploader::symbol_endpoints")
	require.True(t, ok)
	endpoint := endpoints[0].(confMap)
	require.Equal(t, "12345", endpoint["api_key"])
}

func TestWithoutAgentCheckReceiversConvertsNonStringAppKey(t *testing.T) {
	result := loadAsStandaloneMode(t, "converts_non_string_app_key.yaml")

	// Check that app_key was converted to string
	endpoints, ok := Get[[]any](result, "receivers::hostprofiler::symbol_uploader::symbol_endpoints")
	require.True(t, ok)
	endpoint := endpoints[0].(confMap)
	require.Equal(t, "67890", endpoint["app_key"])
}

func TestWithoutAgentCheckReceiversAddsHostprofilerToPipeline(t *testing.T) {
	result := loadAsStandaloneMode(t, "adds_hostprofiler_to_pipeline.yaml")

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

func TestWithoutAgentCheckReceiversMultipleSymbolEndpoints(t *testing.T) {
	result := loadAsStandaloneMode(t, "multiple_symbol_endpoints.yaml")

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

func TestWithoutAgentCheckReceiversNonStringReceiverName(t *testing.T) {
	// Test that non-string receiver names in pipeline are rejected
	path := filepath.Join("testdata", "non_string_receiver_name_in_pipeline.yaml")
	data, err := os.ReadFile(path)
	require.NoError(t, err)

	retrieved, err := confmap.NewRetrievedFromYAML(data)
	require.NoError(t, err)

	conf, err := retrieved.AsConf()
	require.NoError(t, err)

	converter := &converterWithoutAgent{}
	err = converter.Convert(context.Background(), conf)

	require.Error(t, err)
	require.Contains(t, err.Error(), "receiver name must be a string")
}

func TestWithoutAgentCheckReceiversMultipleHostprofilers(t *testing.T) {
	// Test that multiple hostprofiler receivers in pipeline are all processed
	result := loadAsStandaloneMode(t, "multiple_hostprofiler_receivers.yaml")

	// Check first hostprofiler - keys should be converted to strings
	endpoints1, ok := Get[[]any](result, "receivers::hostprofiler::symbol_uploader::symbol_endpoints")
	require.True(t, ok)
	require.Len(t, endpoints1, 1)
	ep1 := endpoints1[0].(confMap)
	require.Equal(t, "11111", ep1["api_key"])
	require.Equal(t, "22222", ep1["app_key"])

	// Check second hostprofiler/custom - keys should be converted to strings
	endpoints2, ok := Get[[]any](result, "receivers::hostprofiler/custom::symbol_uploader::symbol_endpoints")
	require.True(t, ok)
	require.Len(t, endpoints2, 1)
	ep2 := endpoints2[0].(confMap)
	require.Equal(t, "33333", ep2["api_key"])
	require.Equal(t, "string-app", ep2["app_key"])
}

func TestWithoutAgentCheckReceiversSymbolEndpointsWrongType(t *testing.T) {
	// Test that symbol_endpoints with wrong type (string not list) returns error
	path := filepath.Join("testdata", "symbol_endpoints_wrong_type.yaml")
	data, err := os.ReadFile(path)
	require.NoError(t, err)

	retrieved, err := confmap.NewRetrievedFromYAML(data)
	require.NoError(t, err)

	conf, err := retrieved.AsConf()
	require.NoError(t, err)

	converter := &converterWithoutAgent{}
	err = converter.Convert(context.Background(), conf)

	require.Error(t, err)
	require.Contains(t, err.Error(), "symbol_endpoints must be a list")
}

func TestWithoutAgentReceiversSymbolUploaderEnabledWithEmptyEndpoints(t *testing.T) {
	// Edge case: symbol_uploader enabled but endpoints list is empty - should error
	path := filepath.Join("testdata", "symbol_uploader_empty_endpoints.yaml")
	data, err := os.ReadFile(path)
	require.NoError(t, err)

	retrieved, err := confmap.NewRetrievedFromYAML(data)
	require.NoError(t, err)

	conf, err := retrieved.AsConf()
	require.NoError(t, err)

	converter := &converterWithoutAgent{}
	err = converter.Convert(context.Background(), conf)
	require.Error(t, err)
	require.Contains(t, err.Error(), "symbol_endpoints cannot be empty")
}

// ============================================================================
// Exporter Tests for converterWithoutAgent
// ============================================================================

func TestWithoutAgentCheckOtlpHttpExporterEnsuresHeaders(t *testing.T) {
	result := loadAsStandaloneMode(t, "ensures_headers.yaml")

	// Check that headers was created
	_, ok := Get[confMap](result, "exporters::otlphttp::headers")
	require.True(t, ok)
}

func TestWithoutAgentCheckOtlpHttpExporterWithStringApiKey(t *testing.T) {
	result := loadAsStandaloneMode(t, "otlphttp_with_string_api_key.yaml")

	// Check that dd-api-key is preserved as string
	apiKey, ok := Get[string](result, "exporters::otlphttp::headers::dd-api-key")
	require.True(t, ok)
	require.Equal(t, "test-api-key", apiKey)
}

func TestWithoutAgentCheckOtlpHttpExporterConvertsNonStringApiKey(t *testing.T) {
	result := loadAsStandaloneMode(t, "otlphttp_converts_non_string_api_key.yaml")

	// Check that dd-api-key was converted to string
	apiKey, ok := Get[string](result, "exporters::otlphttp::headers::dd-api-key")
	require.True(t, ok)
	require.Equal(t, "12345", apiKey)
}

func TestWithoutAgentCheckOtlpHttpExporterMultipleExporters(t *testing.T) {
	result := loadAsStandaloneMode(t, "multiple_otlphttp_exporters.yaml")

	// Check prod exporter api key was converted to string
	prodAPIKey, ok := Get[string](result, "exporters::otlphttp/prod::headers::dd-api-key")
	require.True(t, ok)
	require.Equal(t, "11111", prodAPIKey)

	// Check staging exporter api key is preserved as string
	stagingAPIKey, ok := Get[string](result, "exporters::otlphttp/staging::headers::dd-api-key")
	require.True(t, ok)
	require.Equal(t, "staging-key", stagingAPIKey)

	// Check that logging exporter still exists
	_, ok = Get[confMap](result, "exporters::logging")
	require.True(t, ok)
}

func TestWithoutAgentCheckOtlpHttpExporterIgnoresNonOtlpHttp(t *testing.T) {
	result := loadAsStandaloneMode(t, "ignores_non_otlphttp.yaml")

	// Check that non-otlphttp exporters are preserved
	_, ok := Get[confMap](result, "exporters::logging")
	require.True(t, ok)

	_, ok = Get[confMap](result, "exporters::debug")
	require.True(t, ok)
}

func TestWithoutAgentCheckExportersErrorsWhenNoOtlpHttp(t *testing.T) {
	path := filepath.Join("testdata", "errors_when_no_otlphttp.yaml")
	data, err := os.ReadFile(path)
	require.NoError(t, err)

	retrieved, err := confmap.NewRetrievedFromYAML(data)
	require.NoError(t, err)

	conf, err := retrieved.AsConf()
	require.NoError(t, err)

	converter := &converterWithoutAgent{}
	err = converter.Convert(context.Background(), conf)
	require.Error(t, err)
	require.Contains(t, err.Error(), "no otlphttp exporter configured")
}

func TestWithoutAgentHeadersExistButWrongType(t *testing.T) {
	// Tricky: exporter headers exist but are a string, not a map
	// Ensure silently replaces wrong-typed values with correct empty types
	result := loadAsStandaloneMode(t, "headers_exist_but_wrong_type.yaml")

	// The invalid string should have been replaced with a map
	headers, ok := Get[confMap](result, "exporters::otlphttp::headers")
	require.True(t, ok)
	require.NotNil(t, headers)

	// ensureStringKey fills in dd-api-key from config when it doesn't exist
	// So after replacement, the headers map will have the default api key from config
	require.NotEmpty(t, headers) // Now contains dd-api-key from config
	_, hasAPIKey := headers["dd-api-key"]
	require.True(t, hasAPIKey) // Filled from config
}

func TestWithoutAgentRemovesAgentExtensions(t *testing.T) {
	result := loadAsStandaloneMode(t, "wo_agent_removes_extensions.yaml")

	// Check that all agent extensions were removed from definitions (both base and custom names)
	extensions, ok := Get[confMap](result, "extensions")
	require.True(t, ok)
	require.NotContains(t, extensions, "ddprofiling", "ddprofiling extension should be removed")
	require.NotContains(t, extensions, "ddprofiling/custom", "ddprofiling/custom extension should be removed")
	require.NotContains(t, extensions, "hpflare", "hpflare extension should be removed")
	require.NotContains(t, extensions, "hpflare/prod", "hpflare/prod extension should be removed")

	// Check that non-agent extensions are preserved
	require.Contains(t, extensions, "custom", "custom extension should be preserved")
	require.Contains(t, extensions, "other", "other extension should be preserved")

	// Check that all agent extensions were removed from service extensions list
	serviceExtensions, ok := Get[[]any](result, "service::extensions")
	require.True(t, ok)
	require.NotContains(t, serviceExtensions, "ddprofiling", "ddprofiling should be removed from service extensions")
	require.NotContains(t, serviceExtensions, "ddprofiling/custom", "ddprofiling/custom should be removed from service extensions")
	require.NotContains(t, serviceExtensions, "hpflare", "hpflare should be removed from service extensions")
	require.NotContains(t, serviceExtensions, "hpflare/prod", "hpflare/prod should be removed from service extensions")

	// Check that non-agent extensions are preserved in service list
	require.Contains(t, serviceExtensions, "custom", "custom should be preserved in service extensions")
	require.Contains(t, serviceExtensions, "other", "other should be preserved in service extensions")
}

func TestWithoutAgentGlobalProcessorsSectionIsNotMap(t *testing.T) {
	// Tricky: processors section exists but is a string, not a map
	// Ensure silently replaces wrong-typed values with correct empty types
	result := loadAsStandaloneMode(t, "wo_agent_global_procs_not_map.yaml")

	// The invalid string should have been replaced with a valid map
	processors, ok := Get[confMap](result, "processors")
	require.True(t, ok)
	require.NotNil(t, processors)

	// resourcedetection/default should have been added
	_, exists := processors["resourcedetection/default"]
	require.True(t, exists)
}

func TestWithoutAgentEmptyPipeline(t *testing.T) {
	// Edge case: Empty everything in pipeline
	path := filepath.Join("testdata", "without_agent_empty_pipeline.yaml")
	data, err := os.ReadFile(path)
	require.NoError(t, err)

	retrieved, err := confmap.NewRetrievedFromYAML(data)
	require.NoError(t, err)

	conf, err := retrieved.AsConf()
	require.NoError(t, err)

	converter := &converterWithoutAgent{}
	err = converter.Convert(context.Background(), conf)

	// Should error - no otlphttp exporter
	require.Error(t, err)
	require.Contains(t, err.Error(), "no otlphttp exporter configured")
}

func TestWithoutAgentNonStringProcessorNameInPipeline(t *testing.T) {
	// Edge case: Non-string value in processors list (should be handled gracefully)
	path := filepath.Join("testdata", "non_string_processor_name_in_pipeline.yaml")
	data, err := os.ReadFile(path)
	require.NoError(t, err)

	retrieved, err := confmap.NewRetrievedFromYAML(data)
	require.NoError(t, err)

	conf, err := retrieved.AsConf()
	require.NoError(t, err)

	converter := &converterWithoutAgent{}
	err = converter.Convert(context.Background(), conf)

	// Should error on the first non-string processor (123)
	require.Error(t, err)
	require.Contains(t, err.Error(), "processor name must be a string")
}

func TestWithoutAgentConverterErrorPropagationFromEnsure(t *testing.T) {
	// Test that converter properly propagates errors from Ensure
	// This tests the full integration path
	path := filepath.Join("testdata", "error_pipelines_not_map.yaml")
	data, err := os.ReadFile(path)
	require.NoError(t, err)

	retrieved, err := confmap.NewRetrievedFromYAML(data)
	require.NoError(t, err)

	conf, err := retrieved.AsConf()
	require.NoError(t, err)

	converter := &converterWithoutAgent{}
	err = converter.Convert(context.Background(), conf)

	// Should error because service::pipelines is not a map
	require.Error(t, err)
	require.Contains(t, err.Error(), "path element \"pipelines\" is not a map")
}
