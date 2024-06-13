// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

// Package converterimpl provides the implementation of the otel-agent converter.
package converterimpl

import (
	"context"
	"strings"

	converter "github.com/DataDog/datadog-agent/comp/otelcol/converter/def"
	"go.opentelemetry.io/collector/confmap"
	"gopkg.in/yaml.v3"
)

type ddConverter struct {
	confDump confDump
}

var (
	_ confmap.Converter = (*ddConverter)(nil)

	ddEnhancedSuffix = "dd-enhanced"
	// pprof
	pProfName         = "pprof"
	pProfEnhancedName = pProfName + "/" + ddEnhancedSuffix
	pProfConfig       any

	// zpages
	zpagesName         = "zpages"
	zpagesEnhancedName = zpagesName + "/" + ddEnhancedSuffix
	zpagesConfig       = map[string]any{
		"endpoint": "localhost:55679",
	}

	// healthcheck
	healthCheckName         = "health_check"
	healthCheckEnhancedName = healthCheckName + "/" + ddEnhancedSuffix
	healthCheckConfig       any

	// infraattributes
	infraAttributesName         = "infraattributes"
	infraAttributesEnhancedName = infraAttributesName + "/" + ddEnhancedSuffix
	infraAttributesConfig       any

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
)

type confDump struct {
	provided string
	enhanced string
}

type component struct {
	Type         string
	Name         string
	EnhancedName string
	// Signal corresponds to which signal the component corresponds to. If left
	// empty, means all signals.
	Signal string
	Config any
}

var extensions = []component{
	{
		Name:         pProfName,
		EnhancedName: pProfEnhancedName,
		Type:         "extensions",
		Config:       pProfConfig,
	},
	{
		Name:         zpagesName,
		EnhancedName: zpagesEnhancedName,
		Type:         "extensions",
		Config:       zpagesConfig,
	},
	{
		Name:         healthCheckName,
		EnhancedName: healthCheckEnhancedName,
		Type:         "extensions",
		Config:       healthCheckConfig,
	},
}

var processors = []component{
	{
		Name:         infraAttributesName,
		EnhancedName: infraAttributesEnhancedName,
		Type:         "processors",
		Config:       infraAttributesConfig,
	},
}

var receivers = []component{
	{
		Name:         prometheusName,
		EnhancedName: prometheusEnhancedName,
		Type:         "receivers",
		Config:       prometheusConfig,
		Signal:       "metrics",
	},
}

// NewConverter currently only supports a single URI in the uris slice, and this URI needs to be a file path.
func NewConverter() (converter.Component, error) {
	return &ddConverter{
		confDump: confDump{
			provided: "not supported",
			enhanced: "not supported",
		},
	}, nil
}

func (c *ddConverter) Convert(ctx context.Context, conf *confmap.Conf) error {
	// c.addProvidedConf(conf)

	enhanceConfig(conf)

	// c.addEnhancedConf(conf)
	return nil
}

func enhanceConfig(conf *confmap.Conf) {
	// add extensions if missing
	for _, component := range extensions {
		if ExtensionIsInServicePipeline(conf, component) {
			continue
		}
		addComponentToConfig(conf, component)
		addExtensionToPipeline(conf, component)
	}

	// add processors in pipelines with DD Exporter if missing
	for _, component := range processors {
		addProcessorToPipelinesWithDDExporter(conf, component)
	}

	// add receivers in pipelines
	addPrometheusReceiver(conf, receivers[0])
}

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

func componentName(fullName string) string {
	parts := strings.SplitN(fullName, "/", 2)
	return parts[0]
}

func ExtensionIsInServicePipeline(conf *confmap.Conf, comp component) bool {
	pipelineComponents := conf.Get("service::extensions")
	if pipelineComponents == nil {
		return false
	}

	if componentSlice, ok := pipelineComponents.([]any); ok {
		for _, component := range componentSlice {
			if componentString, ok := component.(string); ok {
				if componentName(componentString) == comp.Name {
					return true
				}
			}
		}
	}
	return false
}

func addComponentToConfig(conf *confmap.Conf, comp component) {
	stringMapConf := conf.ToStringMap()

	if components, ok := stringMapConf[comp.Type]; ok {
		if componentMap, ok := components.(map[string]any); ok {
			componentMap[comp.EnhancedName] = comp.Config
		}
	} else {
		stringMapConf[comp.Type] = map[string]any{
			comp.EnhancedName: comp.Config,
		}
	}
	*conf = *confmap.NewFromStringMap(stringMapConf)
}

