/// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-2020 Datadog, Inc.

package config

import (
	"errors"
)

// RuntimeSettings registers all runtime editable config
var runtimeSettings = make(map[string](RuntimeSetting))

// RuntimeSetting represents a setting that can be changed and read at runtime.
type RuntimeSetting interface {
	Get() (interface{}, error)
	Set(v interface{}) error
	Name() string
	Description() string
}

func initRuntimeSettings() {
	// Runtime-editable settings must be registered here to dynamically populate command-line information
	registerRuntimeSetting(RuntimeSetting(ll))
}

// RegisterRuntimeSettings keeps track of configurable settings
func registerRuntimeSetting(setting RuntimeSetting) error {
	if _, ok := runtimeSettings[setting.Name()]; ok {
		return errors.New("duplicated settings detected")
	}
	runtimeSettings[setting.Name()] = setting
	return nil
}

// RuntimeSettings return all runtime configurable settings
func RuntimeSettings() map[string]RuntimeSetting {
	return runtimeSettings
}

// SetRuntimeSetting change the value of a runtime configurable setting
func SetRuntimeSetting(setting string, value interface{}) error {
	if _, ok := runtimeSettings[setting]; !ok {
		return errors.New("unknown setting")
	}
	if err := runtimeSettings[setting].Set(value); err != nil {
		return err
	}
	return nil
}
