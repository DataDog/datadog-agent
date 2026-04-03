// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

//go:build linux

// Package converters implements the converters for the host profiler collector.
package converters

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"slices"
	"strings"

	"github.com/DataDog/datadog-agent/comp/host-profiler/version"
	"go.opentelemetry.io/collector/confmap"
	"go.opentelemetry.io/collector/confmap/xconfmap"
	"go.uber.org/zap/exp/zapslog"
)

var resourceDetectionDefaultConfig = confMap{
	"detectors": []any{"system"},
	"system": confMap{
		"resource_attributes": confMap{
			"host.arch": confMap{
				"enabled": true,
			},
			"host.name": confMap{
				"enabled": false,
			},
			"os.type": confMap{
				"enabled": false,
			},
		},
	},
}

// converterWithoutAgent ensures sane configuration that satisfies the following conditions:
//   - At least one resourcedetection processor declared and used with required defaults
//   - If no resourcedetection processor used, declare & use a minimal resourcedetection processor
//   - No infraattributes processor configured nor declared
//   - remove infraattributes processor from metrics processors pipeline
//   - At least one otlphttpexporter with dd-api-key declared & used
//   - Check if used otlphttpexporter has dd-api-key as string, if not string convert it, if not at all notify user
//   - If profiling::symbol_uploader::enabled == true, convert api_key/app_key to strings in each endpoint
//   - If no profiling is used & configured, add minimal one with symbol_uploader: false
//   - remove ddprofiling & hpflare extensions
type converterWithoutAgent struct{}

func newConverterWithoutAgent(convSettings confmap.ConverterSettings) confmap.Converter {
	logger := convSettings.Logger
	slog.SetDefault(slog.New(zapslog.NewHandler(logger.Core())))
	return &converterWithoutAgent{}
}

func (c *converterWithoutAgent) Convert(_ context.Context, conf *confmap.Conf) error {
	confStringMap := xconfmap.ToStringMapRaw(conf)

	profilesPipeline, err := Ensure[confMap](confStringMap, "service::pipelines::profiles")
	if err != nil {
		return err
	}

	// no need to check for errors here as we directly depend on profilesPipeline that had to be valid for this code
	// path to be executed
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

	// If there's no otlphttpexporter configured. We can't infer necessary configurations as it needs URLs and API keys
	// so if nothing is found, notify user
	if err := c.ensureOtlpHTTPExporterConfig(confStringMap, exporterNames); err != nil {
		return err
	}

	// Determines what components we need to check and ensures at least one resourcedetection is configured
	// Deletes any infraattributes configured in the profiles pipeline
	newProcessorNames, err := c.fixProcessorsPipeline(confStringMap, processorNames)
	if err != nil {
		return err
	}
	newProcessorNames, err = addProfilerMetadataTags(confStringMap, newProcessorNames)
	if err != nil {
		return err
	}
	profilesPipeline["processors"] = newProcessorNames

	// Ensures at least one profiling receiver is used & configured
	// If not, create a minimal component with symbol uploading disabled
	newReceiverNames, err := c.fixReceiversPipeline(confStringMap, receiverNames)
	if err != nil {
		return err
	}
	profilesPipeline["receivers"] = newReceiverNames

	// Go through every configured processor to make sure there are no infraattributes declared that were not in the
	// pipeline
	if err := c.ensureGlobalProcessors(confStringMap); err != nil {
		return err
	}

	// Remove agent-only extensions
	if err := c.removeAgentOnlyExtensions(confStringMap); err != nil {
		return err
	}

	// infraattributes processor can also be used in metrics pipeline
	if err := c.ensureMetricsPipeline(confStringMap); err != nil {
		return err
	}

	// Add internal health metrics pipeline
	// Get updated exporter list from profiles pipeline (may have been modified by ensureOtlpHTTPExporterConfig)
	updatedExporterNames, err := Ensure[[]any](profilesPipeline, "exporters")
	if err != nil {
		return err
	}
	if err := c.addInternalHealthMetricsPipeline(confStringMap, updatedExporterNames, newProcessorNames); err != nil {
		slog.Warn("failed to configure internal health metric pipeline, skipping", slog.Any("error", err))
	}

	*conf = *confmap.NewFromStringMap(confStringMap)
	return nil
}