// addComponentToPipeline adds comp into pipelineName. If pipelineName does not exist,
// it creates it. It only supports receivers, processors and exporters.
func addComponentToPipeline(conf *confmap.Conf, comp component, pipelineName string) {
	stringMapConf := conf.ToStringMap()
	if service, ok := stringMapConf["service"]; ok {
		if serviceMap, ok := service.(map[string]any); ok {
			if pipelines, ok := serviceMap["pipelines"]; ok {
				if pipelinesMap, ok := pipelines.(map[string]any); ok {
					_, ok := pipelinesMap[pipelineName]
					if !ok {
						// create pipeline
						pipelinesMap[pipelineName] = map[string]any{}
					}
					if pipeline, ok := pipelinesMap[pipelineName].(map[string]any); ok {
						_, ok := pipeline[comp.Type]
						if !ok {
							pipeline[comp.Type] = []any{}
						}
						if pipelineTypeSlice, ok := pipeline[comp.Type].([]any); ok {
							pipelineTypeSlice = append(pipelineTypeSlice, comp.EnhancedName)
							pipeline[comp.Type] = pipelineTypeSlice
						}
					}
				}
			}
		}
	}
	*conf = *confmap.NewFromStringMap(stringMapConf)
}

func addExtensionToPipeline(conf *confmap.Conf, comp component) {
	stringMapConf := conf.ToStringMap()
	if service, ok := stringMapConf["service"]; ok {
		if serviceMap, ok := service.(map[string]any); ok {
			if components, ok := serviceMap[comp.Type]; ok {
				if componentsSlice, ok := components.([]any); ok {
					componentsSlice = append(componentsSlice, comp.EnhancedName)
					serviceMap[comp.Type] = componentsSlice
				}
			} else {
				serviceMap[comp.Type] = []any{comp.EnhancedName}
			}
		}
	}
	*conf = *confmap.NewFromStringMap(stringMapConf)
}

func addProcessorToPipelinesWithDDExporter(conf *confmap.Conf, comp component) {
	var componentAddedToConfig bool
	stringMapConf := conf.ToStringMap()
	if service, ok := stringMapConf["service"]; ok {
		if serviceMap, ok := service.(map[string]any); ok {
			if pipelines, ok := serviceMap["pipelines"]; ok {
				if pipelinesMap, ok := pipelines.(map[string]any); ok {
					for pipelineName, components := range pipelinesMap {
						if componentsMap, ok := components.(map[string]any); ok {
							if exporters, ok := componentsMap["exporters"]; ok {
								if exportersSlice, ok := exporters.([]any); ok {
									for _, exporter := range exportersSlice {
										if exporterString, ok := exporter.(string); ok {
											if componentName(exporterString) == "datadog" {
												// datadog component is an exporter in this pipeline. Need to make sure that processor is also configured.
												_, ok := componentsMap[comp.Type]
												if !ok {
													componentsMap[comp.Type] = []any{}
												}

												infraAttrsInPipeline := false
												if processorsSlice, ok := componentsMap[comp.Type].([]any); ok {
													for _, processor := range processorsSlice {
														if processorString, ok := processor.(string); ok {
															if componentName(processorString) == comp.Name {
																infraAttrsInPipeline = true
															}
														}
													}
													if !infraAttrsInPipeline {
														// no processors are defined
														if !componentAddedToConfig {
															addComponentToConfig(conf, comp)
															componentAddedToConfig = true
														}
														addComponentToPipeline(conf, comp, pipelineName)
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

// nolint: deadcode, unused
func (c *ddConverter) addProvidedConf(conf *confmap.Conf) error {
	bytesConf, err := confToString(conf)
	if err != nil {
		return err
	}

	c.confDump.provided = bytesConf
	return nil
}

// nolint: deadcode, unused
func (c *ddConverter) addEnhancedConf(conf *confmap.Conf) error {
	bytesConf, err := confToString(conf)
	if err != nil {
		return err
	}

	c.confDump.enhanced = bytesConf
	return nil
}

// GetProvidedConf returns a string representing the collector configuration passed
// by the user.
// Note: this is currently not supported.
func (c *ddConverter) GetProvidedConf() string {
	return c.confDump.provided
}

// GetEnhancedConf returns a string representing the enhanced collector configuration.
// Note: this is currently not supported.
func (c *ddConverter) GetEnhancedConf() string {
	return c.confDump.enhanced
}

// confToString takes in an *confmap.Conf and returns a string with the yaml
// representation. It takes advantage of the confmaps opaquevalue to redact any
// sensitive fields.
// Note: Currently not supported until the following upstream PR:
// https://github.com/open-telemetry/opentelemetry-collector/pull/10139 is merged.
// nolint: deadcode, unused
func confToString(conf *confmap.Conf) (string, error) {
	bytesConf, err := yaml.Marshal(conf.ToStringMap())
	if err != nil {
		return "", err
	}

	return string(bytesConf), nil
}
