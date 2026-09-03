// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package settings

import (
	"fmt"

	"github.com/DataDog/datadog-agent/comp/core/config"
	"github.com/DataDog/datadog-agent/pkg/config/model"
)

// InternalTelemetryRuntimeSetting wraps operations to change what the telemetry check reports
// from the Agent's internal telemetry registry, at runtime.
type InternalTelemetryRuntimeSetting struct {
	name string
	desc string
}

// NewInternalTelemetryRuntimeSetting creates a new instance of InternalTelemetryRuntimeSetting
func NewInternalTelemetryRuntimeSetting(name, desc string) *InternalTelemetryRuntimeSetting {
	return &InternalTelemetryRuntimeSetting{
		name: name,
		desc: desc,
	}
}

// Description returns the runtime setting's description
func (t *InternalTelemetryRuntimeSetting) Description() string {
	return t.desc + " Possible values: true, false"
}

// Hidden returns whether or not this setting is hidden from the list of runtime settings
func (t *InternalTelemetryRuntimeSetting) Hidden() bool {
	return false
}

// Name returns the name of the runtime setting
func (t *InternalTelemetryRuntimeSetting) Name() string {
	return t.name
}

// Get returns the current value of the runtime setting
func (t *InternalTelemetryRuntimeSetting) Get(config config.Component) (interface{}, error) {
	return config.GetBool(t.name), nil
}

// Set changes the value of the runtime setting
func (t *InternalTelemetryRuntimeSetting) Set(config config.Component, v interface{}, source model.Source) error {
	newValue, err := getBool(v)
	if err != nil {
		return fmt.Errorf("%v: %v", t.name, err)
	}

	config.Set(t.name, newValue, source)
	return nil
}
