// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

//go:build linux

// Package converters implements the converters for the host profiler collector.
package converters

import (
	"context"
	"fmt"
	"log/slog"
	"slices"
	"strings"

	"github.com/DataDog/datadog-agent/comp/core/config"
	configutils "github.com/DataDog/datadog-agent/pkg/config/utils"
	"go.opentelemetry.io/collector/confmap"
)

type endpoint struct {
	site    string
	url     string
	apiKeys []string
}

type configManager struct {
	endpoints []endpoint
	config    config.Component
}

func newConfigManager(config config.Component) configManager {
	profilingDDURL := config.GetString("apm_config.profiling_dd_url")
	ddSite := config.GetString("site")
	apiKey := config.GetString(fieldAPIKey)

	var usedURL, usedSite string
	if profilingDDURL != "" {
		usedSite = configutils.ExtractSiteFromURL(profilingDDURL)
		if usedSite == "" {
			slog.Warn("could not extract site from apm_config.profiling_dd_url, skipping endpoint", slog.String("url", profilingDDURL))
		}
		usedURL = profilingDDURL
	} else if ddSite != "" {
		usedSite = ddSite
	}

	profilingAdditionalEndpoints := config.GetStringMapStringSlice("apm_config.profiling_additional_endpoints")
	var endpoints []endpoint
	for endpointURL, keys := range profilingAdditionalEndpoints {
		site := configutils.ExtractSiteFromURL(endpointURL)
		if site == "" {
			slog.Warn("could not extract site from URL, skipping endpoint", slog.String("url", endpointURL))
			continue
		}
		endpoints = append(endpoints, endpoint{
			site:    site,
			url:     endpointURL,
			apiKeys: keys,
		})
	}
	slog.Info("main site inferred from core configuration", slog.String("site", usedSite))

	// Add main endpoint if we have a valid site
	if usedSite == "" {
		slog.Warn("could not determine site from core configuration, no default endpoint will be configured")
	} else {
		endpoints = append(endpoints, endpoint{site: usedSite, url: usedURL, apiKeys: []string{apiKey}})
	}

	if len(endpoints) == 0 {
		slog.Warn("no valid endpoints in core agent configured for inference")
	}

	return configManager{config: config, endpoints: endpoints}
}

// converterWithAgent ensures sane configuration that satisfies the following conditions:
//   - At least one infraattributes processor declared and used with `allow_hostname_override: true`
//   - If no infraattributes processor used, declare & use a minimal infraattributes processor
//   - No resourcedetection configured nor declared
//   - At least one otlphttpexporter with dd-api-key declared & used
//   - Check if used otlphttpexporter has dd-api-key as string, if not string convert it, if not at all notify user
//   - If hostprofiler::symbol_uploader::enabled == true, convert api_key/app_key to strings in each endpoint
//   - If no hostprofiler is used & configured, add minimal one with symbol_uploader: false
type converterWithAgent struct {
	configManager configManager
}

func newConverterWithAgent(_ confmap.ConverterSettings, config config.Component) confmap.Converter {
	return &converterWithAgent{configManager: newConfigManager(config)}
}

// Convert implements the confmap.Converter interface for converterWithAgent.
func (c *converterWithAgent) Convert(_ context.Context, conf *confmap.Conf) error {
	confStringMap := conf.ToStringMap()

	profilesPipeline, err := Ensure[confMap](confStringMap, "service::pipelines::profiles")
	if err != nil {
		return err
	}

	processorNames, err := Ensure[[]any](profilesPipeline, "processors")
	if err != nil {
		return err
	}

	receiverNames, err := Ensure[[]any](profilesPipeline, "receivers")
	if err != nil {
		return err
	}

	exporterNames, err := Ensure[[]any](profilesPipeline, "exporters")
	if err != nil {
		return err
	}

	// See if there is any otlpHTTP exporter configured, if not, infer as many exporters as possible
	if err := c.ensureOtlpHTTPExporterConfig(confStringMap, exporterNames); err != nil {
		return err
	}

	// Determines what components we need to check and ensures at least one infraattributes is configured
	// Deletes any resourcedetection configured in the profiles pipeline
	newProcessorNames, err := c.fixProcessorsPipeline(confStringMap, processorNames)
	if err != nil {
		return err
	}
	newProcessorNames, err = addProfilerMetadataTags(confStringMap, newProcessorNames)
	if err != nil {
		return err
	}

	profilesPipeline["processors"] = newProcessorNames

	// Ensures at least one hostprofiler is used & configured
	// If not, create a minimal component with symbol uploading disabled
	newReceiverNames, err := c.fixReceiversPipeline(confStringMap, receiverNames)
	if err != nil {
		return err
	}
	profilesPipeline["receivers"] = newReceiverNames

	// Go through every configured processors to make sure there are no resourcedetections declared that were not in the
	// pipeline
	if err := c.ensureGlobalProcessors(confStringMap); err != nil {
		return err
	}

	// Add internal health metrics pipeline
	// Get updated exporter list from profiles pipeline (may have been modified by ensureOtlpHTTPExporterConfig)
	updatedExporterNames, err := Ensure[[]any](profilesPipeline, "exporters")
	if err != nil {
		return err
	}
	if err := c.addMetricsPipeline(confStringMap, updatedExporterNames); err != nil {
		return err
	}

	*conf = *confmap.NewFromStringMap(confStringMap)
	return nil
}

