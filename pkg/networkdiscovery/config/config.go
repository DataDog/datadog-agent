// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2020-present Datadog, Inc.

package config

import (
	coreconfig "github.com/DataDog/datadog-agent/pkg/config"
	"github.com/DataDog/datadog-agent/pkg/networkdiscovery/common"
)

// NetworkDiscoveryConfig contains configuration for NetFlow collector.
type NetworkDiscoveryConfig struct {
	// TODO: Declare configs in agent config.go
	StopTimeout           int `mapstructure:"stop_timeout"`
	MinCollectionInterval int `mapstructure:"min_collection_interval"`

	IPAddress       string `mapstructure:"ip_address"`
	Port            int    `mapstructure:"port"`
	CommunityString string `mapstructure:"community_string"`
	SnmpVersion     string `mapstructure:"snmp_version"`
	Timeout         int    `mapstructure:"timeout"`
	Retries         int    `mapstructure:"retries"`
	User            string `mapstructure:"user"`
	AuthProtocol    string `mapstructure:"authProtocol"`
	AuthKey         string `mapstructure:"authKey"`
	PrivProtocol    string `mapstructure:"privProtocol"`
	PrivKey         string `mapstructure:"privKey"`
	ContextName     string `mapstructure:"context_name"`
	OidBatchSize    int    `mapstructure:"oid_batch_size"`
}

// ReadConfig builds and returns configuration from Agent configuration.
func ReadConfig() (*NetworkDiscoveryConfig, error) {
	var mainConfig NetworkDiscoveryConfig

	err := coreconfig.Datadog.UnmarshalKey("network_devices.discovery", &mainConfig)
	if err != nil {
		return nil, err
	}
	if mainConfig.StopTimeout == 0 {
		mainConfig.StopTimeout = common.DefaultStopTimeout
	}
	if mainConfig.MinCollectionInterval == 0 {
		mainConfig.MinCollectionInterval = common.DefaultMinCollectionInterval
	}
	if mainConfig.OidBatchSize == 0 {
		mainConfig.OidBatchSize = common.DefautOidBatchSize
	}

	if mainConfig.Timeout == 0 {
		mainConfig.Timeout = 5
	}

	if mainConfig.Retries == 0 {
		mainConfig.Retries = 3
	}

	return &mainConfig, nil
}
