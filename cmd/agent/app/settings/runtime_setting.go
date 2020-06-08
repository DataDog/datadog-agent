// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-2020 Datadog, Inc.

package settings

import (
	"errors"
	"fmt"
)

var runtimeSettings = make(map[string]RuntimeSetting)

// SettingNotFoundError is used to warn about non existing/not registered runtime setting
type SettingNotFoundError struct {
	name string
}

func (e *SettingNotFoundError) Error() string {
	return fmt.Sprintf("setting %s not found", e.name)
}

// RuntimeSetting represents a setting that can be changed and read at runtime.
type RuntimeSetting interface {
	Get() (interface{}, error)
	Set(v interface{}) error
	Name() string
	Description() string
}

// InitRuntimeSettings builds the map of runtime settings configurable at runtime.
func InitRuntimeSettings() error {
	// Runtime-editable settings must be registered here to dynamically populate command-line information
	if err := registerRuntimeSetting(logLevelRuntimeSetting("log_level")); err != nil {
		return err
	}
	if err := registerRuntimeSetting(dsdStatsRuntimeSetting("dogstatsd_stats")); err != nil {
		return err
	}
	if err := registerRuntimeSetting(profilingRuntimeSetting("profiling")); err != nil {
		return err
	}

	return nil
}

// RegisterRuntimeSettings keeps track of configurable settings
func registerRuntimeSetting(setting RuntimeSetting) error {
	if _, ok := runtimeSettings[setting.Name()]; ok {
		return errors.New("duplicated settings detected")
	}
	runtimeSettings[setting.Name()] = setting
	return nil
}

// RuntimeSettings returns all runtime configurable settings
func RuntimeSettings() map[string]RuntimeSetting {
	return runtimeSettings
}

// SetRuntimeSetting changes the value of a runtime configurable setting
func SetRuntimeSetting(setting string, value interface{}) error {
	if _, ok := runtimeSettings[setting]; !ok {
		return &SettingNotFoundError{name: setting}
	}
	if err := runtimeSettings[setting].Set(value); err != nil {
		return err
	}
	return nil
}

// GetRuntimeSetting returns the value of a runtime configurable setting
func GetRuntimeSetting(setting string) (interface{}, error) {
	if _, ok := runtimeSettings[setting]; !ok {
		return nil, &SettingNotFoundError{name: setting}
	}
	value, err := runtimeSettings[setting].Get()
	if err != nil {
		return nil, err
	}
	return value, nil
}

// getBool returns the bool value contained in value.
// If value is a bool, returns its value
// If value is a string, it converts "true" to true and "false" to false.
// Else, returns an error.
func getBool(v interface{}) (bool, error) {
	// to be cautious, take care of both calls with a string (cli) or a bool (programmaticaly)
	str, ok := v.(string)
	if ok {
		// string value
		switch str {
		case "true":
			return true, nil
		case "false":
			return false, nil
		default:
			return false, fmt.Errorf("getBool: bad parameter value provided: %v", str)
		}

	}
	b, ok := v.(bool)
	if !ok {
		return false, fmt.Errorf("getBool: bad parameter value provided")
	}
	return b, nil
}
