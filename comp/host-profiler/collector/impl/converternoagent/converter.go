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

func newConverter(_ confmap.ConverterSettings) confmap.Converter {
	return &converterNoAgent{}
}

func (c *converterNoAgent) Convert(_ context.Context, conf *confmap.Conf) error {
	return removeInfraAttributesProcessor(conf)
}

func infraAttributesName() string {
	return "infraattributes/default"
}

func removeInfraAttributesProcessor(conf *confmap.Conf) error {
	conf.Delete("processors::" + infraAttributesName())
	confStringMap := conf.ToStringMap()
	if err := removeFromList(confStringMap, []string{"service", "pipelines", "profiles"}, "processors", infraAttributesName()); err != nil {
		return err
	}
	*conf = *confmap.NewFromStringMap(confStringMap)
	return nil
}

func removeFromList(confStringMap map[string]any, parentNames []string, listName string, itemToRemove string) error {
	parentMap, err := getMapStr(confStringMap, parentNames)
	if err != nil {
		return err
	}

	if parentMap != nil {
		children, ok := parentMap[listName].([]any)
		if !ok {
			return nil
		}
		children = slices.DeleteFunc(children, func(item any) bool {
			str, ok := item.(string)
			if !ok {
				return false
			}
			return str == itemToRemove
		})
		parentMap[listName] = children
	}
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
