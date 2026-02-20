// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

//go:build linux

// Package agentprovider generates OpenTelemetry Collector configuration from Datadog Agent configuration.
package agentprovider

import (
	"fmt"

	"github.com/DataDog/datadog-agent/comp/host-profiler/collector/impl/converters"
	"github.com/DataDog/datadog-agent/comp/host-profiler/version"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

type confMap = map[string]any

func buildReceivers(conf confMap, agent configManager) []any {
	receivers := make(confMap)

	hostProfiler := make(confMap)
	_ = converters.Set(hostProfiler, "symbol_uploader::enabled", true)

	symbolEndpoints := make([]any, 0, agent.endpointsTotalLength)
	for _, endpoint := range agent.endpoints {
		for _, key := range endpoint.apiKeys {
			symbolEndpoints = append(symbolEndpoints, confMap{
				"site":    endpoint.site,
				"api_key": key,
			})
		}
	}

	_ = converters.Set(hostProfiler, "symbol_uploader::symbol_endpoints", symbolEndpoints)

	receivers["hostprofiler"] = hostProfiler
	conf["receivers"] = receivers
	return []any{"hostprofiler"}
}

func buildExporters(conf confMap, agent configManager) []any {
	const (
		profilesEndpointFormat = "https://intake.profile.%s/v1development/profiles"
		metricsEndpointFormat  = "https://otlp.%s/v1/metrics"
		otlpHTTPNameFormat     = "otlphttp/%s_%d"
	)

	exporters := make(confMap)

	createOtlpHTTPFromEndpoint := func(site, key string) confMap {
		return confMap{
			"profiles_endpoint": fmt.Sprintf(profilesEndpointFormat, site),
			"metrics_endpoint":  fmt.Sprintf(metricsEndpointFormat, site),
			"headers": confMap{
				"dd-api-key": key,
			},
		}
	}

	profilesExporters := make([]any, 0, agent.endpointsTotalLength)
	// Track exporter count per site to ensure unique names for duplicate sites
	siteExporterCount := make(map[string]int)
	for _, endpoint := range agent.endpoints {
		for _, key := range endpoint.apiKeys {
			index := siteExporterCount[endpoint.site]
			siteExporterCount[endpoint.site]++
			exporterName := fmt.Sprintf(otlpHTTPNameFormat, endpoint.site, index)
			_ = converters.Set(exporters, exporterName, createOtlpHTTPFromEndpoint(endpoint.site, key))
			profilesExporters = append(profilesExporters, exporterName)
		}
	}

	conf["exporters"] = exporters
	return profilesExporters
}

func buildProcessors(conf confMap) []any {
	processors := make(confMap)

	infraattributes := confMap{
		"allow_hostname_override": true,
		"cardinality":             2,
	}
	_ = converters.Set(processors, "infraattributes/default", infraattributes)

	metadata := confMap{
		"attributes": []any{
			confMap{
				"key":    "profiler_name",
				"value":  version.ProfilerName,
				"action": "upsert",
			},
			confMap{
				"key":    "profiler_version",
				"value":  version.ProfilerVersion,
				"action": "upsert",
			},
		},
	}
	_ = converters.Set(processors, "resource/dd-profiler-internal-metadata", metadata)

	conf["processors"] = processors
	return []any{"infraattributes/default", "resource/dd-profiler-internal-metadata"}
}

func buildMetricsPipeline(conf confMap, profilesProcessors, profilesExporters []any) {
	metricsPipeline, _ := converters.Ensure[confMap](conf, "service::pipelines::metrics")

	// Add OTLP receiver
	receivers, _ := converters.Ensure[confMap](conf, "receivers")
	receivers["otlp"] = confMap{
		"protocols": confMap{
			"grpc": nil,
			"http": nil,
		},
	}

	// Add cumulativetodelta processor
	processors, _ := converters.Ensure[confMap](conf, "processors")
	processors["cumulativetodelta"] = confMap{}

	// Build metrics processors: cumulativetodelta + profile processors (infraattributes, metadata)
	metricsProcessors := []any{"cumulativetodelta"}
	metricsProcessors = append(metricsProcessors, profilesProcessors...)

	// Use all exporters from profiles pipeline (they all have metrics_endpoint)
	metricsPipeline["receivers"] = []any{"otlp"}
	metricsPipeline["processors"] = metricsProcessors
	metricsPipeline["exporters"] = profilesExporters
}

func buildConfig(agent configManager, params CollectorParams) confMap {
	config := make(confMap)

	profilesPipeline, _ := converters.Ensure[confMap](config, "service::pipelines::profiles")

	profilesProcessors := buildProcessors(config)
	profilesExporters := buildExporters(config, agent)
	profilesReceivers := buildReceivers(config, agent)

	profilesPipeline["processors"] = profilesProcessors
	profilesPipeline["exporters"] = profilesExporters
	profilesPipeline["receivers"] = profilesReceivers

	if params.GetGoRuntimeMetrics() {
		buildMetricsPipeline(config, profilesProcessors, profilesExporters)
	}

	_ = converters.Set(config, "extensions::ddprofiling/default", confMap{})
	_ = converters.Set(config, "extensions::hpflare/default", confMap{})

	log.Debugf("Generated configuration: %+v", config)

	return config
}
