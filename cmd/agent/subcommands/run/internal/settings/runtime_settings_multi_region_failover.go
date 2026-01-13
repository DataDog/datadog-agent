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

// MultiRegionFailoverRuntimeSetting wraps operations to change the Multi-Region Failover settings at runtime.
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
	if h.value == "multi_region_failover.metric_allowlist" {
		return config.GetStringSlice(h.value), nil
	}

	return config.GetBool(h.value), nil
}

// Set changes the value of the runtime setting; expected to be boolean or []string
func (h *MultiRegionFailoverRuntimeSetting) Set(config config.Component, v interface{}, source model.Source) error {
	var newValue interface{}
	var err error

	switch v.(type) {
	case bool:
		newValue, err = settings.GetBool(v)
	case []string, nil:
		// nil means "value not set" - for allowlist, this means every metric is allowed.
		newValue, err = settings.GetStringSlice(v)
	default:
		return fmt.Errorf("%v: bad parameter value provided: %v", h.value, v)
	}
	if err != nil {
		return fmt.Errorf("%v: %v", h.value, err)
	}

	config.Set(h.value, newValue, source)
	return nil
}
