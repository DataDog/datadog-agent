// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

// Package converterimpl provides the implementation of the otel-agent converter.
package converterimpl

import (
	"context"
	"slices"

	"github.com/DataDog/datadog-agent/pkg/util/confmaputils"
)

var ddAutoconfiguredSuffix = confmaputils.AutoConfiguredSuffix

const (
	defaultSite = "datadoghq.com"
	secretRegex = "ENC\\[.*\\][ \t]*$"
)

type component struct {
	Type         string
	Name         string
	EnhancedName string
	Config       any
}

// Applies selected feature changes
func (c *ddConverter) enhanceConfig(ctx context.Context, conf confmaputils.ConfMap) {
	var enabledFeatures []string
	// If not specified, assume all features are enabled (ocb tests will not have coreConfig)
	if c.coreConfig != nil {
		enabledFeatures = c.coreConfig.GetStringSlice("otelcollector.converter.features")
	} else {
		enabledFeatures = []string{"infraattributes", "prometheus", "pprof", "zpages", "health_check", "ddflare", "datadog", "cumulativetodelta"}
	}

	// extensions (pprof, zpages, health_check, ddflare, datadog)
	extensions := createExtensions(enabledFeatures)
	for _, extension := range extensions {
		if !slices.Contains(enabledFeatures, extension.Name) || extensionIsInServicePipeline(conf, extension) {
			continue
		}
		if extension.Name == datadogName {
			if c.coreConfig == nil || c.coreConfig.GetString("api_key") == "" {
				continue
			}
			// User already defined a datadog extension but forgot to wire it into
			// service.extensions — reuse their definition instead of creating a
			// second datadog/dd-autoconfigured.
			if existingID := findExistingExtensionID(conf, datadogName); existingID != "" {
				wireExtensionIDToPipeline(conf, existingID)
				continue
			}
			site := defaultSite
			if c.coreConfig.GetString("site") != "" {
				site = c.coreConfig.GetString("site")
			}
			deploymentType := "daemonset"
			if c.coreConfig.GetBool("otelcollector.gateway.mode") {
				deploymentType = "gateway"
			}
			resolvedHostname := ""
			if c.hostname != nil {
				if hostname, err := c.hostname.Get(ctx); err == nil {
					resolvedHostname = hostname
				}
			}
			extension.Config = map[string]any{
				"api": map[string]any{
					"key":  c.coreConfig.GetString("api_key"),
					"site": site,
				},
				"deployment_type":     deploymentType,
				"hostname":            resolvedHostname,
				"installation_method": c.coreConfig.GetString("otelcollector.installation_method"),
			}
		}
		addComponentToConfig(conf, extension)
		addExtensionToPipeline(conf, extension)
	}

	// dogtel extension (standalone mode only)
	if c.coreConfig != nil && c.coreConfig.GetBool("otel_standalone") && !extensionIsInServicePipeline(conf, dogtelComponent) {
		if existingID := findExistingExtensionID(conf, dogtelName); existingID != "" {
			// User already defined a dogtel extension but forgot to wire it into
			// service.extensions — reuse their definition instead of creating a
			// second dogtel/dd-autoconfigured with empty config.
			wireExtensionIDToPipeline(conf, existingID)
		} else {
			addComponentToConfig(conf, dogtelComponent)
			addExtensionToPipeline(conf, dogtelComponent)
		}
	}

	// infra attributes processor (all pipeline types; safe in mixed-exporter
	// pipelines because it only adds resource attributes)
	if slices.Contains(enabledFeatures, "infraattributes") {
		c.addProcessorToPipelinesWithDDExporter(conf, infraAttributesProcessor, pipelineAll, false)
	}
	// cumulativetodelta processor (metrics pipelines only — the processor only
	// implements CreateMetrics, so it must never land in a traces/logs pipeline).
	// warnOnMixedExporters: it changes metric temporality for every exporter in the
	// pipeline, so warn (but still inject) when a metrics pipeline also fans out to a
	// non-Datadog exporter, and let the user decide whether to split it.
	if slices.Contains(enabledFeatures, "cumulativetodelta") {
		c.addProcessorToPipelinesWithDDExporter(conf, cumulativeToDeltaProcessor, pipelineMetrics, true)
	}
	// prometheus receiver
	if slices.Contains(enabledFeatures, "prometheus") {
		addPrometheusReceiver(conf, findInternalMetricsAddress(conf))
	}

	// add datadog agent sourced config
	addCoreAgentConfig(conf, c.coreConfig)

	// warn about problematic receiver configurations
	c.warnIfHostmetricsInConnectedMode(conf)
}

