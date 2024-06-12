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
			"scrape_configs": []map[string]any{
				{
					"job_name":        "otelcol",
					"scrape_interval": "10s",
					"static_configs": []map[string]any{
						{
							"targets": []string{"0.0.0.0:8888"},
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
		if inServicePipeline(conf, component) {
			continue
		}
		addComponentToConfig(conf, component)
		addExtensionToPipeline(conf, component)
	}

	// add processors in pipelines with DD Exporter if missing
	for _, component := range processors {
		addProcessorToPipelinesWithDDExporter(conf, component)
	}
}

func inServicePipeline(conf *confmap.Conf, comp component) bool {
	pipelineComponents := conf.Get("service::" + comp.componentType)
	if pipelineComponents == nil {
		return false
	}

	if componentSlice, ok := pipelineComponents.([]any); ok {
		for _, component := range componentSlice {
			if componentString, ok := component.(string); ok {
				parts := strings.SplitN(componentString, "/", 2)
				if parts[0] == comp.componentName {
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
	var infraAttrsConfAddedToConfig bool
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
											parts := strings.SplitN(exporterString, "/", 2)
											if parts[0] == "datadog" {
												// datadog component is an exporter in this pipeline. Need to make sure that infraattributes processor is also configured.
												_, ok := componentsMap["processors"]
												if !ok {
													componentsMap["processors"] = []any{}
												}

												infraAttrsInPipeline := false
												if processorsSlice, ok := componentsMap["processors"].([]any); ok {
													for _, processor := range processorsSlice {
														if processorString, ok := processor.(string); ok {
															parts := strings.SplitN(processorString, "/", 2)
															if parts[0] == "infraattributes" {
																infraAttrsInPipeline = true
															}
														}
													}
													if !infraAttrsInPipeline {
														// no processors are defined
														if !infraAttrsConfAddedToConfig {
															addComponentToConfig(conf, comp)
															infraAttrsConfAddedToConfig = true
														}
														processorsSlice = append(processorsSlice, comp.componentEnhancedName)
														componentsMap["processors"] = processorsSlice
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
