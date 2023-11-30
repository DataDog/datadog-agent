// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package secretsimpl

import (
	"github.com/DataDog/datadog-agent/comp/core/secrets"
)

type resolverCallback func([]string, string) (string, error)

type walker struct {
	// resolver is called to fetch the value of a handle
	resolver resolverCallback
	// notifier is called each time a key from the YAML is updated. This is used by ResolveWithCallback.
	//
	// When a slice is updated, this will be called once with the final slice content.
	notifier secrets.ResolveCallback
}

func (w *walker) notify(yamlPath []string, value any) {
	if w.notifier != nil {
		w.notifier(yamlPath, value)
	}
}

func (w *walker) string(yamlPath []string, value string, shouldNotify bool) (string, error) {
	newValue, err := w.resolver(yamlPath, value)
	if err != nil {
		return value, err
	}

	if shouldNotify && value != newValue {
		w.notify(yamlPath, newValue)
	}
	return newValue, err
}

func (w *walker) slice(data []interface{}, yamlPath []string) error {
	var shouldNotify bool
	for idx, k := range data {
		switch v := k.(type) {
		case string:
			if newValue, err := w.string(yamlPath, v, false); err == nil {
				if v != newValue {
					data[idx] = newValue
					shouldNotify = true
				}
			} else {
				return err
			}
		case map[interface{}]interface{}:
			if err := w.hash(v, yamlPath); err != nil {
				return err
			}
		case []interface{}:
			if err := w.slice(v, yamlPath); err != nil {
				return err
			}
		}
	}
	// for slice we notify once with the final values
	if shouldNotify {
		w.notify(yamlPath, data)
	}
	return nil
}

func (w *walker) hash(data map[interface{}]interface{}, yamlPath []string) error {
	for k := range data {
		path := yamlPath
		if newkey, ok := k.(string); ok {
			path = append(path, newkey)
		}

		switch v := data[k].(type) {
		case string:
			if newValue, err := w.string(path, v, true); err == nil {
				data[k] = newValue
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

// walk will go through loaded yaml and invoke the callback on every string node allowing
// the callback to overwrite the string value
func (w *walker) walk(data *interface{}) error {
	switch v := (*data).(type) {
	case string:
		if newValue, err := w.string(nil, v, true); err == nil {
			*data = newValue
		} else {
			return err
		}
	case map[interface{}]interface{}:
		return w.hash(v, nil)
	case []interface{}:
		return w.slice(v, nil)
	}
	return nil
}