// fixReceiversPipeline ensures at least one hostprofiler receiver is configured in the pipeline
// If none exists, it adds a minimal hostprofiler receiver with symbol_uploader disabled
func (c *converterWithAgent) fixReceiversPipeline(conf confMap, receiverNames []any) ([]any, error) {
	// Check if hostprofiler is in the pipeline
	hasHostProfiler := false
	for _, nameAny := range receiverNames {
		name, ok := nameAny.(string)
		if !ok {
			return nil, fmt.Errorf("receiver name must be a string, got %T", nameAny)
		}

		if !isComponentType(name, componentTypeHostProfiler) {
			continue
		}

		hasHostProfiler = true

		if hostProfilerConfig, ok := Get[confMap](conf, pathPrefixReceivers+name); ok {
			if err := c.checkHostProfilerReceiverConfig(hostProfilerConfig); err != nil {
				return nil, err
			}
		}
	}

	if hasHostProfiler {
		return receiverNames, nil
	}

	if err := Set(conf, pathPrefixReceivers+defaultHostProfilerName+"::symbol_uploader::enabled", true); err != nil {
		return nil, err
	}

	defaultHostProfiler, _ := Get[confMap](conf, pathPrefixReceivers+defaultHostProfilerName)
	if err := c.inferHostProfilerEndpointConfig(defaultHostProfiler); err != nil {
		return nil, err
	}

	return append(receiverNames, defaultHostProfilerName), nil
}

// checkHostProfilerReceiverConfig validates and normalizes hostprofiler receiver configuration
// It ensures that if symbol_uploader is enabled, symbol_endpoints is properly configured
// and all api_key/app_key values are strings
func (c *converterWithAgent) checkHostProfilerReceiverConfig(hostProfiler confMap) error {
	if isEnabled, ok := Get[bool](hostProfiler, pathSymbolUploaderEnabled); !ok || !isEnabled {
		return nil
	}

	endpoints, ok := Get[[]any](hostProfiler, pathSymbolEndpoints)

	// If symbol_endpoints is missing, wrong type, or empty, infer from agent config
	if !ok || len(endpoints) == 0 {
		slog.Info("symbol uploader enabled but endpoints not configured, inferring from agent config")
		if err := c.inferHostProfilerEndpointConfig(hostProfiler); err != nil {
			return err
		}
		return nil
	}

	// We have valid endpoints, just ensure keys are strings
	for _, epAny := range endpoints {
		if ep, ok := epAny.(confMap); ok {
			ensureKeyStringValue(ep, fieldAPIKey)
			ensureKeyStringValue(ep, fieldAppKey)
		}
	}
	return nil
}

func (c *converterWithAgent) ensureOtlpHTTPExporterConfig(conf confMap, exporterNames []any) error {
	hasOtlpHTTP := false
	for _, nameAny := range exporterNames {
		if name, ok := nameAny.(string); ok && isComponentType(name, componentTypeOtlpHTTP) {
			hasOtlpHTTP = true

			headers, ok := Get[confMap](conf, pathPrefixExporters+name+"::headers")
			if !ok {
				return fmt.Errorf("exporter %s is not configured", name)
			}

			if !ensureKeyStringValue(headers, fieldDDAPIKey) {
				// should we try to infer those keys as well? we might have a key for the given site
				return fmt.Errorf("%s exporter missing required dd-api-key header", name)
			}
		}
	}

	if !hasOtlpHTTP {
		slog.Info("no otlphttp exporter configured, inferring from agent config")
		if err := c.inferOtlpHTTPConfig(conf); err != nil {
			return err
		}
	}

	return nil
}

func (c *converterWithAgent) inferHostProfilerEndpointConfig(hostProfiler confMap) error {
	var symbolEndpoints []any
	for _, endpoint := range c.configManager.endpoints {
		for _, key := range endpoint.apiKeys {
			symbolEndpoints = append(symbolEndpoints, confMap{
				"site":      endpoint.site,
				fieldAPIKey: key,
			})
		}
	}

	if err := Set(hostProfiler, "symbol_uploader::symbol_endpoints", symbolEndpoints); err != nil {
		return err
	}
	return nil
}

