// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package secretsimpl

import (
	"fmt"

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

	// notificationPending is true when a notification needs to be send. See documentation for ResolveCallback for
	// more information.
	notificationPending bool
}

// notify calls the 'notifier' callback with the current path in the yaml and its value. If the notifier returns true,
// the notification was acknowledged. If false if returned we set 'notificationPending' to true which will retrigger the
// notification on the parent node in the yamlPath.
func (w *walker) notify(yamlPath []string, value any) {
	if w.notifier != nil {
		if !w.notifier(yamlPath, value) {
			// notification was refuse, will retry from the parent type.
			w.notificationPending = true
		}
	}
}

// string handles string types, calling the resolver and returning the value to replace the original string with.
//
// 'shouldNotify' should be set to false when the string is contained in a slice as we want to send a single
// notification per slice and not 1 per item in it.
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

// slice handles slice types, the walker will recursively explore each element of the slice continuing its search for
// strings to replace.
func (w *walker) slice(currentSlice []interface{}, yamlPath []string) error {
	var shouldNotify bool
	for idx, k := range currentSlice {
		switch v := k.(type) {
		case string:
			if newValue, err := w.string(yamlPath, v, false); err == nil {
				if v != newValue {
					currentSlice[idx] = newValue
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
	if shouldNotify || w.notificationPending {
		w.notify(yamlPath, currentSlice)
	}
	return nil
}

// hash handles map types, the walker will recursively explore each element of the map continuing its search for
// strings to replace.
func (w *walker) hash(currentMap map[interface{}]interface{}, yamlPath []string) error {
	for configKey := range currentMap {
		path := yamlPath
		if newkey, ok := configKey.(string); ok {
			path = append(path, newkey)
		}

		switch v := currentMap[configKey].(type) {
		case string:
			if newValue, err := w.string(path, v, true); err == nil {
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
	if w.notificationPending {
		w.notify(yamlPath, currentMap)
	}
	return nil
}

// walk recursively explores a loaded YAML in search for string values to replace. For each string the 'resolver' will
// be called allowing it to overwrite the string value.
//
// Each time a value is changed by 'resolver' a notification is sent using the 'notifier' callback.
func (w *walker) walk(data *interface{}) error {
	// In case all notification are refused by the 'notifier' callback we clear the state to be ready for the next
	// call to 'walk'.
	defer func() { w.notificationPending = false }()

	switch v := (*data).(type) {
	case map[interface{}]interface{}:
		return w.hash(v, nil)
	case []interface{}:
		return w.slice(v, nil)
	default:
		return fmt.Errorf("given data is not of expected type map not slice")
	}
}
