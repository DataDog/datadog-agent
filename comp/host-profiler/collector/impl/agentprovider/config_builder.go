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

	"github.com/DataDog/datadog-agent/comp/host-profiler/collector/impl/extensions/hpflareextension"
	"github.com/DataDog/datadog-agent/comp/host-profiler/collector/impl/params"
	"github.com/DataDog/datadog-agent/comp/host-profiler/symboluploader/cgroup"
	"github.com/DataDog/datadog-agent/comp/host-profiler/version"
	"github.com/DataDog/datadog-agent/pkg/util/confmaputils"
)

type confMap = map[string]any

const (
	infraAttributesName = "infraattributes"
	hpflareName         = "hpflare"
	ddprofilingName     = "ddprofiling"
)

func buildReceivers(conf confMap, agent configManager) []any {
	receivers := make(confMap)

	profiling := make(confMap)
	_ = confmaputils.Set(profiling, "symbol_uploader::enabled", true)
	if agent.hostProfilerConfig.HeapProfiling {
		profiling["heap_profiling"] = true
	}
	if agent.hostProfilerConfig.LiveHeapProfiling {
		profiling["live_heap_profiling"] = true
	}
	if agent.hostProfilerConfig.Tracers != "" {
		profiling["tracers"] = agent.hostProfilerConfig.Tracers
	}

	symbolEndpoints := make([]any, 0, agent.endpointsTotalLength)
	for _, endpoint := range agent.endpoints {
		for _, key := range endpoint.apiKeys {
			symbolEndpoints = append(symbolEndpoints, confMap{
				"site":    endpoint.site,
				"api_key": key,
			})
		}
	}

	_ = confmaputils.Set(profiling, "symbol_uploader::symbol_endpoints", symbolEndpoints)

	receivers["profiling"] = profiling
	conf["receivers"] = receivers
	return []any{"profiling"}
}

