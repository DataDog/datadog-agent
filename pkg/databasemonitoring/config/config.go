// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2020-present Datadog, Inc.

// Package config contains database-monitoring auto-discovery configuration
package config

import coreconfig "github.com/DataDog/datadog-agent/pkg/config"

const (
	autoDiscoveryAuroraConfigKey = "database_monitoring.autodiscovery.aurora"
)

// AutodiscoveryConfig represents the auto-discovery configuration for database-monitoring
type AutodiscoveryConfig struct {
	AuroraConfig AuroraConfig
}

// AuroraConfig represents the configuration for auto-discovering database clusters
type AuroraConfig struct {
	Enabled           bool
	DiscoveryInterval int
	QueryTimeout      int
	Tags              []string
	Region            string // auto-discovered from instance metadata
}

// NewAuroraAutodiscoveryConfig parses configuration and returns a built AuroraConfig
func NewAuroraAutodiscoveryConfig() (AuroraConfig, error) {
	var discoveryConfigs AuroraConfig
	// defaults for all values are set in the config package
	discoveryConfigs.Enabled = coreconfig.Datadog.GetBool(autoDiscoveryAuroraConfigKey + ".enabled")
	discoveryConfigs.QueryTimeout = coreconfig.Datadog.GetInt(autoDiscoveryAuroraConfigKey + ".query_timeout")
	discoveryConfigs.DiscoveryInterval = coreconfig.Datadog.GetInt(autoDiscoveryAuroraConfigKey + ".discovery_interval")
	discoveryConfigs.Tags = coreconfig.Datadog.GetStringSlice(autoDiscoveryAuroraConfigKey + ".tags")
	return discoveryConfigs, nil
}
