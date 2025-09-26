// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

//go:build linux

// Package converternoagent implements the converterNoAgent component interface when the Agent Core is not available.
package converternoagent

import (
	"context"
	"fmt"
	"slices"

	"go.opentelemetry.io/collector/confmap"
)

// NewFactory returns a new converterNoAgent factory.
func NewFactory() confmap.ConverterFactory {
	return confmap.NewConverterFactory(newConverter)
}

type converterNoAgent struct{}

func newConverter(set confmap.ConverterSettings) confmap.Converter {
	return &converterNoAgent{}
}

func (c *converterNoAgent) Convert(_ context.Context, conf *confmap.Conf) error {
	return removeInfraAttributesProcessor(conf)
}

func removeInfraAttributesProcessor(conf *confmap.Conf) error {
	conf.Delete("processors::infraattributes/default")
	confStringMap := conf.ToStringMap()
	profilesMap, err := getMapStr(confStringMap, []string{"service", "pipelines", "profiles"})
	if err != nil {
		return err
	}

	if profilesMap != nil {
		processors, ok := profilesMap["processors"].([]any)
		if !ok {
			return nil
		}
		processors = slices.DeleteFunc(processors, func(item any) bool {
			str, ok := item.(string)
			if !ok {
				return false
			}
			return str == "infraattributes/default"
		})
		profilesMap["processors"] = processors
	}
	*conf = *confmap.NewFromStringMap(confStringMap)
	return nil
}

func getMapStr(confStringMap map[string]any, keys []string) (map[string]any, error) {
	for _, key := range keys {
		value, ok := confStringMap[key]
		if !ok {
			return nil, nil
		}
		confStringMap, ok = value.(map[string]any)
		if !ok {
			return nil, fmt.Errorf("value is not a map[string]any:%v", value)
		}
	}
	return confStringMap, nil
}
