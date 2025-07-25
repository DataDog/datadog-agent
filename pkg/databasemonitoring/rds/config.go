// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2020-present Datadog, Inc.

// Package rds contains configuration for RDS autodiscovery
package rds

import pkgconfigsetup "github.com/DataDog/datadog-agent/pkg/config/setup"

const (
	autoDiscoveryConfigKey = "database_monitoring.autodiscovery.rds"
)

// AutodiscoveryConfig represents the auto-discovery configuration for database-monitoring
type AutodiscoveryConfig struct {
	Config Config
}

// Config represents the configuration for auto-discovering database clusters
type Config struct {
	Enabled           bool `json:"enabled"`
	DiscoveryInterval int  `json:"discovery_interval"`
	QueryTimeout      int  `json:"query_timeout"`
	Tags              []string `json:"tags"`
	DbmTag            string `json:"dbm_tag"`
	Region            string `json:"region"` // auto-discovered from instance metadata
}

// NewRdsAutodiscoveryConfig parses configuration and returns a built Config
func NewRdsAutodiscoveryConfig() (Config, error) {
	var discoveryConfigs Config
	// defaults for all values are set in the config package
	discoveryConfigs.Enabled = pkgconfigsetup.Datadog().GetBool(autoDiscoveryConfigKey + ".enabled")
	discoveryConfigs.QueryTimeout = pkgconfigsetup.Datadog().GetInt(autoDiscoveryConfigKey + ".query_timeout")
	discoveryConfigs.DiscoveryInterval = pkgconfigsetup.Datadog().GetInt(autoDiscoveryConfigKey + ".discovery_interval")
	discoveryConfigs.Tags = pkgconfigsetup.Datadog().GetStringSlice(autoDiscoveryConfigKey + ".tags")
	discoveryConfigs.DbmTag = pkgconfigsetup.Datadog().GetString(autoDiscoveryConfigKey + ".dbm_tag")
	discoveryConfigs.Region = pkgconfigsetup.Datadog().GetString(autoDiscoveryConfigKey + ".region")
	return discoveryConfigs, nil
}
