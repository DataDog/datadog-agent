// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package settings implements runtime settings and profiling
package settings

import (
	"fmt"
	"strconv"
	"sync"

	"github.com/DataDog/datadog-agent/pkg/config/model"
)

var runtimeSettings = make(map[string]RuntimeSetting)
var runtimeSettingsLock = sync.Mutex{}

// SettingNotFoundError is used to warn about non existing/not registered runtime setting
type SettingNotFoundError struct {
	name string
}

// RuntimeSettingResponse is used to communicate settings config
type RuntimeSettingResponse struct {
	Description string
	Hidden      bool
}

func (e *SettingNotFoundError) Error() string {
	return fmt.Sprintf("setting %s not found", e.name)
}

// RuntimeSetting represents a setting that can be changed and read at runtime.
type RuntimeSetting interface {
	Get() (interface{}, error)
	Set(v interface{}, source model.Source) error
	Name() string
	Description() string
	Hidden() bool
}

// RegisterRuntimeSetting keeps track of configurable settings
func RegisterRuntimeSetting(setting RuntimeSetting) error {
	if _, ok := runtimeSettings[setting.Name()]; ok {
		return fmt.Errorf("duplicated settings detected: %s", setting.Name())
	}
	runtimeSettings[setting.Name()] = setting
	return nil
}

// RuntimeSettings returns all runtime configurable settings
func RuntimeSettings() map[string]RuntimeSetting {
	return runtimeSettings
}

// SetRuntimeSetting changes the value of a runtime configurable setting
func SetRuntimeSetting(setting string, value interface{}, source model.Source) error {
	runtimeSettingsLock.Lock()
	defer runtimeSettingsLock.Unlock()
	if _, ok := runtimeSettings[setting]; !ok {
		return &SettingNotFoundError{name: setting}
	}
	return runtimeSettings[setting].Set(value, source)
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

// GetBool returns the bool value contained in value.
// If value is a bool, returns its value
// If value is a string, it converts "true" to true and "false" to false.
// Else, returns an error.
func GetBool(v interface{}) (bool, error) {
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
			return false, fmt.Errorf("GetBool: bad parameter value provided: %v", str)
		}

	}
	b, ok := v.(bool)
	if !ok {
		return false, fmt.Errorf("GetBool: bad parameter value provided")
	}
	return b, nil
}

// GetInt returns the integer value contained in value.
// If value is a integer, returns its value
// If value is a string, it parses the string into an integer.
// Else, returns an error.
func GetInt(v interface{}) (int, error) {
	switch v := v.(type) {
	case int:
		return v, nil
	case string:
		i, err := strconv.ParseInt(v, 10, 0)
		if err != nil {
			return 0, fmt.Errorf("GetInt: %s", err)
		}
		return int(i), nil
	default:
		return 0, fmt.Errorf("GetInt: bad parameter value provided: %v", v)
	}
}