func (c *converterWithoutAgent) ensureMetricsPipeline(conf confMap) error {
	metrics, err := Ensure[confMap](conf, "service::pipelines::metrics")
	if err != nil {
		return err
	}

	processors, err := Ensure[[]any](metrics, "processors")
	if err != nil {
		return err
	}

	filteredProcessors := make([]any, 0, len(processors))
	for _, processorAny := range processors {
		processor, ok := processorAny.(string)
		if !ok {
			return errors.New("processors in metrics pipeline should be strings")
		}

		if isComponentType(processor, componentTypeInfraAttributes) {
			continue
		}

		filteredProcessors = append(filteredProcessors, processorAny)
	}

	metrics["processors"] = filteredProcessors

	// infraattributes processor should've already been deleted by now, no need to check
	return nil
}

func (c *converterWithoutAgent) ensureGlobalProcessors(conf confMap) error {
	processors, err := Ensure[confMap](conf, "processors")
	if err != nil {
		return err
	}

	for name := range processors {
		if isComponentType(name, componentTypeInfraAttributes) {
			delete(processors, name)
		}
	}
	return nil
}

func (c *converterWithoutAgent) fixProcessorsPipeline(conf confMap, processorNames []any) ([]any, error) {
	processors, err := Ensure[confMap](conf, "processors")
	if err != nil {
		return nil, err
	}
	foundResourcedetection := false
	toDelete := make(map[string]bool)

	// remove infraattributes, track & sanitize resourcedetection
	for _, nameAny := range processorNames {
		name, ok := nameAny.(string)
		if !ok {
			return nil, fmt.Errorf("processor name must be a string, got %T", nameAny)
		}

		// Remove infraattributes from pipeline and global config
		if isComponentType(name, componentTypeInfraAttributes) {
			delete(processors, name)
			toDelete[name] = true
			continue
		}

		// Track if we have resourcedetection
		if isComponentType(name, componentTypeResourceDetection) {
			if resourceDetectionConfig, ok := Get[confMap](conf, pathPrefixProcessors+name); ok {
				if err := c.ensureResourceDetectionConfig(resourceDetectionConfig); err != nil {
					return nil, err
				}
			}
			foundResourcedetection = true
		}
	}

	// Add resourcedetection/default if none found
	if !foundResourcedetection {
		if err := Set(processors, defaultResourceDetectionName, resourceDetectionDefaultConfig); err != nil {
			return nil, err
		}
		slog.Warn("Added minimal resourcedetection processor to user configuration")
		processorNames = append(processorNames, defaultResourceDetectionName)
	}

	// Remove processors marked for deletion
	processorNames = slices.DeleteFunc(processorNames, func(processor any) bool {
		name := processor.(string)
		_, exists := toDelete[name]
		return exists
	})

	return processorNames, nil
}

func (c *converterWithoutAgent) ensureResourceDetectionConfig(resourceDetection confMap) error {
	detectors, err := Ensure[[]any](resourceDetection, "detectors")
	if err != nil {
		return err
	}

	hasSystemDetector := slices.ContainsFunc(detectors, func(detector any) bool {
		d, ok := detector.(string)
		return ok && d == "system"
	})

	if !hasSystemDetector {
		resourceDetection["detectors"] = append(detectors, "system")
	}

	// Always ensure host.arch is enabled
	ddDefaultValue, err := SetDefault(resourceDetection, "system::resource_attributes::host.arch::enabled", true)
	if err != nil {
		return err
	}
	if !ddDefaultValue {
		slog.Warn("host.arch is required but is disabled by user configuration; preserving user value. Profiles for compiled languages will be missing symbols.")
	}

	// Only set these defaults if we added the system detector
	if !hasSystemDetector {
		if _, err := SetDefault(resourceDetection, "system::resource_attributes::host.name::enabled", false); err != nil {
			return err
		}
		if _, err := SetDefault(resourceDetection, "system::resource_attributes::os.type::enabled", false); err != nil {
			return err
		}
	}

	return nil
}

