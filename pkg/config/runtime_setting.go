/// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-2020 Datadog, Inc.

package config

import (
	"errors"

	"github.com/DataDog/datadog-agent/pkg/util/log"
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

// LogLevel wraps operations to change log level at runtime
type logLevel string

var ll logLevel = "log_level"

// RegisterRuntimeSettings keeps track of configurable settings
func registerRuntimeSetting(setting RuntimeSetting) error {
	if _, ok := RuntimeSettings[setting.Name()]; ok {
		return errors.New("duplicated settings detected")
	}
	RuntimeSettings[setting.Name()] = setting
	return nil
}

func (l logLevel) Description() string {
	return "Set/get the log level, valid values are: trace, debug, info, warn, error, critical and off"
}

func (l logLevel) Name() string {
	return string(l)
}

func (l logLevel) Get() (interface{}, error) {
	level, err := log.GetLogLevel()
	if err != nil {
		return "", err
	}
	return level.String(), nil
}

func (l logLevel) Set(v interface{}) error {
	logLevel := v.(string)
	err := log.ChangeLogLevel(logLevel)
	if err != nil {
		return err
	}
	Datadog.Set("log_level", logLevel)
	return nil
}
