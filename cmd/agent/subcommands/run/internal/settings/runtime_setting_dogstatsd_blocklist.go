// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package settings

import (
	"fmt"

	"github.com/DataDog/datadog-agent/comp/core/config"
	dogstatsd "github.com/DataDog/datadog-agent/comp/dogstatsd/server"
	"github.com/DataDog/datadog-agent/pkg/config/model"
	"github.com/DataDog/datadog-agent/pkg/config/settings"
)

// DsdBlocklistRuntimeSetting wraps operations to change the collection of dogstatsd stats at runtime.
type DsdBlocklistRuntimeSetting struct {
	Server dogstatsd.Component
}

// NewDsdStatsRuntimeSetting creates a new instance of DsdBlocklistRuntimeSetting
func NewDsdBlocklistRuntimeSetting(server dogstatsd.Component) *DsdBlocklistRuntimeSetting {
	return &DsdBlocklistRuntimeSetting{
		Server: server,
	}
}

// Description returns the runtime setting's description
func (s *DsdBlocklistRuntimeSetting) Description() string {
	return "Enable/disable the dogstatsd debug stats. Possible values: true, false"
}

// Hidden returns whether or not this setting is hidden from the list of runtime settings
func (s *DsdBlocklistRuntimeSetting) Hidden() bool {
	return true
}

// Name returns the name of the runtime setting
func (s *DsdBlocklistRuntimeSetting) Name() string {
	return string("dogstatsd_stats")
}

// Get returns the current value of the runtime setting
func (s *DsdBlocklistRuntimeSetting) Get(_ config.Component) (interface{}, error) {
	return s.Server.GetBlocklist(), nil
}

// Set changes the value of the runtime setting
func (s *DsdBlocklistRuntimeSetting) Set(config config.Component, v interface{}, source model.Source) error {
	var values []string
	var err error

	if values, err = settings.GetStrings(v, " "); err != nil {
		return fmt.Errorf("DsdBlocklistRuntimeSetting: %v", err)
	}

	s.Server.SetBlocklist(values)

	config.Set("statsd_metric_blocklist", values, source)
	return nil
}