// warnIfHostmetricsInConnectedMode logs a warning when the hostmetrics receiver
// is configured while the OTel Agent runs in connected mode (not standalone).
// In connected mode the core Datadog Agent already collects host metrics, so the
// hostmetrics receiver will produce duplicate or conflicting metric names once
// the otel. prefix remapping is disabled.
func (c *ddConverter) warnIfHostmetricsInConnectedMode(conf confmaputils.ConfMap) {
	if c.coreConfig == nil || c.coreConfig.GetBool("otel_standalone") {
		return
	}
	if receivers := findComps(conf, "hostmetrics", "receivers"); len(receivers) > 0 {
		if c.logger != nil {
			c.logger.Warn("The hostmetrics receiver is enabled but the OTel Agent is running " +
				"in connected mode (DD_OTEL_STANDALONE=false). In connected mode, the core " +
				"Datadog Agent already collects host metrics. The hostmetrics receiver should " +
				"only be used in standalone mode (DD_OTEL_STANDALONE=true) to avoid metric conflicts.")
		}
	}
}

// pipelineType selects which service pipelines a processor is injected into.
type pipelineType string

const (
	// pipelineAll injects into every pipeline type (traces, metrics, logs).
	pipelineAll pipelineType = ""
	// pipelineMetrics injects into metrics pipelines only.
	pipelineMetrics pipelineType = "metrics"
)

// pipelineExportsToDatadog reports whether the pipeline exports to the datadog
// exporter. Matching is by base component name, so datadog/<name> counts too.
func pipelineExportsToDatadog(componentsMap map[string]any) bool {
	exporters, ok := componentsMap["exporters"].([]any)
	if !ok {
		return false
	}
	for _, exporter := range exporters {
		if s, ok := exporter.(string); ok && confmaputils.IsComponentType(s, "datadog") {
			return true
		}
	}
	return false
}

// pipelineHasNonDatadogExporter reports whether the pipeline exports to any exporter
// other than the datadog exporter (matched by base component name).
func pipelineHasNonDatadogExporter(componentsMap map[string]any) bool {
	exporters, ok := componentsMap["exporters"].([]any)
	if !ok {
		return false
	}
	for _, exporter := range exporters {
		if s, ok := exporter.(string); ok && !confmaputils.IsComponentType(s, "datadog") {
			return true
		}
	}
	return false
}

// pipelineHasProcessor reports whether the pipeline already lists a processor with
// the given base component name (so a user-defined processor is not duplicated).
func pipelineHasProcessor(componentsMap map[string]any, name string) bool {
	processors, ok := componentsMap["processors"].([]any)
	if !ok {
		return false
	}
	for _, processor := range processors {
		if s, ok := processor.(string); ok && confmaputils.IsComponentType(s, name) {
			return true
		}
	}
	return false
}

