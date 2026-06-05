// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

package converters

// TODO: refactor Prometheus common code with ddot

import (
	"strconv"

	"github.com/DataDog/datadog-agent/pkg/util/confmaputils"
	"github.com/DataDog/datadog-agent/pkg/util/hostport"
)

const (
	defaultTelemetryReaderTarget = "localhost:8888"
)

func selectTelemetryPrometheusTargets(conf confMap) ([]string, bool) {
	metrics, ok := confmaputils.Get[confMap](conf, "service::telemetry::metrics")
	if !ok {
		return []string{defaultTelemetryReaderTarget}, true
	}
	readers, exists := metrics["readers"]
	if !exists {
		return []string{defaultTelemetryReaderTarget}, true
	}

	readersSlice, ok := readers.([]any)
	if !ok || len(readersSlice) == 0 {
		return nil, false
	}

	targets := []string{}
	for _, reader := range readersSlice {
		readerConfMap, ok := reader.(confMap)
		if !ok || len(readerConfMap) != 1 {
			continue
		}

		prometheusExporter, ok := confmaputils.Get[confMap](readerConfMap, "pull::exporter::prometheus")
		if !ok {
			continue
		}

		host, hostOk := confmaputils.Get[string](prometheusExporter, "host")
		port, portOk := confmaputils.Get[int](prometheusExporter, "port")
		if !hostOk || !portOk {
			continue
		}

		targets = append(targets, hostport.Join(host, strconv.Itoa(port)))
	}

	if len(targets) == 0 {
		return nil, false
	}
	return targets, true
}

func exportersInMetricsPipelinesWithReceiver(conf confMap, scrapeTargets []string, exporterFilter func(string) bool) (map[string]bool, bool) {
	coveredExporters := make(map[string]bool)
	targetSet := make(map[string]struct{}, len(scrapeTargets))
	for _, scrapeTarget := range scrapeTargets {
		targetSet[scrapeTarget] = struct{}{}
	}

	cfgsByReceiver := findComponentConfigs(conf, "prometheus", "receivers")
	for receiverName, receiver := range cfgsByReceiver {
		prometheusConfig, ok := receiver["config"]
		if !ok {
			continue
		}
		prometheusConfigMap, ok := prometheusConfig.(confMap)
		if !ok {
			return nil, false
		}
		prometheusScrapeConfigs, ok := prometheusConfigMap["scrape_configs"]
		if !ok {
			continue
		}
		prometheusScrapeConfigsSlice, ok := prometheusScrapeConfigs.([]any)
		if !ok {
			return nil, false
		}
		for _, scrapeConfig := range prometheusScrapeConfigsSlice {
			scrapeConfigMap, ok := scrapeConfig.(confMap)
			if !ok {
				return nil, false
			}
			staticConfigSlice, ok := confmaputils.Get[[]any](scrapeConfigMap, "static_configs")
			if !ok {
				continue
			}
			for _, staticConfig := range staticConfigSlice {
				staticConfigMap, ok := staticConfig.(confMap)
				if !ok {
					return nil, false
				}
				targets, ok := staticConfigMap["targets"]
				if !ok {
					continue
				}
				targetsSlice, ok := targets.([]any)
				if !ok {
					return nil, false
				}
				for _, target := range targetsSlice {
					targetString, ok := target.(string)
					if !ok {
						return nil, false
					}
					if _, ok := targetSet[targetString]; ok {
						for _, exporterName := range receiverInMetricsPipelineWithExporters(conf, receiverName, exporterFilter) {
							coveredExporters[exporterName] = true
						}
					}
				}
			}
		}
	}
	return coveredExporters, true
}

func receiverInMetricsPipelineWithExporters(conf confMap, receiverName string, exporterFilter func(string) bool) []string {
	var exporters []string
	pipelines, ok := confmaputils.Get[confMap](conf, "service::pipelines")
	if !ok {
		return nil
	}
	for pipelineName, pipeline := range pipelines {
		if !confmaputils.IsComponentType(pipelineName, "metrics") {
			continue
		}
		pipelineConfMap, ok := pipeline.(confMap)
		if !ok {
			continue
		}
		pipelineExporters, ok := confmaputils.Get[[]any](pipelineConfMap, "exporters")
		if !ok {
			continue
		}
		for _, exporter := range pipelineExporters {
			exporterName, ok := exporter.(string)
			if !ok || !exporterFilter(exporterName) {
				continue
			}
			pipelineReceivers, ok := confmaputils.Get[[]any](pipelineConfMap, "receivers")
			if !ok {
				continue
			}
			for _, receiver := range pipelineReceivers {
				receiverNameInPipeline, ok := receiver.(string)
				if ok && receiverNameInPipeline == receiverName {
					exporters = append(exporters, exporterName)
				}
			}
		}
	}
	return exporters
}

func findComponentConfigs(conf confMap, componentName string, componentType string) map[string]confMap {
	componentsMap, ok := confmaputils.Get[confMap](conf, componentType)
	if !ok {
		return nil
	}

	configsByComponent := make(map[string]confMap)
	for name, cfg := range componentsMap {
		if !confmaputils.IsComponentType(name, componentName) {
			continue
		}
		cfgMap, ok := cfg.(confMap)
		if !ok {
			cfgMap = nil // some components can leave configs empty and use defaults
		}
		configsByComponent[name] = cfgMap
	}
	return configsByComponent
}