func (c *converterWithAgent) inferOtlpHTTPConfig(conf confMap) error {
	const (
		profilesEndpointFormat = "https://intake.profile.%s/v1development/profiles"
		metricsEndpointFormat  = "https://otlp.%s/v1/metrics"
		otlpHTTPNameFormat     = "otlphttp/%s_%d"
	)

	createOtlpHTTPFromEndpoint := func(site, key string) confMap {
		return confMap{
			"profiles_endpoint": fmt.Sprintf(profilesEndpointFormat, site),
			"metrics_endpoint":  fmt.Sprintf(metricsEndpointFormat, site),
			"headers": confMap{
				fieldDDAPIKey: key,
			},
		}
	}

	const profilesExportersPath = "service::pipelines::profiles::exporters"
	profilesExporters, _ := Get[[]any](conf, profilesExportersPath)
	siteCounter := make(map[string]int)
	for _, endpoint := range c.configManager.endpoints {
		for _, key := range endpoint.apiKeys {
			exporterName := fmt.Sprintf(otlpHTTPNameFormat, endpoint.site, siteCounter[endpoint.site])
			siteCounter[endpoint.site]++
			if err := Set(conf, pathPrefixExporters+exporterName, createOtlpHTTPFromEndpoint(endpoint.site, key)); err != nil {
				return err
			}
			profilesExporters = append(profilesExporters, exporterName)
		}
	}

	if err := Set(conf, profilesExportersPath, profilesExporters); err != nil {
		return err
	}
	return nil
}

func (c *converterWithAgent) ensureGlobalProcessors(conf confMap) error {
	processors, err := Ensure[confMap](conf, "processors")
	if err != nil {
		return err
	}

	for name := range processors {
		if isComponentType(name, componentTypeResourceDetection) {
			delete(processors, name)
		}
	}
	return nil
}

func (c *converterWithAgent) fixProcessorsPipeline(conf confMap, processorNames []any) ([]any, error) {
	processors, err := Ensure[confMap](conf, "processors")
	if err != nil {
		return nil, err
	}
	foundInfraattributes := false
	toDelete := make(map[string]bool)

	// remove resourcedetection, track & sanitize infraattributes
	for _, nameAny := range processorNames {
		name, ok := nameAny.(string)
		if !ok {
			return nil, fmt.Errorf("processor name must be a string, got %T", nameAny)
		}

		// Remove resourcedetection from pipeline and global config
		if isComponentType(name, componentTypeResourceDetection) {
			delete(processors, name)
			toDelete[name] = true
			continue
		}

		// Track if we have infraattributes
		if isComponentType(name, componentTypeInfraAttributes) {
			ddDefaultValue, err := SetDefault(processors, name+"::"+fieldAllowHostnameOverride, true)
			if err != nil {
				return nil, err
			}
			if !ddDefaultValue {
				slog.Warn("allow_hostname_override is required but is disabled by user configuration; preserving user value.")
			}
			foundInfraattributes = true
		}
	}

	// Add infraattributes/default if none found
	if !foundInfraattributes {
		if err := Set(processors, defaultInfraAttributesName+"::"+fieldAllowHostnameOverride, true); err != nil {
			return nil, err
		}
		slog.Warn("added minimal infraattributes processor to user configuration")
		processorNames = append(processorNames, defaultInfraAttributesName)
	}

	// Remove processors marked for deletion
	processorNames = slices.DeleteFunc(processorNames, func(processor any) bool {
		name := processor.(string)
		_, exists := toDelete[name]
		return exists
	})

	return processorNames, nil
}

