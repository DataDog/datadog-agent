// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

package settings

import (
	"fmt"

	"github.com/DataDog/datadog-agent/pkg/config"
	"github.com/DataDog/datadog-agent/pkg/config/model"
)

// HARuntimeSetting determines whether or not the agent is in high availability failover mode
type HARuntimeSetting struct {
	ConfigKey string
}

// NewHARuntimeSetting returns a new HARuntimeSetting
func NewHARuntimeSetting() *HARuntimeSetting {
	return &HARuntimeSetting{ConfigKey: "ha_failover"}
}

// Description returns the runtime setting's description
func (l *HARuntimeSetting) Description() string {
	return "Enables high availability failover mode at runtime."
}

// Hidden returns whether or not this setting is hidden from the list of runtime settings
func (l *HARuntimeSetting) Hidden() bool {
	return false
}

// Name returns the name of the runtime setting
func (l *HARuntimeSetting) Name() string {
	return l.ConfigKey
}

// Get returns the current value of the runtime setting
func (l *HARuntimeSetting) Get() (interface{}, error) {
	return config.Datadog.GetBool("ha.failover"), nil
}

// Set changes the value of the runtime setting
func (l *HARuntimeSetting) Set(v interface{}, source model.Source) error {
	var newValue bool
	var err error

	if newValue, err = GetBool(v); err != nil {
		return fmt.Errorf("HARuntimeSetting: %v", err)
	}

	config.Datadog.Set("ha.failover", newValue, source)
	return nil
}
