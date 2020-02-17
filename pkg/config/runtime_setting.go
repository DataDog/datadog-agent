/// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-2020 Datadog, Inc.

package config

import (
	"errors"
)

// RuntimeSettings registers all runtime editable config
var RuntimeSettings = make(map[string](RuntimeSetting))

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
	if _, ok := RuntimeSettings[setting.Name()]; ok {
		return errors.New("duplicated settings detected")
	}
	RuntimeSettings[setting.Name()] = setting
	return nil
}