// addProcessorToPipelinesWithDDExporter injects comp (a processor) into every
// pipeline that exports to the datadog exporter, appending it to the end of the
// pipeline's processors list. pt restricts the eligible pipeline types
// (pipelineAll = every type, pipelineMetrics = metrics only). It is a no-op for a
// pipeline that already defines a processor with the same base name.
//
// When warnOnMixedExporters is set, injecting into a pipeline that also exports to a
// non-Datadog exporter emits a warning (the processor is still injected). Collector
// processors are pipeline-scoped and run before the fan-out to all exporters, so a
// processor that rewrites data — e.g. cumulativetodelta changing metric temporality —
// also alters what the non-Datadog exporter receives. This is the user's pipeline
// topology to own; we surface it loudly and let them split the pipeline if unintended.
func (c *ddConverter) addProcessorToPipelinesWithDDExporter(conf confmaputils.ConfMap, comp component, pt pipelineType, warnOnMixedExporters bool) {
	pipelinesMap, ok := confmaputils.Get[confmaputils.ConfMap](conf, "service::pipelines")
	if !ok {
		return
	}

	componentAddedToConfig := false
	for pipelineName, components := range pipelinesMap {
		// Restrict to the requested pipeline type when one is set.
		// IsComponentType handles both "metrics" and "metrics/foo".
		if pt != pipelineAll && !confmaputils.IsComponentType(pipelineName, string(pt)) {
			continue
		}
		componentsMap, ok := components.(map[string]any)
		if !ok {
			continue
		}
		if !pipelineExportsToDatadog(componentsMap) || pipelineHasProcessor(componentsMap, comp.Name) {
			continue
		}
		// Loudly flag mixed-exporter pipelines: the processor is pipeline-scoped, so it
		// rewrites data for the non-Datadog exporter(s) too. The user owns the topology
		// — inform them and let them split the pipeline (or disable the feature) if the
		// side effect is unwanted.
		if warnOnMixedExporters && c.logger != nil && pipelineHasNonDatadogExporter(componentsMap) {
			c.logger.Warn("Auto-injected the " + comp.Name + " processor into metrics pipeline \"" +
				pipelineName + "\", which also exports to a non-Datadog exporter. Because Collector " +
				"processors are pipeline-scoped, cumulative metrics are now converted to delta for ALL " +
				"exporters in this pipeline, not only datadog — this changes the temporality other exporters " +
				"receive. If that is unintended, route the datadog exporter through its own metrics pipeline, " +
				"or disable this by removing \"cumulativetodelta\" from otelcollector.converter.features.")
		}
		// The datadog exporter is present but this processor is not yet in the
		// pipeline: add it once to the top-level config, then to this pipeline.
		if !componentAddedToConfig {
			addComponentToConfig(conf, comp)
			componentAddedToConfig = true
		}
		addComponentToPipeline(conf, comp, pipelineName)
	}
}


// addComponentToConfig adds comp to the collector config. It supports receivers,
// processors, exporters and extensions.
func addComponentToConfig(conf confmaputils.ConfMap, comp component) {
	_ = confmaputils.Set(conf, comp.Type+"::"+comp.EnhancedName, comp.Config)
}

// addComponentToPipeline adds comp into pipelineName. If pipelineName does not exist,
// it creates it. It only supports receivers, processors and exporters.
func addComponentToPipeline(conf confmaputils.ConfMap, comp component, pipelineName string) {
	pipelinesMap, ok := confmaputils.Get[confmaputils.ConfMap](conf, "service::pipelines")
	if !ok {
		return
	}
	_, ok = pipelinesMap[pipelineName]
	if !ok {
		pipelinesMap[pipelineName] = map[string]any{}
	}
	pipelineMap, ok := pipelinesMap[pipelineName].(map[string]any)
	if !ok {
		return
	}

	_, ok = pipelineMap[comp.Type]
	if !ok {
		pipelineMap[comp.Type] = []any{}
	}
	if pipelineOfTypeSlice, ok := pipelineMap[comp.Type].([]any); ok {
		pipelineOfTypeSlice = append(pipelineOfTypeSlice, comp.EnhancedName)
		pipelineMap[comp.Type] = pipelineOfTypeSlice
	}
}

// findComps finds and returns the matching components and their configs.
// Component can be receivers, processors, connectors or exporters.
func findComps(conf confmaputils.ConfMap, compName string, compType string) map[string]confmaputils.ConfMap {
	compsMap, ok := confmaputils.Get[confmaputils.ConfMap](conf, compType)
	if !ok {
		return nil
	}
	cfgsByRecv := make(map[string]confmaputils.ConfMap)
	for name, cfg := range compsMap {
		if !confmaputils.IsComponentType(name, compName) {
			continue
		}
		cfgMap, ok := cfg.(confmaputils.ConfMap)
		if !ok {
			cfgMap = nil // some components like debug exporter can leave configs empty and use defaults
		}
		cfgsByRecv[name] = cfgMap
	}
	return cfgsByRecv
}
