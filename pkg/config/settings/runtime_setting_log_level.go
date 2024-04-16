// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package settings

import (
	"github.com/DataDog/datadog-agent/comp/core/config"
	pkgconfiglogs "github.com/DataDog/datadog-agent/pkg/config/logs"
	"github.com/DataDog/datadog-agent/pkg/config/model"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

// LogLevelRuntimeSetting wraps operations to change log level at runtime.
type LogLevelRuntimeSetting struct {
	ConfigKey string
}

// NewLogLevelRuntimeSetting returns a new LogLevelRuntimeSetting
func NewLogLevelRuntimeSetting() *LogLevelRuntimeSetting {
	return &LogLevelRuntimeSetting{
		ConfigKey: "log_level",
	}
}

// Description returns the runtime setting's description
func (l *LogLevelRuntimeSetting) Description() string {
	return "Set/get the log level, valid values are: trace, debug, info, warn, error, critical and off"
}

// Hidden returns whether or not this setting is hidden from the list of runtime settings
func (l *LogLevelRuntimeSetting) Hidden() bool {
	return false
}

// Name returns the name of the runtime setting
func (l *LogLevelRuntimeSetting) Name() string {
	return l.ConfigKey
}

// Get returns the current value of the runtime setting
func (l *LogLevelRuntimeSetting) Get(_ config.Component) (interface{}, error) {
	level, err := log.GetLogLevel()
	if err != nil {
		return "", err
	}
	return level.String(), nil
}

// Set changes the value of the runtime setting
func (l *LogLevelRuntimeSetting) Set(config config.Component, v interface{}, source model.Source) error {
	level := v.(string)

	err := pkgconfiglogs.ChangeLogLevel(level)
	if err != nil {
		return err
	}

	key := "log_level"
	if l.ConfigKey != "" {
		key = l.ConfigKey
	}

	config.Set(key, level, source)
	return nil
}
