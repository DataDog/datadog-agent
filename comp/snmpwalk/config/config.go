// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2020-present Datadog, Inc.

// Package config defines the configuration options for the snmpwalk services.
package config

import (
	"fmt"
	"github.com/DataDog/datadog-agent/comp/core/log"

	"github.com/DataDog/datadog-agent/comp/core/config"

	"github.com/DataDog/datadog-agent/pkg/snmp/utils"

	"github.com/DataDog/datadog-agent/comp/snmpwalk/common"
)

// SnmpwalkConfig contains configuration for Snmpwalk collector.
type SnmpwalkConfig struct {
	Enabled     bool `mapstructure:"enabled"`
	StopTimeout int  `mapstructure:"stop_timeout"`
}

// ReadConfig builds and returns configuration from Agent configuration.
func ReadConfig(conf config.Component, logger log.Component) (*SnmpwalkConfig, error) {
	var mainConfig SnmpwalkConfig

	err := conf.UnmarshalKey("network_devices.snmpwalk", &mainConfig)
	if err != nil {
		return nil, err
	}
	if err = mainConfig.SetDefaults(conf.GetString("network_devices.namespace"), logger); err != nil {
		return nil, err
	}
	return &mainConfig, nil
}

// SetDefaults sets default values wherever possible, returning an error if
// any values are malformed.
func (mainConfig *SnmpwalkConfig) SetDefaults(namespace string, logger log.Component) error {
	if mainConfig.StopTimeout == 0 {
		mainConfig.StopTimeout = common.DefaultStopTimeout
	}

	return nil
}
