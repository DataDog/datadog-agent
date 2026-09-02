// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package model

import (
	"fmt"
	"slices"
	"strconv"
	"strings"
)

// PathWriter is the config subset required by AssignAtPath.
type PathWriter interface {
	Get(string) interface{}
	IsKnown(string) bool
	Set(string, interface{}, Source)
}

// AssignAtPath updates one value inside a config setting. Path elements after the
// nearest known setting address map keys or slice indices.
func AssignAtPath(config PathWriter, settingPath []string, newValue any, source Source) error {
	settingName := strings.Join(settingPath, ".")
	if config.IsKnown(settingName) {
		config.Set(settingName, newValue, source)
		return nil
	}

	path := slices.Clone(settingPath)
	trailing := make([]string, 0, len(path))
	for {
		if len(path) == 0 {
			return fmt.Errorf("unknown config setting '%s'", settingPath)
		}
		last := path[len(path)-1]
		trailing = append(trailing, last)
		path = path[:len(path)-1]
		settingName = strings.Join(path, ".")
		if config.IsKnown(settingName) {
			break
		}
	}
	slices.Reverse(trailing)

	root := config.Get(settingName)
	current := root
	for i, elem := range trailing {
		last := i == len(trailing)-1
		switch value := current.(type) {
		case map[string]interface{}:
			if last {
				value[elem] = newValue
			} else {
				current = value[elem]
			}
		case map[string][]string:
			if last {
				return fmt.Errorf("cannot assign scalar value to list setting '%s'", settingPath)
			}
			current = value[elem]
		case map[interface{}]interface{}:
			key := any(elem)
			if index, err := strconv.Atoi(elem); err == nil {
				if _, found := value[index]; found {
					key = index
				}
			}
			if last {
				value[key] = newValue
			} else {
				current = value[key]
			}
		case []string:
			index, err := pathIndex(elem, len(value))
			if err != nil {
				return err
			}
			if last {
				value[index] = fmt.Sprint(newValue)
			} else {
				current = value[index]
			}
		case []interface{}:
			index, err := pathIndex(elem, len(value))
			if err != nil {
				return err
			}
			if last {
				value[index] = newValue
			} else {
				current = value[index]
			}
		case []map[string]interface{}:
			index, err := pathIndex(elem, len(value))
			if err != nil {
				return err
			}
			if last {
				return fmt.Errorf("cannot replace map entry '%s' with a scalar", settingPath)
			}
			current = value[index]
		case []map[interface{}]interface{}:
			index, err := pathIndex(elem, len(value))
			if err != nil {
				return err
			}
			if last {
				return fmt.Errorf("cannot replace map entry '%s' with a scalar", settingPath)
			}
			current = value[index]
		default:
			return fmt.Errorf("cannot assign to setting '%s' of type %T", settingPath, current)
		}
	}

	config.Set(settingName, root, source)
	return nil
}

func pathIndex(value string, length int) (int, error) {
	index, err := strconv.Atoi(value)
	if err != nil {
		return 0, err
	}
	if index < 0 || index >= length {
		return 0, fmt.Errorf("index out of range %d >= %d", index, length)
	}
	return index, nil
}
