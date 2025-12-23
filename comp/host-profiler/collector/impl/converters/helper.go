// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

//go:build linux

// Package converters implements the converters for the host profiler collector.
package converters

import (
	"fmt"
	"github.com/DataDog/datadog-agent/pkg/util/log"
	"slices"
)

func addToProcessors(processor string) yamlNode {
	return yamlNode{
		"pipelines": yamlNode{
			"profiles": yamlNode{
				"processors": []any{processor},
			},
		}}
}

func agentModeRequiredConfig() yamlNode {
	return yamlNode{
		"processors": yamlNode{
			infraAttributesName(): yamlNode{
				"allow_hostname_override": true,
			},
		},
		"service": addToProcessors(infraAttributesName()),
	}
}

func standaloneModeRequiredConfig() yamlNode {
	return yamlNode{
		"processors": yamlNode{
			resourceDetectionName(): yamlNode{
				"detectors": []any{"system"},
				"system": yamlNode{
					"resource_attributes": yamlNode{
						"host.arch": yamlNode{
							"enabled": true,
						},
						"host.name": yamlNode{
							"enabled": false,
						},
						"os.type": yamlNode{
							"enabled": false,
						},
					},
				},
			},
		},
		"service": addToProcessors(resourceDetectionName()),
	}
}

func removeInfraAttributesProcessor(confStringMap map[string]any) error {
	if err := removeFromMap(confStringMap, []string{"processors"}, infraAttributesName()); err != nil {
		return err
	}

	err := removeFromList(confStringMap, []string{"service", "pipelines", "profiles"}, "processors", infraAttributesName())
	if err != nil {
		return err
	}
	return removeFromList(confStringMap, []string{"service", "pipelines", "metrics"}, "processors", infraAttributesName())
}

func removeResourceDetectionProcessor(confStringMap map[string]any) error {
	if err := removeFromMap(confStringMap, []string{"processors"}, resourceDetectionName()); err != nil {
		return err
	}

	err := removeFromList(confStringMap, []string{"service", "pipelines", "profiles"}, "processors", resourceDetectionName())
	if err != nil {
		return err
	}
	return removeFromList(confStringMap, []string{"service", "pipelines", "metrics"}, "processors", resourceDetectionName())
}

func removeDDProfilingExtension(confStringMap map[string]any) error {
	if err := removeFromMap(confStringMap, []string{"extensions"}, ddprofilingName()); err != nil {
		return err
	}

	return removeFromList(confStringMap, []string{"service"}, "extensions", ddprofilingName())
}

func removeHpFlareExtension(confStringMap map[string]any) error {
	if err := removeFromMap(confStringMap, []string{"extensions"}, hpflareName()); err != nil {
		return err
	}

	return removeFromList(confStringMap, []string{"service"}, "extensions", hpflareName())
}

func infraAttributesName() string {
	return "infraattributes/default"
}

func resourceDetectionName() string {
	return "resourcedetection"
}

func ddprofilingName() string {
	return "ddprofiling/default"
}

func hpflareName() string {
	return "hpflare/default"
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

func mergeValues(destinationValue any, sourceValue any) (any, bool) {
	destList, isDestList := toList(destinationValue)
	sourceList, isSourceList := toList(sourceValue)

	// If neither was a list, just replace
	if !isDestList && !isSourceList {
		return sourceValue, sourceValue != destinationValue
	}

	// At least one is a list, so merge them
	return mergeListsWithDedup(destList, sourceList)
}

func toList(value any) ([]any, bool) {
	if list, ok := value.([]any); ok {
		return list, true
	}

	return []any{value}, false
}

func mergeListsWithDedup(dest []any, source []any) ([]any, bool) {
	result := slices.Clone(dest)
	hasChanged := false
	for _, item := range source {
		if !slices.ContainsFunc(result, func(e any) bool { return e == item }) {
			result = append(result, item)
			hasChanged = true
		}
	}

	return result, hasChanged
}

func mergeMap(destination map[string]any, source map[string]any) bool {
	hasMerged := false
	for key, sourceValue := range source {
		destinationChild, ok := destination[key]
		if !ok {
			// if node not present in destination, add it
			destination[key] = sourceValue
			hasMerged = true
			log.Debugf("Added node to configuration file: %s:\n\t%v", key, sourceValue)
			continue
		}

		sourceChildMap, isSourceMap := sourceValue.(map[string]any)
		destinationChildMap, isDestinationMap := destinationChild.(map[string]any)

		if isSourceMap && isDestinationMap {
			changed := mergeMap(destinationChildMap, sourceChildMap)
			hasMerged = hasMerged || changed
			continue
		}

		merged, hasChanged := mergeValues(destinationChild, sourceValue)
		if hasChanged {
			log.Debugf("Modified configuration leaf %s:%v", key, merged)
			destination[key] = merged
			hasMerged = true
		}
	}

	return hasMerged
}
