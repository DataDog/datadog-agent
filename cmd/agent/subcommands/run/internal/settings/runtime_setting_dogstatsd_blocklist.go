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

// DsdMetricNameBlocklist wraps operations to change metric names blocklist at runtime.
type DsdMetricNameBlocklist struct {
	Server dogstatsd.Component
}

// NewDsdMetricNameBlocklist creates a new instance of DsdMetricNameBlocklist
func NewDsdMetricNameBlocklist(server dogstatsd.Component) *DsdMetricNameBlocklist {
	return &DsdMetricNameBlocklist{
		Server: server,
	}
}

// Description returns the runtime setting's description
func (s *DsdMetricNameBlocklist) Description() string {
	return "Set the metric names blocklist. Format: metric names separated by space"
}

// Hidden returns whether or not this setting is hidden from the list of runtime settings
func (s *DsdMetricNameBlocklist) Hidden() bool {
	return true
}

// Name returns the name of the runtime setting
func (s *DsdMetricNameBlocklist) Name() string {
	return string("dogstatsd_metric_name_blocklist")
}

// Get returns the current value of the runtime setting
func (s *DsdMetricNameBlocklist) Get(_ config.Component) (interface{}, error) {
	return s.Server.GetBlocklist(), nil
}

// Set changes the value of the runtime setting
func (s *DsdMetricNameBlocklist) Set(config config.Component, v interface{}, source model.Source) error {
	var values []string
	var err error

	if values, err = settings.GetStrings(v, " "); err != nil {
		return fmt.Errorf("DsdMetricNameBlocklist: %v", err)
	}

	s.Server.SetBlocklist(values)

	config.Set("statsd_metric_blocklist", values, source)
	return nil
}