// fixReceiversPipeline ensures at least one profiling receiver is configured in the pipeline
// If none exists, it adds a minimal profiling receiver with symbol_uploader disabled
func (c *converterWithoutAgent) fixReceiversPipeline(conf confMap, receiverNames []any) ([]any, error) {
	// Check if profiling is in the pipeline
	hasProfiling := false
	for _, nameAny := range receiverNames {
		name, ok := nameAny.(string)
		if !ok {
			return nil, fmt.Errorf("receiver name must be a string, got %T", nameAny)
		}

		if !isComponentType(name, componentTypeProfiling) {
			continue
		}

		hasProfiling = true

		if profilingConfig, ok := Get[confMap](conf, pathPrefixReceivers+name); ok {
			if err := c.checkProfilingReceiverConfig(profilingConfig); err != nil {
				return nil, err
			}
		}
	}

	if hasProfiling {
		return receiverNames, nil
	}

	// Ensure default config exists if profiling receiver is not configured
	if err := Set(conf, pathPrefixReceivers+defaultProfilingName+"::"+pathSymbolUploaderEnabled, false); err != nil {
		return nil, err
	}

	slog.Warn("Added minimal profiling receiver to user configuration")
	return append(receiverNames, defaultProfilingName), nil
}

// checkProfilingReceiverConfig validates and normalizes a profiling receiver configuration.
// It ensures that if symbol_uploader is enabled, symbol_endpoints is properly configured
// and all api_key/app_key values are strings.
func (c *converterWithoutAgent) checkProfilingReceiverConfig(profiling confMap) error {
	if isEnabled, ok := Get[bool](profiling, pathSymbolUploaderEnabled); !ok || !isEnabled {
		return nil
	}

	endpoints, ok := Get[[]any](profiling, pathSymbolEndpoints)

	if !ok {
		return errors.New("symbol_endpoints must be a list")
	}

	if len(endpoints) == 0 {
		return errors.New("symbol_endpoints cannot be empty when symbol_uploader is enabled")
	}

	for _, epAny := range endpoints {
		if ep, ok := epAny.(confMap); ok {
			ensureKeyStringValue(ep, fieldAPIKey)
			ensureKeyStringValue(ep, fieldAppKey)
		}
	}
	return nil
}

func (c *converterWithoutAgent) ensureOtlpHTTPExporterConfig(conf confMap, exporterNames []any) error {
	// for each otlphttpexporter used, check if necessary api key is present
	hasOtlpHTTP := false
	for _, nameAny := range exporterNames {
		if name, ok := nameAny.(string); ok && isComponentType(name, componentTypeOtlpHTTP) {
			hasOtlpHTTP = true

			if _, err := SetDefault(conf, pathPrefixExporters+name+"::compression", "zstd"); err != nil {
				return err
			}

			headers, err := Ensure[confMap](conf, pathPrefixExporters+name+"::headers")
			if err != nil {
				return err
			}

			if !ensureKeyStringValue(headers, fieldDDAPIKey) {
				return fmt.Errorf("%s exporter missing required dd-api-key header", name)
			}
			if _, err := SetDefault(headers, fieldDDEVPOrigin, version.ProfilerName); err != nil {
				return err
			}
			if _, err := SetDefault(headers, fieldDDEVPOriginVersion, version.ProfilerVersion); err != nil {
				return err
			}
		}
	}

	if !hasOtlpHTTP {
		return errors.New("no otlphttp exporter configured in profiles pipeline")
	}

	return nil
}

func (c *converterWithoutAgent) removeAgentOnlyExtensions(conf confMap) error {
	service, err := Ensure[confMap](conf, "service")
	if err != nil {
		return err
	}

	extensions, ok := Get[[]any](service, "extensions")
	if !ok {
		return nil
	}

	// Filter out agent-only extensions
	filteredExtensions := make([]any, 0, len(extensions))
	for _, extAny := range extensions {
		ext, ok := extAny.(string)
		if !ok {
			return errors.New("extension names in service should be strings")
		}

		// Skip ddprofiling and hpflare extensions
		if isComponentType(ext, componentTypeDDProfiling) || isComponentType(ext, componentTypeHPFlare) {
			continue
		}

		filteredExtensions = append(filteredExtensions, extAny)
	}

	service["extensions"] = filteredExtensions

	// Also remove the extension definitions from global config
	extensionsConf, ok := Get[confMap](conf, "extensions")
	if ok {
		for name := range extensionsConf {
			if isComponentType(name, componentTypeDDProfiling) || isComponentType(name, componentTypeHPFlare) {
				delete(extensionsConf, name)
			}
		}
	}

	return nil
}

