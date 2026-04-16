// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

// Package agentprovider generates OpenTelemetry Collector configuration from Datadog Agent configuration.
package agentprovider

import (
	"fmt"
	"net"
	"strconv"

	"github.com/DataDog/datadog-agent/comp/host-profiler/collector/impl/converters"
	"github.com/DataDog/datadog-agent/comp/host-profiler/collector/impl/params"
	"github.com/DataDog/datadog-agent/comp/host-profiler/version"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

type confMap = map[string]any

func buildReceivers(conf confMap, agent configManager) []any {
	receivers := make(confMap)

	profiling := make(confMap)
	_ = converters.Set(profiling, "symbol_uploader::enabled", true)

	symbolEndpoints := make([]any, 0, agent.endpointsTotalLength)
	for _, endpoint := range agent.endpoints {
		for _, key := range endpoint.apiKeys {
			symbolEndpoints = append(symbolEndpoints, confMap{
				"site":    endpoint.site,
				"api_key": key,
			})
		}
	}

	_ = converters.Set(profiling, "symbol_uploader::symbol_endpoints", symbolEndpoints)

	receivers["profiling"] = profiling
	conf["receivers"] = receivers
	return []any{"profiling"}
}

func buildExporters(conf confMap, agent configManager) []any {
	const (
		profilesEndpointFormat = "https://intake.profile.%s/v1development/profiles"
		metricsEndpointFormat  = "https://otlp.%s/v1/metrics"
		otlpHTTPNameFormat     = "otlphttp/%s_%d"
		debugExporterName      = "debug"
	)

	exporters := make(confMap)

	createOtlpHTTPFromEndpoint := func(site, key string) confMap {
		headers := make(confMap, 3+len(agent.hostProfilerConfig.AdditionalHTTPHeaders))
		for k, v := range agent.hostProfilerConfig.AdditionalHTTPHeaders {
			headers[k] = v
		}
		// Required headers set after additional headers to prevent overrides
		headers["dd-api-key"] = key
		headers["dd-evp-origin"] = version.ProfilerName
		headers["dd-evp-origin-version"] = version.ProfilerVersion
		return confMap{
			"profiles_endpoint": fmt.Sprintf(profilesEndpointFormat, site),
			"metrics_endpoint":  fmt.Sprintf(metricsEndpointFormat, site),
			"compression":       "zstd",
			"headers":           headers,
		}
	}

	debugEnabled := agent.hostProfilerConfig.DebugVerbosity != ""
	capacity := agent.endpointsTotalLength
	if debugEnabled {
		capacity++
	}
	profilesExporters := make([]any, 0, capacity)
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

	if debugEnabled {
		exporters[debugExporterName] = confMap{
			"verbosity": agent.hostProfilerConfig.DebugVerbosity,
		}
		profilesExporters = append(profilesExporters, debugExporterName)
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

func buildMetricsTelemetry(conf confMap, healthMetrics healthMetricsConfig) {
	if !healthMetrics.Enabled {
		_ = converters.Set(conf, "service::telemetry::metrics::level", "none")
		return
	}
	host, portStr, _ := net.SplitHostPort(healthMetrics.Target)
	port, _ := strconv.Atoi(portStr)
	_ = converters.Set(conf, "service::telemetry::metrics::readers", []any{
		confMap{"pull": confMap{"exporter": confMap{"prometheus": confMap{"host": host, "port": port}}}},
	})
}

func buildMetricsPipeline(conf confMap, enableGoRuntimeMetrics bool, healthMetrics healthMetricsConfig, profilesProcessors, profilesExporters []any) {
	if !healthMetrics.Enabled && !enableGoRuntimeMetrics {
		return
	}

	metricsPipeline, _ := converters.Ensure[confMap](conf, "service::pipelines::metrics")
	receivers, _ := converters.Ensure[confMap](conf, "receivers")
	processors, _ := converters.Ensure[confMap](conf, "processors")

	var metricsReceivers []any
	metricsProcessors := profilesProcessors

	if healthMetrics.Enabled {
		receivers["prometheus"] = converters.PrometheusReceiverConfigWithTarget(healthMetrics.Target)
		processors["filter"] = converters.FilterProcessorConfig()
		processors["cumulativetodelta"] = confMap{}
		metricsProcessors = append([]any{"filter", "cumulativetodelta"}, profilesProcessors...)
		metricsReceivers = append(metricsReceivers, "prometheus")
	}

	if enableGoRuntimeMetrics {
		receivers["otlp"] = confMap{"protocols": confMap{"grpc": nil, "http": nil}}
		metricsReceivers = append(metricsReceivers, "otlp")
	}

	metricsPipeline["receivers"] = metricsReceivers
	metricsPipeline["processors"] = metricsProcessors
	metricsPipeline["exporters"] = profilesExporters
}

func buildConfig(agent configManager, p params.CollectorParams) confMap {
	config := make(confMap)

	profilesPipeline, _ := converters.Ensure[confMap](config, "service::pipelines::profiles")

	profilesProcessors := buildProcessors(config)
	profilesExporters := buildExporters(config, agent)
	profilesReceivers := buildReceivers(config, agent)

	profilesPipeline["processors"] = profilesProcessors
	profilesPipeline["exporters"] = profilesExporters
	profilesPipeline["receivers"] = profilesReceivers

	buildMetricsTelemetry(config, agent.hostProfilerConfig.HealthMetrics)
	buildMetricsPipeline(config, p.GetGoRuntimeMetrics(), agent.hostProfilerConfig.HealthMetrics, profilesProcessors, profilesExporters)

	_ = converters.Set(config, "extensions::hpflare/default", confMap{})
	serviceExtensions := []any{"hpflare/default"}
	if agent.hostProfilerConfig.DDProfilingEnabled {
		ddprofilingConfig := make(confMap)
		if agent.hostProfilerConfig.DDProfilingPeriod > 0 {
			_ = converters.Set(ddprofilingConfig, "profiler_options::period", agent.hostProfilerConfig.DDProfilingPeriod)
		}
		_ = converters.Set(config, "extensions::ddprofiling/default", ddprofilingConfig)
		serviceExtensions = append(serviceExtensions, "ddprofiling/default")
	}
	_ = converters.Set(config, "service::extensions", serviceExtensions)

	log.Debugf("Generated configuration: %+v", config)

	return config
}
