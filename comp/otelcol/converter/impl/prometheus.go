// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

// Package converterimpl provides the implementation of the otel-agent converter.
package converterimpl

import (
	"strconv"
	"strings"

	"github.com/DataDog/datadog-agent/pkg/util/confmaputils"
	"github.com/DataDog/datadog-agent/pkg/util/hostport"
)

var (
	// prometheus
	prometheusName         = "prometheus"
	prometheusEnhancedName = prometheusName + "/" + ddAutoconfiguredSuffix
)

// addPrometheusReceiver ensures that each datadogexporter is configured with a prometheus receiver
// which points to the collectors internal telemetry metrics. In cases where this is not true, it adds
// a pipeline with the prometheus exporter and datadog exporter.
// todo(mackjmr): in the case where there are two datadog exporters with the same API key, we may not
// want to configure two pipelines with the prometheus receiver for each exporter, as this will lead
// to shipping the health metrics twice to the same org. If there are two datadog exporters with
// different API keys, there is no way to know if these are from the same org, so there is a risk
// of double shipping.
func addPrometheusReceiver(conf confmaputils.ConfMap, promServerAddr string) {
	if mLevel, ok := confmaputils.Get[string](conf, "service::telemetry::metrics::level"); ok && strings.ToLower(mLevel) == "none" {
		return
	}

	datadogExportersMap := getDatadogExporters(conf)

	// find prometheus receivers which point to internal telemetry metrics. If present, check if it is defined
	// in pipeline with DD exporter. If so, remove from datadog exporter map.
	cfgsByRecv := findComps(conf, prometheusName, "receivers")
	for name, cfg := range cfgsByRecv {
		prometheusConfig, ok := cfg["config"]
		if !ok {
			continue
		}
		prometheusConfigMap, ok := prometheusConfig.(map[string]any)
		if !ok {
			return
		}
		prometheusScrapeConfigs, ok := prometheusConfigMap["scrape_configs"]
		if !ok {
			continue
		}
		prometheusScrapeConfigsSlice, ok := prometheusScrapeConfigs.([]any)
		if !ok {
			return
		}
		for _, scrapeConfig := range prometheusScrapeConfigsSlice {
			scrapeConfigMap, ok := scrapeConfig.(map[string]any)
			if !ok {
				return
			}
			staticConfig, ok := scrapeConfigMap["static_configs"]
			if !ok {
				continue
			}
			staticConfigSlice, ok := staticConfig.([]any)
			if !ok {
				continue
			}
			for _, staticConfig := range staticConfigSlice {
				staticConfigMap, ok := staticConfig.(map[string]any)
				if !ok {
					return
				}
				targets, ok := staticConfigMap["targets"]
				if !ok {
					continue
				}
				targetsSlice, ok := targets.([]any)
				if !ok {
					return
				}
				for _, target := range targetsSlice {
					targetString, ok := target.(string)
					if !ok {
						return
					}
					if targetString == promServerAddr {
						if ddExporters := receiverInPipelineWithDatadogExporter(conf, name); ddExporters != nil {
							scrapeConfigMap["job_name"] = "datadog-agent"
							for _, ddExporter := range ddExporters {
								delete(datadogExportersMap, ddExporter)
							}
						}
					}
				}
			}
		}
	}

	if len(datadogExportersMap) == 0 {
		return
	}

	comp := component{
		Name:         prometheusName,
		EnhancedName: prometheusEnhancedName,
		Type:         "receivers",
		Config:       confmaputils.PrometheusReceiverConfig("datadog-agent", promServerAddr),
	}

	addComponentToConfig(conf, comp)

	processorInternalPipeline := getProcessorInternalPipeline()
	addComponentToConfig(conf, processorInternalPipeline)
	addDDExpToInternalPipeline(conf, []component{comp, processorInternalPipeline}, datadogExportersMap)
}

func addDDExpToInternalPipeline(conf confmaputils.ConfMap, comps []component, datadogExportersMap map[string]any) {
	for ddExporterName := range datadogExportersMap {
		pipelineName := "metrics" + "/" + ddAutoconfiguredSuffix + "/" + ddExporterName
		for _, comp := range comps {
			addComponentToPipeline(conf, comp, pipelineName)
		}
		addComponentToPipeline(conf, component{
			Type:         "exporters",
			EnhancedName: ddExporterName,
		}, pipelineName)
	}
}

func getProcessorInternalPipeline() component {
	name := "filter/drop-prometheus-internal-metrics"
	return component{
		Type:         "processors",
		Name:         name,
		EnhancedName: name + "/" + ddAutoconfiguredSuffix,
		Config:       confmaputils.FilterProcessorConfig(),
	}
}

func receiverInPipelineWithDatadogExporter(conf confmaputils.ConfMap, receiverName string) []string {
	var ddExporters []string
	pipelinesMap, ok := confmaputils.Get[confmaputils.ConfMap](conf, "service::pipelines")
	if !ok {
		return nil
	}
	for _, components := range pipelinesMap {
		componentsMap, ok := components.(map[string]any)
		if !ok {
			return nil
		}
		exporters, ok := componentsMap["exporters"]
		if !ok {
			continue
		}
		exportersSlice, ok := exporters.([]any)
		if !ok {
			return nil
		}
		for _, exporter := range exportersSlice {
			if exporterString, ok := exporter.(string); ok {
				if confmaputils.IsComponentType(exporterString, "datadog") {
					// datadog component is an exporter in this pipeline. Check if the prometheusReceiver is configured
					receivers, ok := componentsMap["receivers"]
					if !ok {
						continue
					}
					receiverSlice, ok := receivers.([]any)
					if !ok {
						return nil
					}
					for _, receiver := range receiverSlice {
						receiverString, ok := receiver.(string)
						if !ok {
							return nil
						}
						if receiverString == receiverName {
							ddExporters = append(ddExporters, exporterString)
						}
					}
				}
			}
		}
	}
	return ddExporters
}

func getDatadogExporters(conf confmaputils.ConfMap) map[string]any {
	datadogExporters := map[string]any{}
	exportersMap, ok := confmaputils.Get[confmaputils.ConfMap](conf, "exporters")
	if !ok {
		return datadogExporters
	}
	for exporterName, exporterConfig := range exportersMap {
		if confmaputils.IsComponentType(exporterName, "datadog") {
			datadogExporters[exporterName] = exporterConfig
		}
	}
	return datadogExporters
}

// findInternalMetricsAddress returns the address of internal prometheus server if configured
func findInternalMetricsAddress(conf confmaputils.ConfMap) string {
	internalMetricsAddress := "0.0.0.0:8888"
	mreadersSlice, ok := confmaputils.Get[[]any](conf, "service::telemetry::metrics::readers")
	if !ok {
		return internalMetricsAddress
	}
	for _, reader := range mreadersSlice {
		readerMap, ok := reader.(confmaputils.ConfMap)
		if !ok {
			continue
		}
		promExpMap, ok := confmaputils.Get[confmaputils.ConfMap](readerMap, "pull::exporter::prometheus")
		if !ok {
			continue
		}
		host := "0.0.0.0"
		port := 8888
		if h, ok := confmaputils.Get[string](promExpMap, "host"); ok {
			host = h
		}
		if p, ok := confmaputils.Get[int](promExpMap, "port"); ok {
			port = p
		}
		return hostport.Join(host, strconv.Itoa(port))
	}
	return internalMetricsAddress
}
