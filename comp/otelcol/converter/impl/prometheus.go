// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

// Package converterimpl provides the implementation of the otel-agent converter.
package converterimpl

import "go.opentelemetry.io/collector/confmap"

var (
	// prometheus
	prometheusName         = "prometheus"
	prometheusEnhancedName = prometheusName + "/" + ddAutoconfiguredSuffix
	prometheusConfig       = map[string]any{
		"config": map[string]any{
			"scrape_configs": []any{
				map[string]any{
					"job_name":        "otelcol",
					"scrape_interval": "10s",
					"static_configs": []any{
						map[string]any{
							"targets": []any{"0.0.0.0:8888"},
						},
					},
				},
			},
		},
	}

	// component
	prometheusReceiver = component{
		Name:         prometheusName,
		EnhancedName: prometheusEnhancedName,
		Type:         "receivers",
		Config:       prometheusConfig,
	}
)

// addPrometheusReceiver ensures that each datadogexporter is configured with a prometheus receiver
// which points to the collectors internal telemetry metrics. In cases where this is not true, it adds
// a pipeline with the prometheus exporter and datadog exporter.
// todo(mackjmr): in the case where there are two datadog exporters with the same API key, we may not
// want to configure two pipelines with the prometheus receiver for each exporter, as this will lead
// to shipping the health metrics twice to the same org. If there are two datadog exporters with
// different API keys, there is no way to know if these are from the same org, so there is a risk
// of double shipping.
func addPrometheusReceiver(conf *confmap.Conf, comp component) {
	datadogExportersMap := getDatadogExporters(conf)
	internalMetricsAddress := conf.Get("service::telemetry::metrics::address")
	if internalMetricsAddress == nil {
		internalMetricsAddress = "0.0.0.0:8888"
	}
	stringMapConf := conf.ToStringMap()

	// find prometheus receivers which point to internal telemetry metrics. If present, check if it is defined
	// in pipeline with DD exporter. If so, remove from datadog exporter map.
	if receivers, ok := stringMapConf["receivers"]; ok {
		receiversMap, ok := receivers.(map[string]any)
		if !ok {
			return
		}
		for receiver, receiverConfig := range receiversMap {
			if componentName(receiver) == comp.Name {
				receiverConfigMap, ok := receiverConfig.(map[string]any)
				if !ok {
					return
				}
				prometheusConfig, ok := receiverConfigMap["config"]
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
						return
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
							if targetString == internalMetricsAddress {
								if ddExporter := receiverInPipelineWithDatadogExporter(conf, receiver); ddExporter != "" {
									delete(datadogExportersMap, ddExporter)
								}
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

	// update default prometheus config based on service telemetry address.
	prometheusConfigMap, ok := comp.Config.(map[string]any)
	if !ok {
		return
	}
	config, ok := prometheusConfigMap["config"]
	if !ok {
		return
	}
	configMap, ok := config.(map[string]any)
	if !ok {
		return
	}
	scrapeConfig, ok := configMap["scrape_configs"]
	if !ok {
		return
	}
	scrapeConfigSlice, ok := scrapeConfig.([]any)
	if !ok {
		return
	}
	onlyScrapeConfig := scrapeConfigSlice[0]
	onlyScrapeConfigMap, ok := onlyScrapeConfig.(map[string]any)
	if !ok {
		return
	}
	staticConfigs, ok := onlyScrapeConfigMap["static_configs"]
	if !ok {
		return
	}
	staticConfigsSlice, ok := staticConfigs.([]any)
	if !ok {
		return
	}
	onlyStaticConfig := staticConfigsSlice[0]
	onlyStaticConfigMap, ok := onlyStaticConfig.(map[string]any)
	if !ok {
		return
	}
	onlyStaticConfigMap["targets"] = []any{internalMetricsAddress}

	addComponentToConfig(conf, comp)

	for ddExporterName := range datadogExportersMap {
		pipelineName := "metrics" + "/" + ddAutoconfiguredSuffix + "/" + ddExporterName
		addComponentToPipeline(conf, comp, pipelineName)
		addComponentToPipeline(conf, component{
			Type:         "exporters",
			EnhancedName: ddExporterName,
		}, pipelineName)
	}
}

func receiverInPipelineWithDatadogExporter(conf *confmap.Conf, receiverName string) string {
	stringMapConf := conf.ToStringMap()
	service, ok := stringMapConf["service"]
	if !ok {
		return ""
	}
	serviceMap, ok := service.(map[string]any)
	if !ok {
		return ""
	}
	pipelines, ok := serviceMap["pipelines"]
	if !ok {
		return ""
	}
	pipelinesMap, ok := pipelines.(map[string]any)
	if !ok {
		return ""
	}
	for _, components := range pipelinesMap {
		componentsMap, ok := components.(map[string]any)
		if !ok {
			return ""
		}
		exporters, ok := componentsMap["exporters"]
		if !ok {
			continue
		}
		exportersSlice, ok := exporters.([]any)
		if !ok {
			return ""
		}
		for _, exporter := range exportersSlice {
			if exporterString, ok := exporter.(string); ok {
				if componentName(exporterString) == "datadog" {
					// datadog component is an exporter in this pipeline. Check if the prometheusReceiver is configured
					receivers, ok := componentsMap["receivers"]
					if !ok {
						continue
					}
					receiverSlice, ok := receivers.([]any)
					if !ok {
						return ""
					}
					for _, receiver := range receiverSlice {
						receiverString, ok := receiver.(string)
						if !ok {
							return ""
						}
						if receiverString == receiverName {
							return exporterString
						}

					}

				}
			}
		}

	}

	return ""
}

func getDatadogExporters(conf *confmap.Conf) map[string]any {
	datadogExporters := map[string]any{}
	stringMapConf := conf.ToStringMap()
	exporters, ok := stringMapConf["exporters"]
	if !ok {
		return datadogExporters
	}
	exportersMap, ok := exporters.(map[string]any)
	if !ok {
		return datadogExporters
	}
	for exporterName, exporterConfig := range exportersMap {
		if componentName(exporterName) == "datadog" {
			datadogExporters[exporterName] = exporterConfig
		}
	}

	return datadogExporters
}
