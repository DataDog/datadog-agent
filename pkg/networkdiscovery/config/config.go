// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2020-present Datadog, Inc.

package config

import "github.com/DataDog/datadog-agent/pkg/networkdiscovery/common"

// NetworkDiscoveryConfig contains configuration for NetFlow collector.
type NetworkDiscoveryConfig struct {
	StopTimeout int `mapstructure:"stop_timeout"`
}

// ReadConfig builds and returns configuration from Agent configuration.
func ReadConfig() (*NetworkDiscoveryConfig, error) {
	var mainConfig NetworkDiscoveryConfig

	if mainConfig.StopTimeout == 0 {
		mainConfig.StopTimeout = common.DefaultStopTimeout
	}
	return &mainConfig, nil
}
