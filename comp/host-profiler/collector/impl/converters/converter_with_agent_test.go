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

func TestWithAgentProcessorsAddsDefaultWhenNoInfraattributes(t *testing.T) {
	result := loadAsAgentMode(t, "adds_default_when_no_infraattributes.yaml")

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

func TestWithAgentCheckProcessorsEnsuresInfraattributesConfig(t *testing.T) {
	result := loadAsAgentMode(t, "ensures_infraattributes_config.yaml")

	// Check that allow_hostname_override was set correctly
	allowHostnameOverride, ok := Get[bool](result, "processors::infraattributes::allow_hostname_override")
	require.True(t, ok)
	require.Equal(t, true, allowHostnameOverride)

	// Check that existing config was preserved
	someOtherConfig, ok := Get[string](result, "processors::infraattributes::some_other_config")
	require.True(t, ok)
	require.Equal(t, "value", someOtherConfig)
}

func TestWithAgentCheckProcessorsRemovesResourcedetection(t *testing.T) {
	result := loadAsAgentMode(t, "removes_resourcedetection.yaml")

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

func TestWithAgentCheckProcessorsRemovesResourcedetectionCustomName(t *testing.T) {
	result := loadAsAgentMode(t, "removes_resourcedetection_custom_name.yaml")

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

func TestWithAgentCheckProcessorsHandlesInfraattributesCustomName(t *testing.T) {
	result := loadAsAgentMode(t, "handles_infraattributes_custom_name.yaml")

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

func TestWithAgentCheckReceiversAddsHostprofilerWhenMissing(t *testing.T) {
	result := loadAsAgentMode(t, "adds_hostprofiler_when_missing.yaml")

	// Check that hostprofiler was added with symbol_uploader disabled
	enabled, ok := Get[bool](result, "receivers::hostprofiler::symbol_uploader::enabled")
	require.True(t, ok)
	require.Equal(t, false, enabled)

	// Check that hostprofiler was added to pipeline
	receivers, ok := Get[[]any](result, "service::pipelines::profiles::receivers")
	require.True(t, ok)
	require.Contains(t, receivers, "hostprofiler")
}

func TestWithAgentCheckReceiversPreservesOtlpProtocols(t *testing.T) {
	result := loadAsAgentMode(t, "preserves_otlp_protocols.yaml")

	// Check that existing OTLP protocol config is preserved
	endpoint, ok := Get[string](result, "receivers::otlp::protocols::grpc::endpoint")
	require.True(t, ok)
	require.Equal(t, "0.0.0.0:4317", endpoint)
}

func TestWithAgentCheckReceiversCreatesDefaultHostprofiler(t *testing.T) {
	result := loadAsAgentMode(t, "creates_default_hostprofiler.yaml")

	// Check that hostprofiler was created with symbol_uploader disabled
	enabled, ok := Get[bool](result, "receivers::hostprofiler::symbol_uploader::enabled")
	require.True(t, ok)
	require.Equal(t, false, enabled)
}

func TestWithAgentCheckReceiversSymbolUploaderDisabled(t *testing.T) {
	result := loadAsAgentMode(t, "symbol_uploader_disabled.yaml")

	// Check that symbol_uploader remains disabled
	enabled, ok := Get[bool](result, "receivers::hostprofiler::symbol_uploader::enabled")
	require.True(t, ok)
	require.Equal(t, false, enabled)
}

func TestWithAgentCheckReceiversSymbolUploaderWithStringKeys(t *testing.T) {
	result := loadAsAgentMode(t, "symbol_uploader_with_string_keys.yaml")

	// Get symbol endpoints and check the first endpoint
	endpoints, ok := Get[[]any](result, "receivers::hostprofiler::symbol_uploader::symbol_endpoints")
	require.True(t, ok)
	require.Len(t, endpoints, 1)

	endpoint := endpoints[0].(confMap)
	require.Equal(t, "test-key", endpoint["api_key"])
	require.Equal(t, "test-app-key", endpoint["app_key"])
}

func TestWithAgentCheckReceiversConvertsNonStringApiKey(t *testing.T) {
	result := loadAsAgentMode(t, "converts_non_string_api_key.yaml")

	// Check that api_key was converted to string
	endpoints, ok := Get[[]any](result, "receivers::hostprofiler::symbol_uploader::symbol_endpoints")
	require.True(t, ok)
	endpoint := endpoints[0].(confMap)
	require.Equal(t, "12345", endpoint["api_key"])
}

func TestWithAgentCheckReceiversConvertsNonStringAppKey(t *testing.T) {
	result := loadAsAgentMode(t, "converts_non_string_app_key.yaml")

	// Check that app_key was converted to string
	endpoints, ok := Get[[]any](result, "receivers::hostprofiler::symbol_uploader::symbol_endpoints")
	require.True(t, ok)
	endpoint := endpoints[0].(confMap)
	require.Equal(t, "67890", endpoint["app_key"])
}

func TestWithAgentCheckReceiversAddsHostprofilerToPipeline(t *testing.T) {
	result := loadAsAgentMode(t, "adds_hostprofiler_to_pipeline.yaml")

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

func TestWithAgentCheckReceiversMultipleSymbolEndpoints(t *testing.T) {
	result := loadAsAgentMode(t, "multiple_symbol_endpoints.yaml")

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

func TestWithAgentCheckReceiversNonStringReceiverName(t *testing.T) {
	// Test that non-string receiver names in pipeline are rejected
	path := filepath.Join("testdata", "non_string_receiver_name_in_pipeline.yaml")
	data, err := os.ReadFile(path)
	require.NoError(t, err)

	retrieved, err := confmap.NewRetrievedFromYAML(data)
	require.NoError(t, err)

	conf, err := retrieved.AsConf()
	require.NoError(t, err)

	converter := &converterWithAgent{}
	err = converter.Convert(context.Background(), conf)

	require.Error(t, err)
	require.Contains(t, err.Error(), "receiver name must be a string")
}

func TestWithAgentCheckReceiversMultipleHostprofilers(t *testing.T) {
	// Test that multiple hostprofiler receivers in pipeline are all processed
	result := loadAsAgentMode(t, "multiple_hostprofiler_receivers.yaml")

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

func TestWithAgentCheckReceiversSymbolEndpointsWrongType(t *testing.T) {
	// Test that symbol_endpoints with wrong type (string not list) returns error
	path := filepath.Join("testdata", "symbol_endpoints_wrong_type.yaml")
	data, err := os.ReadFile(path)
	require.NoError(t, err)

	retrieved, err := confmap.NewRetrievedFromYAML(data)
	require.NoError(t, err)

	conf, err := retrieved.AsConf()
	require.NoError(t, err)

	converter := &converterWithAgent{}
	err = converter.Convert(context.Background(), conf)

	require.Error(t, err)
	require.Contains(t, err.Error(), "symbol_endpoints must be a list")
}

func TestWithAgentCheckOtlpHttpExporterEnsuresHeaders(t *testing.T) {
	result := loadAsAgentMode(t, "ensures_headers.yaml")

	// Check that headers was created
	_, ok := Get[confMap](result, "exporters::otlphttp::headers")
	require.True(t, ok)
}

func TestWithAgentCheckOtlpHttpExporterWithStringApiKey(t *testing.T) {
	result := loadAsAgentMode(t, "otlphttp_with_string_api_key.yaml")

	// Check that dd-api-key is preserved as string
	apiKey, ok := Get[string](result, "exporters::otlphttp::headers::dd-api-key")
	require.True(t, ok)
	require.Equal(t, "test-api-key", apiKey)
}

func TestWithAgentCheckOtlpHttpExporterConvertsNonStringApiKey(t *testing.T) {
	result := loadAsAgentMode(t, "otlphttp_converts_non_string_api_key.yaml")

	// Check that dd-api-key was converted to string
	apiKey, ok := Get[string](result, "exporters::otlphttp::headers::dd-api-key")
	require.True(t, ok)
	require.Equal(t, "12345", apiKey)
}

func TestWithAgentCheckOtlpHttpExporterMultipleExporters(t *testing.T) {
	result := loadAsAgentMode(t, "multiple_otlphttp_exporters.yaml")

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

func TestWithAgentCheckOtlpHttpExporterIgnoresNonOtlpHttp(t *testing.T) {
	result := loadAsAgentMode(t, "ignores_non_otlphttp.yaml")

	// Check that non-otlphttp exporters are preserved
	_, ok := Get[confMap](result, "exporters::logging")
	require.True(t, ok)

	_, ok = Get[confMap](result, "exporters::debug")
	require.True(t, ok)
}

func TestWithAgentCheckExportersErrorsWhenNoOtlpHttp(t *testing.T) {
	path := filepath.Join("testdata", "errors_when_no_otlphttp.yaml")
	data, err := os.ReadFile(path)
	require.NoError(t, err)

	retrieved, err := confmap.NewRetrievedFromYAML(data)
	require.NoError(t, err)

	conf, err := retrieved.AsConf()
	require.NoError(t, err)

	converter := &converterWithAgent{}
	err = converter.Convert(context.Background(), conf)
	require.Error(t, err)
	require.Contains(t, err.Error(), "no otlphttp exporter configured")
}

func TestWithAgentProcessorsOverridesAllowHostnameOverrideToTrue(t *testing.T) {
	// Test that even if allow_hostname_override is explicitly set to false, we override it to true
	result := loadAsAgentMode(t, "overrides_hostname_override_true.yaml")

	// Should be overridden to true
	allowHostnameOverride, ok := Get[bool](result, "processors::infraattributes::allow_hostname_override")
	require.True(t, ok)
	require.Equal(t, true, allowHostnameOverride)

	// Other config should be preserved
	someConfig, ok := Get[string](result, "processors::infraattributes::some_config")
	require.True(t, ok)
	require.Equal(t, "value", someConfig)
}

func TestWithAgentProcessorsWithBothDefaultAndCustomInfraattributes(t *testing.T) {
	// Edge case: both infraattributes and infraattributes/custom in pipeline
	result := loadAsAgentMode(t, "default_and_custom_infraattrs.yaml")

	// Both should have allow_hostname_override set to true
	allowHostnameOverride1, ok := Get[bool](result, "processors::infraattributes::allow_hostname_override")
	require.True(t, ok)
	require.Equal(t, true, allowHostnameOverride1)

	allowHostnameOverride2, ok := Get[bool](result, "processors::infraattributes/custom::allow_hostname_override")
	require.True(t, ok)
	require.Equal(t, true, allowHostnameOverride2)
}

func TestWithAgentProcessorsWithMultipleResourcedetectionProcessors(t *testing.T) {
	// Multiple resourcedetection processors with different names - all should be removed
	result := loadAsAgentMode(t, "multiple_resourcedetection_processors.yaml")

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

func TestWithAgentReceiversSymbolUploaderEnabledWithEmptyEndpoints(t *testing.T) {
	// Edge case: symbol_uploader enabled but endpoints list is empty - should error
	path := filepath.Join("testdata", "symbol_uploader_empty_endpoints.yaml")
	data, err := os.ReadFile(path)
	require.NoError(t, err)

	retrieved, err := confmap.NewRetrievedFromYAML(data)
	require.NoError(t, err)

	conf, err := retrieved.AsConf()
	require.NoError(t, err)

	converter := &converterWithAgent{}
	err = converter.Convert(context.Background(), conf)
	require.Error(t, err)
	require.Contains(t, err.Error(), "symbol_endpoints cannot be empty")
}

func TestWithAgentEmptyPipeline(t *testing.T) {
	// Edge case: Empty everything in pipeline
	path := filepath.Join("testdata", "empty_pipeline.yaml")
	data, err := os.ReadFile(path)
	require.NoError(t, err)

	retrieved, err := confmap.NewRetrievedFromYAML(data)
	require.NoError(t, err)

	conf, err := retrieved.AsConf()
	require.NoError(t, err)

	converter := &converterWithAgent{}
	err = converter.Convert(context.Background(), conf)

	// Should error - no otlphttp exporter
	require.Error(t, err)
	require.Contains(t, err.Error(), "no otlphttp exporter configured")
}

func TestWithAgentNonStringProcessorNameInPipeline(t *testing.T) {
	// Edge case: Non-string value in processors list (should be handled gracefully)
	path := filepath.Join("testdata", "non_string_processor_name_in_pipeline.yaml")
	data, err := os.ReadFile(path)
	require.NoError(t, err)

	retrieved, err := confmap.NewRetrievedFromYAML(data)
	require.NoError(t, err)

	conf, err := retrieved.AsConf()
	require.NoError(t, err)

	converter := &converterWithAgent{}
	err = converter.Convert(context.Background(), conf)

	// Should error on the first non-string processor (123)
	require.Error(t, err)
	require.Contains(t, err.Error(), "processor name must be a string")
}

func TestWithAgentHeadersExistButWrongType(t *testing.T) {
	// Tricky: exporter headers exist but are a string, not a map
	// Ensure silently replaces wrong-typed values with correct empty types
	result := loadAsAgentMode(t, "headers_exist_but_wrong_type.yaml")

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

func TestWithAgentEmptyStringProcessorName(t *testing.T) {
	// Tricky: processor name is an empty string
	result := loadAsAgentMode(t, "empty_string_processor_name.yaml")

	// Empty string should be preserved, infraattributes should be added
	processorNames, ok := Get[[]any](result, "service::pipelines::profiles::processors")
	require.True(t, ok)
	require.Contains(t, processorNames, "")
	require.Contains(t, processorNames, "infraattributes/default")
}

func TestWithAgentProcessorNameSimilarButNotExactMatch(t *testing.T) {
	// Tests that similar names don't match - uses proper OTEL type/id parsing
	// In OTEL specs, components must use type/id format (e.g., infraattributes/custom)
	result := loadAsAgentMode(t, "processor_name_similar_not_exact.yaml")

	processorNames, ok := Get[[]any](result, "service::pipelines::profiles::processors")
	require.True(t, ok)

	// Correct behavior: myresourcedetection stays (not "resourcedetection" or "resourcedetection/*")
	require.Contains(t, processorNames, "myresourcedetection")

	// Correct behavior: infraattributes_custom stays unchanged (not "infraattributes" or "infraattributes/*")
	require.Contains(t, processorNames, "infraattributes_custom")

	// Verify it was NOT treated as infraattributes (allow_hostname_override NOT added)
	_, ok = Get[bool](result, "processors::infraattributes_custom::allow_hostname_override")
	require.False(t, ok)

	// batch should remain
	require.Contains(t, processorNames, "batch")

	// Since no valid infraattributes found, default SHOULD be added
	require.Contains(t, processorNames, "infraattributes/default")

	// Verify infraattributes/default was configured correctly
	allowHostnameOverride, ok := Get[bool](result, "processors::infraattributes/default::allow_hostname_override")
	require.True(t, ok)
	require.Equal(t, true, allowHostnameOverride)
}

func TestWithAgentGlobalProcessorsSectionIsNotMap(t *testing.T) {
	// Tricky: processors section exists but is a string, not a map
	// Ensure silently replaces wrong-typed values with correct empty types
	result := loadAsAgentMode(t, "global_processors_section_is_not_map.yaml")

	// The invalid string should have been replaced with a valid map
	processors, ok := Get[confMap](result, "processors")
	require.True(t, ok)
	require.NotNil(t, processors)

	// infraattributes/default should have been added
	_, exists := processors["infraattributes/default"]
	require.True(t, exists)
}

func TestWithAgentAddsResourceProcessorWithMetadata(t *testing.T) {
	// No resource processor in pipeline - should add resource/default with profiler tags
	result := loadAsAgentMode(t, "adds_resource_processor.yaml")

	// resource/default should be added to pipeline
	processors, ok := Get[[]any](result, "service::pipelines::profiles::processors")
	require.True(t, ok)
	require.Contains(t, processors, "resource/default")

	// Check profiler_name tag was added
	attrs, ok := Get[[]any](result, "processors::resource/default::attributes")
	require.True(t, ok)
	require.NotEmpty(t, attrs)

	// Find profiler_name and profiler_version in attributes
	hasProfilerName := false
	hasProfilerVersion := false
	for _, attrAny := range attrs {
		attr := attrAny.(confMap)
		key, ok := attr["key"].(string)
		require.True(t, ok)
		if key == "profiler_name" {
			hasProfilerName = true
			require.Equal(t, "host_profiler", attr["value"])
			require.Equal(t, "upsert", attr["action"])
		}
		if key == "profiler_version" {
			hasProfilerVersion = true
			require.Equal(t, "upsert", attr["action"])
		}
	}

	require.True(t, hasProfilerName, "profiler_name tag should be present")
	require.True(t, hasProfilerVersion, "profiler_version tag should be present")
}
