// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

//go:build linux

// Package converters implements the converters for the host profiler collector.
package converters

import (
	"context"

	"github.com/DataDog/datadog-agent/comp/core/config"
	"github.com/DataDog/datadog-agent/pkg/util/log"
	"go.opentelemetry.io/collector/confmap"
)

// NewFactoryWithoutAgent returns a new converterWithoutAgent factory.
func NewFactoryWithoutAgent() confmap.ConverterFactory {
	return confmap.NewConverterFactory(newConverterWithoutAgent)
}

// NewFactoryWithAgent returns a new converterWithAgent factory.
func NewFactoryWithAgent(config config.Component) confmap.ConverterFactory {
	newConverterWithAgentWrapper := func(settings confmap.ConverterSettings) confmap.Converter {
		return newConverterWithAgent(settings, config)
	}

	return confmap.NewConverterFactory(newConverterWithAgentWrapper)
}

type converterWithoutAgent struct{}

func newConverterWithoutAgent(_ confmap.ConverterSettings) confmap.Converter {
	return &converterWithoutAgent{}
}

type converterWithAgent struct {
	config config.Component
}

func newConverterWithAgent(_ confmap.ConverterSettings, config config.Component) confmap.Converter {
	return &converterWithAgent{config: config}
}

func (c *converterWithAgent) ensureKey(configMap map[string]any, key string, agentKeyName string) {
	if varMap, ok := configMap[key]; !ok || varMap == nil {
		configMap[key] = c.config.GetString(agentKeyName)
		log.Debugf("Added %s to %s", agentKeyName, key)
	} else {
		log.Debugf("%s already present", key)
	}
}

// checkAPIKeys validates that all required API keys are configured
func (c *converterWithAgent) checkAPIKeys(confStringMap map[string]any) error {
	log.Debug("Checking API/APP keys in hostprofiler")
	symbolEndpoints, err := getSymbolEndpoints(confStringMap)
	if err != nil {
		return err
	}


	for _, endpoint := range symbolEndpoints {
		c.ensureKey(endpoint.(map[string]any), "api_key", "api_key")
		c.ensureKey(endpoint.(map[string]any), "app_key", "app_key")
	}

	exporterHeaders, err := getExporterHeaders(confStringMap)
	if err != nil {
		return err
	}

	// Check exporter API key
	if exporterHeaders != nil {
		log.Debug("Checking API key in otlphttpexporter")
		c.ensureKey(exporterHeaders, "dd-api-key", "api_key")
	}

	return nil
}

func (c *converterWithAgent) Convert(_ context.Context, conf *confmap.Conf) error {
	confStringMap := conf.ToStringMap()
	if err := removeResourceDetectionProcessor(confStringMap); err != nil {
		return err
	}

	if err := c.checkAPIKeys(confStringMap); err != nil {
		return err
	}

	*conf = *confmap.NewFromStringMap(confStringMap)
	return nil
}

func (c *converterWithoutAgent) Convert(_ context.Context, conf *confmap.Conf) error {
	confStringMap := conf.ToStringMap()
	if err := removeInfraAttributesProcessor(confStringMap); err != nil {
		return err
	}
	if err := removeDDProfilingExtension(confStringMap); err != nil {
		return err
	}
	if err := removeHpFlareExtension(confStringMap); err != nil {
		return err
	}

	*conf = *confmap.NewFromStringMap(confStringMap)
	return nil

}