// addMetricsPipeline adds a metrics pipeline for internal health metrics collection.
// It creates a prometheus receiver to scrape OTel collector's internal telemetry,
// infers metrics endpoints from profiles endpoints, and creates a pipeline that
// sends metrics to the same orgs that receive profiles.
func (c *converterWithAgent) addMetricsPipeline(conf confMap, profilesExporterNames []any) error {
	// Check if user already configured the internal health metrics pipeline
	if existing, ok := Get[confMap](conf, "service::pipelines::"+internalHealthMetricsPipelineName); ok {
		slog.Warn("metrics/profiler-internal-health pipeline already configured, skipping auto-configuration",
			slog.Any("existing_config", existing))
		return nil
	}

	// Check if telemetry metrics are disabled
	if level, ok := Get[string](conf, "service::telemetry::metrics::level"); ok {
		if strings.ToLower(level) == "none" {
			slog.Info("metrics telemetry disabled (level=none), skipping metrics pipeline")
			return nil
		}
	}

	// Check for reserved component name conflicts
	if receivers, ok := Get[confMap](conf, "receivers"); ok {
		if _, exists := receivers[reservedPrometheusReceiver]; exists {
			slog.Warn("receiver name conflicts with reserved name, skipping metrics pipeline",
				slog.String("receiver", reservedPrometheusReceiver))
			return nil
		}
	}
	if processors, ok := Get[confMap](conf, "processors"); ok {
		if _, exists := processors[reservedFilterProcessor]; exists {
			slog.Warn("processor name conflicts with reserved name, skipping metrics pipeline",
				slog.String("processor", reservedFilterProcessor))
			return nil
		}
		_,_ = Ensure[confMap](processors, "cumulativetodelta")
	}

	// Add prometheus receiver
	if err := Set(conf, pathPrefixReceivers+reservedPrometheusReceiver, prometheusReceiverConfig()); err != nil {
		return fmt.Errorf("failed to add prometheus receiver: %w", err)
	}

	// Add filter processor
	if err := Set(conf, pathPrefixProcessors+reservedFilterProcessor, filterProcessorConfig()); err != nil {
		return fmt.Errorf("failed to add filter processor: %w", err)
	}


	// Collect metrics exporters (same as profiles exporters)
	metricsExporterNames := []any{}
	for _, exporterNameAny := range profilesExporterNames {
		exporterName, ok := exporterNameAny.(string)
		if !ok {
			continue
		}

		// Only process OTLP HTTP exporters
		if !isComponentType(exporterName, componentTypeOtlpHTTP) {
			continue
		}

		// Get exporter config
		exporterConf, ok := Get[confMap](conf, pathPrefixExporters+exporterName)
		if !ok {
			slog.Warn("exporter not found in config", slog.String("exporter", exporterName))
			continue
		}

		// Exporters created by inferOtlpHTTPConfig() already have metrics_endpoint set.
		// User-provided exporters may only have profiles_endpoint, so we infer for those.
		if _, hasMetrics := Get[string](exporterConf, "metrics_endpoint"); hasMetrics {
			slog.Debug("metrics_endpoint already set, using existing value", slog.String("exporter", exporterName))
			metricsExporterNames = append(metricsExporterNames, exporterName)
			continue
		}

		// Infer metrics_endpoint from profiles_endpoint
		profilesEndpoint, ok := Get[string](exporterConf, "profiles_endpoint")
		if !ok {
			slog.Warn("otlphttp exporter missing profiles_endpoint, cannot infer metrics endpoint",
				slog.String("exporter", exporterName))
			continue
		}

		metricsEndpoint, err := inferMetricsEndpoint(profilesEndpoint)
		if err != nil {
			slog.Warn("cannot infer metrics endpoint from profiles endpoint",
				slog.String("exporter", exporterName),
				slog.String("profiles_endpoint", profilesEndpoint),
				slog.Any("error", err))
			continue
		}

		// Set inferred metrics_endpoint
		if err := Set(exporterConf, "metrics_endpoint", metricsEndpoint); err != nil {
			return fmt.Errorf("failed to set metrics_endpoint for %s: %w", exporterName, err)
		}

		slog.Info("inferred metrics endpoint for exporter",
			slog.String("exporter", exporterName),
			slog.String("profiles_endpoint", profilesEndpoint),
			slog.String("metrics_endpoint", metricsEndpoint))

		metricsExporterNames = append(metricsExporterNames, exporterName)
	}

	if len(metricsExporterNames) == 0 {
		slog.Info("no exporters configured for metrics, skipping metrics pipeline")
		return nil
	}

	metricsExporterNames = append(metricsExporterNames, "debug")

	// Build metrics pipeline
	metricsPipeline := confMap{
		"receivers": []any{reservedPrometheusReceiver},
		"processors": []any{
			reservedFilterProcessor,
			defaultInfraAttributesName, // Reuse from profiles pipeline
			"resource/dd-profiler-internal-metadata", // Reuse from profiles pipeline
			"cumulativetodelta",
		},
		"exporters": metricsExporterNames,
	}

	if err := Set(conf, "service::pipelines::"+internalHealthMetricsPipelineName, metricsPipeline); err != nil {
		return fmt.Errorf("failed to create metrics pipeline: %w", err)
	}

	slog.Info("created internal health metrics pipeline",
		slog.Int("exporters", len(metricsExporterNames)),
		slog.String("pipeline", internalHealthMetricsPipelineName))

	return nil
}
