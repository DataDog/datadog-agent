// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package settings

import (
	"fmt"

	"github.com/DataDog/datadog-agent/comp/core/config"
	"github.com/DataDog/datadog-agent/pkg/config/model"
	"github.com/DataDog/datadog-agent/pkg/config/settings"
)

// MultiRegionFailoverRuntimeSetting wraps operations to change the High Availability settings at runtime.
type MultiRegionFailoverRuntimeSetting struct {
	value string
	desc  string
}

// NewMultiRegionFailoverRuntimeSetting creates a new instance of MultiRegionFailoverRuntimeSetting
func NewMultiRegionFailoverRuntimeSetting(name, desc string) *MultiRegionFailoverRuntimeSetting {
	return &MultiRegionFailoverRuntimeSetting{
		value: name,
		desc:  desc,
	}
}

// Description returns the runtime setting's description
func (h *MultiRegionFailoverRuntimeSetting) Description() string {
	return h.desc + " Possible values: true, false"
}

// Hidden returns whether or not this setting is hidden from the list of runtime settings
func (h *MultiRegionFailoverRuntimeSetting) Hidden() bool {
	return true
}

// Name returns the name of the runtime setting
func (h *MultiRegionFailoverRuntimeSetting) Name() string {
	return h.value
}

// Get returns the current value of the runtime setting
func (h *MultiRegionFailoverRuntimeSetting) Get(config config.Component) (interface{}, error) {
	return config.GetBool(h.value), nil
}

// Set changes the value of the runtime setting; expected to be boolean
func (h *MultiRegionFailoverRuntimeSetting) Set(config config.Component, v interface{}, source model.Source) error {
	var newValue bool
	var err error

	if newValue, err = settings.GetBool(v); err != nil {
		return fmt.Errorf("%v: %v", h.value, err)
	}

	config.Set(h.value, newValue, source)
	return nil
}
