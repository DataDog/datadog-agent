// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

package networkconfigmanagementimpl

import (
	"github.com/DataDog/datadog-agent/comp/core/config"
	"github.com/DataDog/datadog-agent/pkg/config/structure"
)

// StoreConfig holds eviction-policy knobs for the local config store.
type StoreConfig struct {
	MinConfigsPerDevice    int   `mapstructure:"min_configs_per_device"`
	MaxConfigsPerDevice    int   `mapstructure:"max_configs_per_device"`
	MaxRawConfigStoreBytes int64 `mapstructure:"max_raw_config_store_bytes"`
}

// RollbackConfig holds the rollback feature configuration, including the local config store knobs.
type RollbackConfig struct {
	Enabled bool        `mapstructure:"enabled"`
	Store   StoreConfig `mapstructure:"store"`
}

// NcmConfig is the agent-side configuration for the NCM component, read from
// the network_devices.config_management subtree of the agent config.
type NcmConfig struct {
	Rollback RollbackConfig `mapstructure:"rollback"`
}

func newConfig(agentConfig config.Component) (*NcmConfig, error) {
	cfg := &NcmConfig{}
	if err := structure.UnmarshalKey(agentConfig, "network_devices.config_management", cfg); err != nil {
		return &NcmConfig{}, err
	}
	return cfg, nil
}