// addInternalHealthMetricsPipeline scrapes OTel collector internal telemetry and exports it
// to the same orgs as profiles. Separate from ensureMetricsPipeline which handles user-defined pipelines.
func (c *converterWithoutAgent) addInternalHealthMetricsPipeline(conf confMap, profilesExporterNames []any, profilesProcessors []any) error {
	if existing, ok := Get[confMap](conf, "service::pipelines::"+internalHealthMetricsPipelineName); ok {
		slog.Warn("metrics/profiler-internal-health pipeline already configured, skipping auto-configuration",
			slog.Any("existing_config", existing))
		return nil
	}

	if level, ok := Get[string](conf, "service::telemetry::metrics::level"); ok {
		if strings.ToLower(level) == "none" {
			slog.Info("metrics telemetry disabled (level=none), skipping metrics pipeline")
			return nil
		}
	}

	if receivers, ok := Get[confMap](conf, "receivers"); ok {
		if _, exists := receivers[reservedPrometheusReceiver]; exists {
			slog.Warn("receiver name conflicts with reserved name, skipping pipeline",
				slog.String("receiver", reservedPrometheusReceiver))
			return nil
		}
	}
	if processors, ok := Get[confMap](conf, "processors"); ok {
		for _, reserved := range []string{reservedFilterProcessor, reservedCumulativeToDeltaProcessor} {
			if _, exists := processors[reserved]; exists {
				slog.Warn("processor name conflicts with reserved name, skipping pipeline",
					slog.String("processor", reserved))
				return nil
			}
		}
	}

	metricsExporterNames := []any{}
	for _, exporterNameAny := range profilesExporterNames {
		exporterName, ok := exporterNameAny.(string)
		if !ok {
			continue
		}

		if !isComponentType(exporterName, componentTypeOtlpHTTP) {
			continue
		}

		exporterConf, ok := Get[confMap](conf, pathPrefixExporters+exporterName)
		if !ok {
			slog.Warn("exporter not found in config", slog.String("exporter", exporterName))
			continue
		}

		if _, hasMetrics := Get[string](exporterConf, "metrics_endpoint"); hasMetrics {
			slog.Debug("metrics_endpoint already set, preserving user config", slog.String("exporter", exporterName))
			metricsExporterNames = append(metricsExporterNames, exporterName)
			continue
		}

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
		slog.Info("no exporters configured, skipping metrics pipeline")
		return nil
	}

	if err := Set(conf, pathPrefixReceivers+reservedPrometheusReceiver, PrometheusReceiverConfig()); err != nil {
		return fmt.Errorf("failed to add prometheus receiver: %w", err)
	}

	if err := Set(conf, pathPrefixProcessors+reservedFilterProcessor, FilterProcessorConfig()); err != nil {
		return fmt.Errorf("failed to add filter processor: %w", err)
	}
	if err := Set(conf, pathPrefixProcessors+reservedCumulativeToDeltaProcessor, confMap{}); err != nil {
		return fmt.Errorf("failed to add cumulativetodelta processor: %w", err)
	}

	metricsProcessors := []any{reservedFilterProcessor, reservedCumulativeToDeltaProcessor}
	metricsProcessors = append(metricsProcessors, profilesProcessors...)

	metricsPipeline := confMap{
		"receivers":  []any{reservedPrometheusReceiver},
		"processors": metricsProcessors,
		"exporters":  metricsExporterNames,
	}

	if err := Set(conf, "service::pipelines::"+internalHealthMetricsPipelineName, metricsPipeline); err != nil {
		return fmt.Errorf("failed to create pipeline: %w", err)
	}

	slog.Info("created internal health metrics pipeline",
		slog.Int("exporters", len(metricsExporterNames)),
		slog.String("pipeline", internalHealthMetricsPipelineName))

	return nil
}
