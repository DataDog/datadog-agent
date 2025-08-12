// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

// Package converterimpl provides the implementation of the otel-agent converter.
package converterimpl

import (
	"fmt"
	"strings"

	"go.opentelemetry.io/collector/confmap"
)

var (
	// prometheus
	prometheusName         = "prometheus"
	prometheusEnhancedName = prometheusName + "/" + ddAutoconfiguredSuffix
	prometheusConfig       = map[string]any{
		"config": map[string]any{
			"scrape_configs": []any{
				map[string]any{
					"fallback_scrape_protocol":      "PrometheusText0.0.4",
					"job_name":                      "datadog-agent",
					"metric_name_validation_scheme": "legacy",
					"metric_name_escaping_scheme":   "underscores",
					"scrape_interval":               "60s",
					"scrape_protocols":              []any{"PrometheusText0.0.4"},
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
func addPrometheusReceiver(conf *confmap.Conf, promServerAddr string) {
	mLevel := conf.Get("service::telemetry::metrics::level")
	if mLevel != nil && strings.ToLower(mLevel.(string)) == "none" {
		return
	}

	datadogExportersMap := getDatadogExporters(conf)

	stringMapConf := conf.ToStringMap()

	// find prometheus receivers which point to internal telemetry metrics. If present, check if it is defined
	// in pipeline with DD exporter. If so, remove from datadog exporter map.
	cfgsByRecv := findComps(stringMapConf, prometheusReceiver.Name, "receivers")
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
	*conf = *confmap.NewFromStringMap(stringMapConf)

	if len(datadogExportersMap) == 0 {
		return
	}

	comp := prometheusReceiver
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
	onlyStaticConfigMap["targets"] = []any{promServerAddr}

	addComponentToConfig(conf, comp)
	addDDExpToInternalPipeline(conf, comp, datadogExportersMap)
}

func addDDExpToInternalPipeline(conf *confmap.Conf, comp component, datadogExportersMap map[string]any) {
	for ddExporterName := range datadogExportersMap {
		pipelineName := "metrics" + "/" + ddAutoconfiguredSuffix + "/" + ddExporterName
		addComponentToPipeline(conf, comp, pipelineName)
		addComponentToPipeline(conf, component{
			Type:         "exporters",
			EnhancedName: ddExporterName,
		}, pipelineName)
	}
}

func receiverInPipelineWithDatadogExporter(conf *confmap.Conf, receiverName string) []string {
	var ddExporters []string
	stringMapConf := conf.ToStringMap()
	service, ok := stringMapConf["service"]
	if !ok {
		return nil
	}
	serviceMap, ok := service.(map[string]any)
	if !ok {
		return nil
	}
	pipelines, ok := serviceMap["pipelines"]
	if !ok {
		return nil
	}
	pipelinesMap, ok := pipelines.(map[string]any)
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
				if componentName(exporterString) == "datadog" {
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

// findInternalMetricsAddress returns the address of internal prometheus server if configured
func findInternalMetricsAddress(conf *confmap.Conf) string {
	internalMetricsAddress := "0.0.0.0:8888"
	mreaders := conf.Get("service::telemetry::metrics::readers")
	mreadersSlice, ok := mreaders.([]any)
	if !ok {
		return internalMetricsAddress
	}
	for _, reader := range mreadersSlice {
		readerMap, ok := reader.(map[string]any)
		if !ok {
			continue
		}
		pull, ok := readerMap["pull"]
		if !ok {
			continue
		}
		pullMap, ok := pull.(map[string]any)
		if !ok {
			continue
		}
		exp, ok := pullMap["exporter"]
		if !ok {
			continue
		}
		expMap, ok := exp.(map[string]any)
		if !ok {
			continue
		}
		promExp, ok := expMap["prometheus"]
		if !ok {
			continue
		}
		promExpMap, ok := promExp.(map[string]any)
		if !ok {
			continue
		}
		host := "0.0.0.0"
		port := 8888
		if h, ok := promExpMap["host"]; ok {
			host = h.(string)
		}
		if p, ok := promExpMap["port"]; ok {
			port = p.(int)
		}
		return fmt.Sprintf("%s:%d", host, port)
	}
	return internalMetricsAddress
}
