// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package settings

import (
	"fmt"

	"github.com/DataDog/datadog-agent/pkg/config"
	"github.com/DataDog/datadog-agent/pkg/config/model"
	"github.com/DataDog/datadog-agent/pkg/config/settings"
)

// HighAvailabilityRuntimeSetting wraps operations to change the High Availability settings at runtime.
type HighAvailabilityRuntimeSetting struct {
	value string
	desc  string
}

// NewHighAvailabilityRuntimeSetting creates a new instance of HighAvailabilityRuntimeSetting
func NewHighAvailabilityRuntimeSetting(name, desc string) *HighAvailabilityRuntimeSetting {
	return &HighAvailabilityRuntimeSetting{
		value: name,
		desc:  desc,
	}
}

// Description returns the runtime setting's description
func (h *HighAvailabilityRuntimeSetting) Description() string {
	return h.desc + " Possible values: true, false"
}

// Hidden returns whether or not this setting is hidden from the list of runtime settings
func (h *HighAvailabilityRuntimeSetting) Hidden() bool {
	return false
}

// Name returns the name of the runtime setting
func (h *HighAvailabilityRuntimeSetting) Name() string {
	return h.value
}

// Get returns the current value of the runtime setting
func (h *HighAvailabilityRuntimeSetting) Get() (interface{}, error) {
	return config.Datadog.GetBool(h.value), nil
}

// Set changes the value of the runtime setting; expected to be boolean
func (h *HighAvailabilityRuntimeSetting) Set(v interface{}, source model.Source) error {
	var newValue bool
	var err error

	if newValue, err = settings.GetBool(v); err != nil {
		return fmt.Errorf("%v: %v", h.value, err)
	}

	config.Datadog.Set(h.value, newValue, source)
	return nil
}
