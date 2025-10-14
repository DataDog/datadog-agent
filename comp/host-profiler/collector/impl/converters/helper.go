// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

//go:build linux

// Package converters implements the converters for the host profiler collector.
package converters

import (
	"fmt"
	"slices"
)

func removeInfraAttributesProcessor(confStringMap map[string]any) error {
	if err := removeFromMap(confStringMap, []string{"processors"}, infraAttributesName()); err != nil {
		return err
	}

	return removeFromList(confStringMap, []string{"service", "pipelines", "profiles"}, "processors", infraAttributesName())
}

func removeDDProfilingExtension(confStringMap map[string]any) error {
	if err := removeFromMap(confStringMap, []string{"extensions"}, ddprofilingName()); err != nil {
		return err
	}

	return removeFromList(confStringMap, []string{"service"}, "extensions", ddprofilingName())
}

func infraAttributesName() string {
	return "infraattributes/default"
}

func ddprofilingName() string {
	return "ddprofiling/default"
}

func removeFromMap(confStringMap map[string]any, parentNames []string, mapName string) error {
	parentMap, err := getMapStr(confStringMap, parentNames)
	if err != nil {
		return err
	}
	if parentMap != nil {
		delete(parentMap, mapName)
	}
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

		// When having hostprofiler: {} in the config, the value is nil
		if !ok || value == nil {
			return nil, nil
		}
		confStringMap, ok = value.(map[string]any)
		if !ok {
			return nil, fmt.Errorf("value is type %T and not a map[string]any:%v", value, value)
		}
	}
	return confStringMap, nil
}
