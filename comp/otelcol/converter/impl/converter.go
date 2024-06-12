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
	componentType         string
	componentName         string
	componentEnhancedName string
	componentConfig       any
}

var extensions = []component{
	{
		componentName:         pProfName,
		componentType:         "extensions",
		componentEnhancedName: pProfEnhancedName,
		componentConfig:       pProfConfig,
	},
	{
		componentName:         zpagesName,
		componentType:         "extensions",
		componentEnhancedName: zpagesEnhancedName,
		componentConfig:       zpagesConfig,
	},
	{
		componentName:         healthCheckName,
		componentType:         "extensions",
		componentEnhancedName: healthCheckEnhancedName,
		componentConfig:       healthCheckConfig,
	},
}

var processors = []component{
	{
		componentName:         infraAttributesName,
		componentType:         "processors",
		componentEnhancedName: infraAttributesEnhancedName,
		componentConfig:       infraAttributesConfig,
	},
}

var receivers = []component{
	{
		componentName:         prometheusName,
		componentType:         "receivers",
		componentEnhancedName: prometheusEnhancedName,
		componentConfig:       prometheusConfig,
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

func addPrometheusReceiver(conf *confmap.Conf, comp component) {
	datadogExportersMap := getDatadogExporters(conf)
	// get the address in which telemetry metrics are exposed
	internalMetricsAddress := conf.Get("service::telemetry::metrics::address")
	if internalMetricsAddress == nil {
		internalMetricsAddress = "0.0.0.0:8888"
	}

	stringMapConf := conf.ToStringMap()
	if receivers, ok := stringMapConf["receivers"]; ok {
		if receiversMap, ok := receivers.(map[string]any); ok {
			for receiver, receiverConfig := range receiversMap {
				if componentName(receiver) == comp.componentName {
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
																					// receiver is scraping from internal metrics. Now need to
																					// check if it's used in a pipeline with the DD exporter.
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
	if receivers, ok := stringMapConf["receivers"]; ok {
		if receiverConfigMap, ok := receivers.(map[string]any); ok {
			if componentMap, ok := comp.componentConfig.(map[string]any); ok {
				// potentially need to do in two steps
				if configMap, ok := componentMap["config"].(map[string]any); ok {
					if scrapeConfig, ok := configMap["scrape_configs"]; ok {
						if scrapeConfigSlice, ok := scrapeConfig.([]any); ok {
							onlyScrapeConfig := scrapeConfigSlice[0]
							if onlyScrapeConfigMap, ok := onlyScrapeConfig.(map[string]any); ok {
								// potentially need to do in two steps
								if staticConfigsSlice, ok := onlyScrapeConfigMap["static_configs"].([]any); ok {
									onlyStaticConfig := staticConfigsSlice[0]
									if onlyStaticConfigMap, ok := onlyStaticConfig.(map[string]any); ok {
										onlyStaticConfigMap["targets"] = []any{internalMetricsAddress}
									}
								}
							}
						}
					}
				}
				// ["scrape_configs"][0]["static_configs"][0]["targets"] = internalMetricsAddress
			}
			receiverConfigMap[comp.componentEnhancedName] = comp.componentConfig
		}
	}

	if service, ok := stringMapConf["service"]; ok {
		if serviceMap, ok := service.(map[string]any); ok {
			if pipelines, ok := serviceMap["pipelines"]; ok {
				if pipelinesMap, ok := pipelines.(map[string]any); ok {
					for ddExporterName, _ := range datadogExportersMap {
						pipelineName := "metrics" + "/" + ddEnhancedSuffix + "/" + ddExporterName
						pipelinesMap[pipelineName] = map[string]any{
							"receivers": []any{comp.componentEnhancedName},
							"exporters": []any{ddExporterName},
						}
					}
				}
			}
		}
	}
	*conf = *confmap.NewFromStringMap(stringMapConf)
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
				if componentName(componentString) == comp.componentName {
					return true
				}
			}
		}
	}
	return false
}

func addComponentToConfig(conf *confmap.Conf, comp component) {
	stringMapConf := conf.ToStringMap()

	if components, ok := stringMapConf[comp.componentType]; ok {
		if componentMap, ok := components.(map[string]any); ok {
			componentMap[comp.componentEnhancedName] = comp.componentConfig
		}
	} else {
		stringMapConf[comp.componentType] = map[string]any{
			comp.componentEnhancedName: comp.componentConfig,
		}
	}
	*conf = *confmap.NewFromStringMap(stringMapConf)
}

func addExtensionToPipeline(conf *confmap.Conf, comp component) {
	stringMapConf := conf.ToStringMap()
	if service, ok := stringMapConf["service"]; ok {
		if serviceMap, ok := service.(map[string]any); ok {
			if components, ok := serviceMap[comp.componentType]; ok {
				if componentsSlice, ok := components.([]any); ok {
					componentsSlice = append(componentsSlice, comp.componentEnhancedName)
					serviceMap[comp.componentType] = componentsSlice
				}
			} else {
				serviceMap[comp.componentType] = []any{comp.componentEnhancedName}
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
					for _, components := range pipelinesMap {
						if componentsMap, ok := components.(map[string]any); ok {
							if exporters, ok := componentsMap["exporters"]; ok {
								if exportersSlice, ok := exporters.([]any); ok {
									for _, exporter := range exportersSlice {
										if exporterString, ok := exporter.(string); ok {

											if componentName(exporterString) == "datadog" {
												// datadog component is an exporter in this pipeline. Need to make sure that processor is also configured.
												_, ok := componentsMap[comp.componentType]
												if !ok {
													componentsMap[comp.componentType] = []any{}
												}

												infraAttrsInPipeline := false
												if processorsSlice, ok := componentsMap[comp.componentType].([]any); ok {
													for _, processor := range processorsSlice {
														if processorString, ok := processor.(string); ok {
															if componentName(processorString) == comp.componentName {
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
														processorsSlice = append(processorsSlice, comp.componentEnhancedName)
														componentsMap[comp.componentType] = processorsSlice
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
	conf.Merge(confmap.NewFromStringMap(stringMapConf))
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
