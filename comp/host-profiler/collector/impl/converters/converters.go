// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

// Package converters implements OTEL collector configuration converters for the host profiler.
//
// Converters normalize user-provided OTEL collector configs by adding required Datadog-specific
// components while preserving explicit user configuration values wherever possible.
package converters

import (
	"fmt"
	"log/slog"

	"github.com/DataDog/datadog-agent/comp/host-profiler/version"
	configutils "github.com/DataDog/datadog-agent/pkg/config/utils"
	"github.com/DataDog/datadog-agent/pkg/util/confmaputils"
	"go.opentelemetry.io/collector/confmap"
	"go.opentelemetry.io/collector/confmap/xconfmap"
)

// NewFactoryWithoutAgent returns a new converterWithoutAgent factory.
func NewFactoryWithoutAgent() confmap.ConverterFactory {
	return confmap.NewConverterFactory(newConverterWithoutAgent)
}

type confMap = map[string]any

// Component type names for OTEL configuration
const (
	componentTypeInfraAttributes     = "infraattributes"
	componentTypeResourceDetection   = "resourcedetection"
	componentTypeDDHostNameProcessor = "ddhostname"
	componentTypeProfiling           = "profiling"
	componentTypeDDProfiling         = "ddprofiling"
	componentTypeHPFlare             = "hpflare"
)

// Component type names for otlp_http
const (
	componentTypeOtlpHTTP           = "otlp_http"
	componentTypeOtlpHTTPDeprecated = "otlphttp"
)

// Default component names
const (
	defaultInfraAttributesName     = componentTypeInfraAttributes + "/" + confmaputils.AutoConfiguredSuffix
	defaultResourceDetectionName   = componentTypeResourceDetection + "/" + confmaputils.AutoConfiguredSuffix
	defaultDDHostNameProcessorName = componentTypeDDHostNameProcessor + "/" + confmaputils.AutoConfiguredSuffix
	defaultProfilingName           = "profiling"
)

// Reserved component names for internal metrics pipeline
const (
	reservedPrometheusReceiver         = "prometheus/dd-hp-internal"
	reservedFilterProcessor            = "filter/dd-hp-drop-internal"
	reservedCumulativeToDeltaProcessor = "cumulativetodelta/dd-hp-internal"
	reservedContainerIDProcessor       = "resource/" + confmaputils.AutoConfiguredSuffix + "-container-attribute"
	internalHealthMetricsPipelineName  = "metrics/profiler-internal-health"
)

// Configuration paths used multiple times across converters
const (
	pathSymbolUploaderEnabled = "symbol_uploader::enabled"
	pathSymbolEndpoints       = "symbol_uploader::symbol_endpoints"
)

// Configuration field names used multiple times
const (
	fieldDDAPIKey                = "dd-api-key"
	fieldDDEVPOrigin             = "dd-evp-origin"
	fieldDDEVPOriginVersion      = "dd-evp-origin-version"
	fieldDDOtelMetricConfig      = "dd-otel-metric-config"
	fieldDDOtelMetricConfigValue = `{"resource_attributes_as_tags": true}`
	fieldAPIKey                  = "api_key"
	fieldAppKey                  = "app_key"
)

// OTEL config path prefixes
const (
	pathPrefixReceivers  = "receivers::"
	pathPrefixExporters  = "exporters::"
	pathPrefixProcessors = "processors::"
)

// isComponentTypeOtlpHTTP checks for both the current ("otlp_http") and deprecated ("otlphttp") component names.
func isComponentTypeOtlpHTTP(name string) bool {
	return confmaputils.IsComponentType(name, componentTypeOtlpHTTP) || confmaputils.IsComponentType(name, componentTypeOtlpHTTPDeprecated)
}

// ensureKeyStringValue checks if a key exists in the config and converts it to a string if needed.
// Only converts primitive numeric types; rejects complex types like maps, slices, or structs.
// Returns true if the key exists and is (or was converted to) a string, false otherwise.
func ensureKeyStringValue(config confMap, key string) bool {
	val, ok := config[key]
	if !ok {
		return false
	}

	if _, isString := val.(string); isString {
		return true
	}

	// Only convert primitive numeric types
	switch v := val.(type) {
	case int, int32, int64, float32, float64, uint, uint32, uint64:
		config[key] = fmt.Sprintf("%v", v)
		return true
	case xconfmap.ExpandedValue:
		// ExpandedValues should not be altered at conversion stage
		return true
	default:
		slog.Warn("API key has unexpected type, cannot convert", slog.String("key", key), slog.String("type", fmt.Sprintf("%T", v)))
		return false
	}
}

// addProfilerMetadataTags always creates a dedicated resource/profiler-metadata processor
// without searching for existing resource processors.
// This function emits OTel semantic convention tags and must only be called from the standalone (no-agent) path.
func addProfilerMetadataTags(conf confMap, profilesProcessors []any) ([]any, error) {
	const resourceProcessorName = "resource/dd-profiler-internal-metadata"

	// Check if the processor is already defined in root processors
	globalProcessors, _ := confmaputils.Get[confMap](conf, "processors")
	if _, exists := globalProcessors[resourceProcessorName]; exists {
		return nil, fmt.Errorf("%s is a reserved resource processor name. Please change it in your configuration file", resourceProcessorName)
	}

	for _, proc := range profilesProcessors {
		if procName := proc.(string); procName == resourceProcessorName {
			return nil, fmt.Errorf("%s is a reserved resource processor name. Please remove it from the profiles pipeline", resourceProcessorName)
		}
	}

	resourceProcessor, err := confmaputils.Ensure[confMap](conf, "processors::"+resourceProcessorName)
	if err != nil {
		return nil, err
	}

	attributes, err := confmaputils.Ensure[[]any](resourceProcessor, "attributes")
	if err != nil {
		return nil, err
	}

	profilerNameElement := confMap{
		"key":    version.OTelProfilerNameKey,
		"value":  version.StandaloneProfilerName,
		"action": "upsert",
	}
	profilerVersionElement := confMap{
		"key":    version.OTelProfilerVersionKey,
		"value":  version.ProfilerVersion,
		"action": "upsert",
	}

	attributes = append(attributes, profilerNameElement)
	attributes = append(attributes, profilerVersionElement)
	if err := confmaputils.Set(resourceProcessor, "attributes", attributes); err != nil {
		return nil, err
	}

	return append(profilesProcessors, resourceProcessorName), nil
}

// inferMetricsEndpoint derives OTLP metrics endpoint from profiles endpoint.
// Transforms profile intake URLs to OTLP metrics endpoints by extracting the site.
//
// Examples:
//   - "https://intake.profile.us3.datadoghq.com/v1development/profiles" -> "https://otlp.us3.datadoghq.com/v1/metrics"
//   - "https://intake.profile.datadoghq.com/v1development/profiles" -> "https://otlp.datadoghq.com/v1/metrics"
//
// Returns an error if the URL cannot be parsed or if the site cannot be extracted.
func inferMetricsEndpoint(profilesEndpoint string) (string, error) {
	site := configutils.ExtractSiteFromURL(profilesEndpoint)
	if site == "" {
		return "", fmt.Errorf("cannot extract site from URL: %s", profilesEndpoint)
	}

	return fmt.Sprintf("https://otlp.%s/v1/metrics", site), nil
}
