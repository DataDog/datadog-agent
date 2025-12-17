// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

//go:build linux

// Package converters implements the converters for the host profiler collector.
package converters

import (
	"context"

	"github.com/DataDog/datadog-agent/pkg/util/log"
	"go.opentelemetry.io/collector/confmap"
)

type yamlNode = map[string]any

// NewFactoryWithoutAgent returns a new converterWithoutAgent factory.
func NewFactoryWithoutAgent() confmap.ConverterFactory {
	return confmap.NewConverterFactory(newConverterWithoutAgent)
}

// NewFactoryWithAgent returns a new converterWithAgent factory.
func NewFactoryWithAgent() confmap.ConverterFactory {
	return confmap.NewConverterFactory(newConverterWithAgent)
}

type converterWithoutAgent struct{}

func newConverterWithoutAgent(_ confmap.ConverterSettings) confmap.Converter {
	return &converterWithoutAgent{}
}

type converterWithAgent struct{}

func newConverterWithAgent(_ confmap.ConverterSettings) confmap.Converter {
	return &converterWithAgent{}
}

func (c *converterWithAgent) Convert(_ context.Context, conf *confmap.Conf) error {
	confStringMap := conf.ToStringMap()
	if err := removeResourceDetectionProcessor(confStringMap); err != nil {
		return err
	}

	if mergeMap(confStringMap, agentModeRequiredConfig()) {
		log.Info("Applied required infraattributes configuration")
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

	if mergeMap(confStringMap, standaloneModeRequiredConfig()) {
		log.Info("Applied required resourcedetection configuration")
	}

	*conf = *confmap.NewFromStringMap(confStringMap)
	return nil

}
