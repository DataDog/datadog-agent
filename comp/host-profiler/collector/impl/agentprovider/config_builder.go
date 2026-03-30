// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

//go:build linux

// Package agentprovider generates OpenTelemetry Collector configuration from Datadog Agent configuration.
package agentprovider

import (
	"fmt"
	"os"
	"strings"

	"github.com/DataDog/datadog-agent/comp/host-profiler/collector/impl/converters"
	"github.com/DataDog/datadog-agent/comp/host-profiler/version"
	"github.com/DataDog/datadog-agent/pkg/trace/traceutil/normalize"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

// parseAdditionalHeaders parses a space or comma separated list of key:value
// pairs, normalizes each entry, and returns them as a comma-separated string.
func parseAdditionalHeaders(raw string) string {
	raw = strings.ReplaceAll(raw, ",", " ")
	parts := strings.Fields(raw)

	var valid []string
	for _, part := range parts {
		normalized := normalize.NormalizeTag(part)
		if normalized == "" {
			log.Warnf("Skipping invalid header entry %q", part)
			continue
		}
		valid = append(valid, normalized)
	}
	return strings.Join(valid, ",")
}

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
		headers := confMap{
			"dd-api-key":            key,
			"dd-evp-origin":         version.ProfilerName,
			"dd-evp-origin-version": version.ProfilerVersion,
		}
		if raw := os.Getenv("DD_HOST_PROFILER_ADDITIONAL_HEADERS"); raw != "" {
			if parsed := parseAdditionalHeaders(raw); parsed != "" {
				headers["x-datadog-additional-headers"] = parsed
				log.Infof("Setting x-datadog-additional-headers: %s", parsed)
			}
		}
		return confMap{
			"profiles_endpoint": fmt.Sprintf(profilesEndpointFormat, site),
			"metrics_endpoint":  fmt.Sprintf(metricsEndpointFormat, site),
			"headers":           headers,
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

func buildConfig(agent configManager) confMap {
	config := make(confMap)

	profilesPipeline, _ := converters.Ensure[confMap](config, "service::pipelines::profiles")

	profilesPipeline["processors"] = buildProcessors(config)
	profilesPipeline["exporters"] = buildExporters(config, agent)
	profilesPipeline["receivers"] = buildReceivers(config, agent)

	_ = converters.Set(config, "extensions::ddprofiling/default", confMap{})
	_ = converters.Set(config, "extensions::hpflare/default", confMap{})
	_ = converters.Set(config, "service::telemetry::metrics::level", "none")

	log.Debugf("Generated configuration: %+v", config)

	return config
}
