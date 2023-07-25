// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package settings

import (
	"github.com/DataDog/datadog-agent/pkg/config"
	"github.com/DataDog/datadog-agent/pkg/metadata/inventories"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

// LogLevelRuntimeSetting wraps operations to change log level at runtime.
type LogLevelRuntimeSetting struct {
	Config    config.ConfigReaderWriter
	ConfigKey string
}

type LogLevelStatus struct {
	Level  string
	Source log.LogLevelSource
}

// Description returns the runtime setting's description
func (l LogLevelRuntimeSetting) Description() string {
	return "Set/get the log level, valid values are: trace, debug, info, warn, error, critical and off"
}

// Hidden returns whether or not this setting is hidden from the list of runtime settings
func (l LogLevelRuntimeSetting) Hidden() bool {
	return false
}

// Name returns the name of the runtime setting
func (l LogLevelRuntimeSetting) Name() string {
	return "log_level"
}

// Get returns the current value of the runtime setting
func (l LogLevelRuntimeSetting) Get() (interface{}, error) {
	level, err := log.GetLogLevel()
	if err != nil {
		return "", err
	}
	return level.String(), nil
}

// Set changes the value of the runtime setting.
// The input interface is either a string or a settings.LogLevelStatus
func (l LogLevelRuntimeSetting) Set(v interface{}) error {
	// This function can be called 2 different ways:
	//     - the "standard" way, by inputing a string
	//     - a "special case", by inputing a LogLevelStatus struct
	// This special case is requiered to handle the source changes
	// We don't want the user to see this source parameter so it cannot be saved as a separate setting
	// Moreover this function is called throughout the hole code
	//     so it would be difficult to change the standard implementation everywhere
	//     and a very error prone process

	logLevelStatus, ok := v.(LogLevelStatus)
	if !ok {
		// If the input is a string it means this function has been called in the "standard way"
		// Therefore we can assume that the change is coming from the CLI
		lvl := v.(string)
		// Doesn't check for casting errors as they were none before
		logLevelStatus = LogLevelStatus{
			Level:  lvl,
			Source: log.LogLevelSourceCLI,
		}
	}
	err := config.ChangeLogLevel(logLevelStatus.Level)
	if err != nil {
		return err
	}

	err = config.ChangeSource(logLevelStatus.Source)
	if err != nil {
		return err
	}

	key := "log_level"
	if l.ConfigKey != "" {
		key = l.ConfigKey
	}
	var cfg config.ConfigReaderWriter = config.Datadog
	if l.Config != nil {
		cfg = l.Config
	}
	cfg.Set(key, logLevelStatus.Level)
	// we trigger a new inventory metadata payload since the configuration was updated by the user.
	inventories.Refresh()
	return nil
}
