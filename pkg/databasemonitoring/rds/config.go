// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2020-present Datadog, Inc.

package rds

import pkgconfigsetup "github.com/DataDog/datadog-agent/pkg/config/setup"

const (
	autoDiscoveryRdsConfigKey = "database_monitoring.autodiscovery.rds"
)

// AutodiscoveryConfig represents the auto-discovery configuration for database-monitoring
type AutodiscoveryConfig struct {
	RdsConfig RdsConfig
}

// RdsConfig represents the configuration for auto-discovering database clusters
type RdsConfig struct {
	Enabled           bool
	DiscoveryInterval int
	QueryTimeout      int
	Tags              []string
	DbmTag            string
	Region            string // auto-discovered from instance metadata
}

// NewRdsAutodiscoveryConfig parses configuration and returns a built RdsConfig
func NewRdsAutodiscoveryConfig() (RdsConfig, error) {
	var discoveryConfigs RdsConfig
	// defaults for all values are set in the config package
	discoveryConfigs.Enabled = pkgconfigsetup.Datadog().GetBool(autoDiscoveryRdsConfigKey + ".enabled")
	discoveryConfigs.QueryTimeout = pkgconfigsetup.Datadog().GetInt(autoDiscoveryRdsConfigKey + ".query_timeout")
	discoveryConfigs.DiscoveryInterval = pkgconfigsetup.Datadog().GetInt(autoDiscoveryRdsConfigKey + ".discovery_interval")
	discoveryConfigs.Tags = pkgconfigsetup.Datadog().GetStringSlice(autoDiscoveryRdsConfigKey + ".tags")
	discoveryConfigs.DbmTag = pkgconfigsetup.Datadog().GetString(autoDiscoveryRdsConfigKey + ".dbm_tag")
	discoveryConfigs.Region = pkgconfigsetup.Datadog().GetString(autoDiscoveryRdsConfigKey + ".region")
	return discoveryConfigs, nil
}
