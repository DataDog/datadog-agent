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
	prometheusEnhancedName = prometheusName + "/" + ddEnhancedSuffix
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

// addPrometheusReceiver ensures that each datadogexporter is configured with a prometheus collector
// which points to the collectors internal telemetry metrics. In cases where this is not true, it adds
// a pipeline with the prometheus exporter and datadog exporter.
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
		if receiversMap, ok := receivers.(map[string]any); ok {
			for receiver, receiverConfig := range receiversMap {
				if componentName(receiver) == comp.Name {
					if receiverConfigMap, ok := receiverConfig.(map[string]any); ok {
						if prometheusConfig, ok := receiverConfigMap["config"]; ok {
							if prometheusConfigMap, ok := prometheusConfig.(map[string]any); ok {
								if prometheusScrapeConfigs, ok := prometheusConfigMap["scrape_configs"]; ok {
									if prometheusScrapeConfigsSlice, ok := prometheusScrapeConfigs.([]any); ok {
										for _, scrapeConfig := range prometheusScrapeConfigsSlice {
											if scrapeConfigMap, ok := scrapeConfig.(map[string]any); ok {
												if staticConfig, ok := scrapeConfigMap["static_configs"]; ok {
													if staticConfigSlice, ok := staticConfig.([]any); ok {
														for _, staticConfig := range staticConfigSlice {
															if staticConfigMap, ok := staticConfig.(map[string]any); ok {
																if targets, ok := staticConfigMap["targets"]; ok {
																	if targetsSlice, ok := targets.([]any); ok {
																		for _, target := range targetsSlice {
																			if targetString, ok := target.(string); ok {
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
													}
												}
											}
										}
									}
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

	_, ok := stringMapConf["receivers"]
	if !ok {
		stringMapConf["receivers"] = map[string]any{}
	}

	// update default prometheus config based on service telemetry address.
	if prometheusConfigMap, ok := comp.Config.(map[string]any); ok {
		if config, ok := prometheusConfigMap["config"]; ok {
			if configMap, ok := config.(map[string]any); ok {
				if scrapeConfig, ok := configMap["scrape_configs"]; ok {
					if scrapeConfigSlice, ok := scrapeConfig.([]any); ok {
						onlyScrapeConfig := scrapeConfigSlice[0]
						if onlyScrapeConfigMap, ok := onlyScrapeConfig.(map[string]any); ok {
							if staticConfigs, ok := onlyScrapeConfigMap["static_configs"]; ok {
								if staticConfigsSlice, ok := staticConfigs.([]any); ok {
									onlyStaticConfig := staticConfigsSlice[0]
									if onlyStaticConfigMap, ok := onlyStaticConfig.(map[string]any); ok {
										onlyStaticConfigMap["targets"] = []any{internalMetricsAddress}
									}
								}
							}
						}
					}
				}
			}
		}
	}
	addComponentToConfig(conf, comp)

	for ddExporterName, _ := range datadogExportersMap {
		pipelineName := "metrics" + "/" + ddEnhancedSuffix + "/" + ddExporterName
		addComponentToPipeline(conf, comp, pipelineName)
		addComponentToPipeline(conf, component{
			Type:         "exporters",
			EnhancedName: ddExporterName,
		}, pipelineName)
	}
}

func receiverInPipelineWithDatadogExporter(conf *confmap.Conf, receiverName string) string {
	stringMapConf := conf.ToStringMap()
	if service, ok := stringMapConf["service"]; ok {
		if serviceMap, ok := service.(map[string]any); ok {
			if pipelines, ok := serviceMap["pipelines"]; ok {
				if pipelinesMap, ok := pipelines.(map[string]any); ok {
					for _, components := range pipelinesMap {
						if componentsMap, ok := components.(map[string]any); ok {
							if exporters, ok := componentsMap["exporters"]; ok {
								if exportersSlice, ok := exporters.([]any); ok {
									for _, exporter := range exportersSlice {
										if exporterString, ok := exporter.(string); ok {
											if componentName(exporterString) == "datadog" {
												// datadog component is an exporter in this pipeline. Check if the prometheusReceiver is configured
												receivers, ok := componentsMap["receivers"]
												if !ok {
													continue
												}
												if receiverSlice, ok := receivers.([]any); ok {
													for _, receiver := range receiverSlice {
														if receiverString, ok := receiver.(string); ok {
															if receiverString == receiverName {
																return exporterString
															}
														}
													}

												}
											}
										}
									}
								}
							}
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
	if exporters, ok := stringMapConf["exporters"]; ok {
		if exportersMap, ok := exporters.(map[string]any); ok {
			for exporterName, exporterConfig := range exportersMap {
				if componentName(exporterName) == "datadog" {
					datadogExporters[exporterName] = exporterConfig
				}
			}
		}
	}
	return datadogExporters
}
