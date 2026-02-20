// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package utils contains a series of helpers function for the secrets Component and its mock. Direct usage of this
// helpers outside of the secrets Component sphere should be avoided.
package utils

import (
	"errors"
	"slices"
	"strconv"
	"strings"
)

// IsEnc returns true is the string match the 'ENC[...]' format
func IsEnc(str string) (bool, string) {
	// trimming space and tabs
	str = strings.Trim(str, " 	")
	if strings.HasPrefix(str, "ENC[") && strings.HasSuffix(str, "]") {
		return true, str[4 : len(str)-1]
	}
	return false, ""
}

// Walker recursively explores a loaded YAML in search for string values to replace. For each string the 'Resolver'
// callback will be called allowing it to overwrite the string value.
type Walker struct {
	// Resolver is called for eachs string type found in the data tree
	Resolver func(path []string, value string) (string, error)
}

// string handles string types, calling the resolver and returning the value to replace the original string with.
func (w *Walker) string(text string, yamlPath []string) (string, error) {
	newValue, err := w.Resolver(yamlPath, text)
	if err != nil {
		return text, err
	}
	return newValue, err
}

// slice handles slice types, the Walker will recursively explore each element of the slice continuing its search for
// strings to replace.
func (w *Walker) slice(currentSlice []interface{}, yamlPath []string) error {
	for idx, k := range currentSlice {

		// clone the path to avoid modifying it, caller still needs to use it
		path := append(slices.Clone(yamlPath), strconv.Itoa(idx))

		switch v := k.(type) {
		case string:
			newValue, err := w.string(v, path)
			if err != nil {
				return err
			}
			currentSlice[idx] = newValue
		case map[interface{}]interface{}:
			if err := w.hash(v, path); err != nil {
				return err
			}
		case []interface{}:
			if err := w.slice(v, path); err != nil {
				return err
			}
		}
	}
	return nil
}

// hash handles map types, the Walker will recursively explore each element of the map continuing its search for
// strings to replace.
func (w *Walker) hash(currentMap map[interface{}]interface{}, yamlPath []string) error {
	for configKey := range currentMap {
		path := yamlPath
		if newkey, ok := configKey.(string); ok {
			path = append(path, newkey)
		}

		switch v := currentMap[configKey].(type) {
		case string:
			if newValue, err := w.string(v, path); err == nil {
				currentMap[configKey] = newValue
			} else {
				return err
			}
		case map[interface{}]interface{}:
			if err := w.hash(v, path); err != nil {
				return err
			}
		case []interface{}:
			if err := w.slice(v, path); err != nil {
				return err
			}
		}
	}
	return nil
}

// Walk recursively explores a loaded YAML in search for string values to replace. For each string the 'resolver' will
// be called allowing it to overwrite the string value.
func (w *Walker) Walk(data *interface{}) error {
	switch v := (*data).(type) {
	case map[interface{}]interface{}:
		return w.hash(v, nil)
	case []interface{}:
		return w.slice(v, nil)
	default:
		return errors.New("given data is not of expected type map not slice")
	}
}
