// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2020-present Datadog, Inc.

// Package impl todo Package config defines the configuration options for the snmpwalk services.
package impl

import (
	"github.com/DataDog/datadog-agent/comp/core/log"

	"github.com/DataDog/datadog-agent/comp/core/config"

	"github.com/DataDog/datadog-agent/comp/snmpwalk/common"
)

// ReadConfig builds and returns configuration from Agent configuration.
func ReadConfig(conf config.Component, logger log.Component) (*common.SnmpwalkConfig, error) {
	var mainConfig common.SnmpwalkConfig

	err := conf.UnmarshalKey("network_devices.snmpwalk", &mainConfig)
	if err != nil {
		return nil, err
	}
	if err = mainConfig.SetDefaults(conf.GetString("network_devices.namespace"), logger); err != nil {
		return nil, err
	}
	return &mainConfig, nil
}