func buildExporters(conf confMap, agent configManager) []any {
	const (
		endpointFormat     = "https://otlp.%s"
		otlpHTTPNameFormat = "otlp_http/%s_%d"
		debugExporterName  = "debug"
	)

	exporters := make(confMap)

	createOtlpHTTPFromEndpoint := func(site, key string) confMap {
		headers := make(confMap, 3+len(agent.hostProfilerConfig.AdditionalHTTPHeaders))
		for k, v := range agent.hostProfilerConfig.AdditionalHTTPHeaders {
			headers[k] = v
		}
		// Required headers set after additional headers to prevent overrides
		headers["dd-api-key"] = key
		headers["dd-evp-origin"] = version.BundledProfilerName
		headers["dd-evp-origin-version"] = version.ProfilerVersion
		headers["dd-otel-metric-config"] = `{"resource_attributes_as_tags": true}`
		return confMap{
			"endpoint":    fmt.Sprintf(endpointFormat, site),
			"compression": "zstd",
			"headers":     headers,
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
			_ = confmaputils.Set(exporters, exporterName, createOtlpHTTPFromEndpoint(endpoint.site, key))
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
	_ = confmaputils.Set(processors, infraAttributesName, infraattributes)

	metadata := confMap{
		"attributes": []any{
			confMap{
				"key":    version.DDProfilerNameKey,
				"value":  version.BundledProfilerName,
				"action": "upsert",
			},
			confMap{
				"key":    version.DDProfilerVersionKey,
				"value":  version.ProfilerVersion,
				"action": "upsert",
			},
		},
	}
	_ = confmaputils.Set(processors, "resource/dd-profiler-internal-metadata", metadata)

	conf["processors"] = processors
	return []any{infraAttributesName, "resource/dd-profiler-internal-metadata"}
}

func buildMetricsTelemetry(conf confMap, healthMetrics healthMetricsConfig) {
	if !healthMetrics.Enabled {
		_ = confmaputils.Set(conf, "service::telemetry::metrics::level", "none")
		return
	}
	host, portStr, _ := net.SplitHostPort(healthMetrics.Target)
	port, _ := strconv.Atoi(portStr)
	_ = confmaputils.Set(conf, "service::telemetry::metrics::readers", []any{
		confMap{"pull": confMap{"exporter": confMap{"prometheus": confMap{"host": host, "port": port}}}},
	})
}

func buildMetricsPipeline(conf confMap, enableGoRuntimeMetrics bool, healthMetrics healthMetricsConfig, profilesProcessors, profilesExporters []any) {
	if !healthMetrics.Enabled && !enableGoRuntimeMetrics {
		return
	}

	metricsPipeline, _ := confmaputils.Ensure[confMap](conf, "service::pipelines::metrics")
	receivers, _ := confmaputils.Ensure[confMap](conf, "receivers")
	processors, _ := confmaputils.Ensure[confMap](conf, "processors")

	var metricsReceivers []any
	metricsProcessors := profilesProcessors

	if healthMetrics.Enabled {
		receivers["prometheus"] = confmaputils.PrometheusReceiverConfig("host-profiler-internal", healthMetrics.Target)
		processors["filter"] = confmaputils.FilterProcessorConfig()
		processors["cumulativetodelta"] = confMap{}
		metricsProcessors = append([]any{"filter", "cumulativetodelta"}, profilesProcessors...)
		metricsReceivers = append(metricsReceivers, "prometheus")
	}

	if enableGoRuntimeMetrics {
		receivers["otlp"] = confMap{"protocols": confMap{"grpc": nil, "http": nil}}
		metricsReceivers = append(metricsReceivers, "otlp")
	}

	if containerID, err := cgroup.GetSelfContainerID(); err == nil {
		const containerIDProcessorName = "resource/dd-profiler-metrics-containerid"
		processors[containerIDProcessorName] = confMap{
			"attributes": []any{confMap{
				"key":    version.OTelContainerIDKey,
				"value":  containerID,
				"action": "insert",
			}},
		}
		metricsProcessors = append([]any{containerIDProcessorName}, metricsProcessors...)
	}

	metricsPipeline["receivers"] = metricsReceivers
	metricsPipeline["processors"] = metricsProcessors
	metricsPipeline["exporters"] = profilesExporters
}

func buildConfig(agent configManager, p params.CollectorParams) confMap {
	config := make(confMap)

	profilesPipeline, _ := confmaputils.Ensure[confMap](config, "service::pipelines::profiles")

	profilesProcessors := buildProcessors(config)
	profilesExporters := buildExporters(config, agent)
	profilesReceivers := buildReceivers(config, agent)

	profilesPipeline["processors"] = profilesProcessors
	profilesPipeline["exporters"] = profilesExporters
	profilesPipeline["receivers"] = profilesReceivers

	buildMetricsTelemetry(config, agent.hostProfilerConfig.HealthMetrics)
	buildMetricsPipeline(config, p.GetGoRuntimeMetrics(), agent.hostProfilerConfig.HealthMetrics, profilesProcessors, profilesExporters)

	hpflareConf := confMap{"endpoint": fmt.Sprintf("localhost:%d", hpflareextension.EffectivePort(agent.hostProfilerConfig.HPFlare.Port))}
	_ = confmaputils.Set(config, "extensions::"+hpflareName, hpflareConf)
	serviceExtensions := []any{hpflareName}
	if agent.hostProfilerConfig.DDProfiling.Enabled {
		ddprofilingConf := make(confMap)
		if agent.hostProfilerConfig.DDProfiling.Period > 0 {
			_ = confmaputils.Set(ddprofilingConf, "profiler_options::period", agent.hostProfilerConfig.DDProfiling.Period)
		}
		if agent.hostProfilerConfig.DDProfiling.Port > 0 {
			// The ddprofiling extension expects a bare port for its "endpoint" field.
			_ = confmaputils.Set(ddprofilingConf, "endpoint", strconv.Itoa(agent.hostProfilerConfig.DDProfiling.Port))
		}
		_ = confmaputils.Set(config, "extensions::"+ddprofilingName, ddprofilingConf)
		serviceExtensions = append(serviceExtensions, ddprofilingName)
	}
	_ = confmaputils.Set(config, "service::extensions", serviceExtensions)

	return config
}
