// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package settings

import (
	"fmt"

	"github.com/DataDog/datadog-agent/pkg/config"
	"github.com/DataDog/datadog-agent/pkg/config/model"
)

// LogPayloadsRuntimeSetting wraps operations to start logging aggregator payload at runtime.
type LogPayloadsRuntimeSetting struct {
	ConfigKey string
}

// NewLogPayloadsRuntimeSetting returns a new LogPayloadsRuntimeSetting
func NewLogPayloadsRuntimeSetting() *LogPayloadsRuntimeSetting {
	return &LogPayloadsRuntimeSetting{ConfigKey: "log_payloads"}
}

// Description returns the runtime setting's description
func (l *LogPayloadsRuntimeSetting) Description() string {
	return "Enable logging payloads at runtime."
}

// Hidden returns whether or not this setting is hidden from the list of runtime settings
func (l *LogPayloadsRuntimeSetting) Hidden() bool {
	return true
}

// Name returns the name of the runtime setting
func (l *LogPayloadsRuntimeSetting) Name() string {
	return l.ConfigKey
}

// Get returns the current value of the runtime setting
func (l *LogPayloadsRuntimeSetting) Get() (interface{}, error) {
	return config.Datadog.GetBool("log_payloads"), nil
}

// Set changes the value of the runtime setting
func (l *LogPayloadsRuntimeSetting) Set(v interface{}, source model.Source) error {
	var newValue bool
	var err error

	if newValue, err = GetBool(v); err != nil {
		return fmt.Errorf("LogPayloadsRuntimeSetting: %v", err)
	}

	config.Datadog.Set("log_payloads", newValue, source)
	return nil
}
