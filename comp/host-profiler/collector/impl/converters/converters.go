// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

//go:build linux

// Package converters implements the converters for the host profiler collector.
package converters

import (
	"context"
	"fmt"

	"github.com/DataDog/datadog-agent/comp/host-profiler/collector/impl/receiver"
	"go.opentelemetry.io/collector/confmap"
	"go.uber.org/zap"
)

// NewFactoryWithoutAgent returns a new converterWithoutAgent factory.
func NewFactoryWithoutAgent() confmap.ConverterFactory {
	return confmap.NewConverterFactory(newConverterWithoutAgent)
}

var _ confmap.Converter = (*converterWithoutAgent)(nil)

type converterWithoutAgent struct{}

func newConverterWithoutAgent(_ confmap.ConverterSettings) confmap.Converter {
	return &converterWithoutAgent{}
}

func (c *converterWithoutAgent) Convert(_ context.Context, conf *confmap.Conf) error {
	confStringMap := conf.ToStringMap()
	if err := removeInfraAttributesProcessor(confStringMap); err != nil {
		return err
	}
	if err := removeDDProfilingExtension(confStringMap); err != nil {
		return err
	}
	*conf = *confmap.NewFromStringMap(confStringMap)
	return nil

}

// NewFactoryWithAgent returns a new converterWithAgent factory.
func NewFactoryWithAgent() confmap.ConverterFactory {
	return confmap.NewConverterFactory(newConverterWithAgent)
}

var _ confmap.Converter = (*converterWithAgent)(nil)

type converterWithAgent struct {
	logger *zap.Logger
}

func newConverterWithAgent(settings confmap.ConverterSettings) confmap.Converter {
	return &converterWithAgent{
		logger: settings.Logger,
	}
}

func (c *converterWithAgent) Convert(_ context.Context, conf *confmap.Conf) error {
	confStringMap := conf.ToStringMap()

	if err := removeDDProfilingExtensionIfDisabled(confStringMap, c.logger); err != nil {
		return err
	}
	*conf = *confmap.NewFromStringMap(confStringMap)
	return nil

}

func removeDDProfilingExtensionIfDisabled(confStringMap map[string]any, logger *zap.Logger) error {
	hostprofilerMap, err := getMapStr(confStringMap, []string{"receivers", "hostprofiler"})
	if err != nil {
		return err
	}

	enableGoRuntimeProfiler := receiver.GetDefaultEnableGoRuntimeProfiler()

	if hostprofilerMap != nil {
		var ok bool
		value, ok := hostprofilerMap["enable_go_runtime_profiler"]
		if ok {
			enableGoRuntimeProfiler, ok = value.(bool)
			if !ok {
				return fmt.Errorf("enable_go_runtime_profiler is not a bool")
			}
		}
	}
	if !enableGoRuntimeProfiler {
		if err := removeDDProfilingExtension(confStringMap); err != nil {
			return err
		}
		logger.Info("DD Profiling is disabled")
	} else {
		logger.Info("DD Profiling is enabled")
	}
	return nil
}
