// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2020-present Datadog, Inc.

//go:build ec2

package aws

import (
	pkgconfigsetup "github.com/DataDog/datadog-agent/pkg/config/setup"
)

// AutodiscoveryConfig represents the auto-discovery configuration for database-monitoring
type AutodiscoveryConfig struct {
	Config Config
}

// Config represents the configuration for auto-discovering database clusters
type Config struct {
	Enabled           bool
	DiscoveryInterval int
	QueryTimeout      int
	Tags              []string
	DbmTag            string
	GlobalViewDbTag   string
	Region            string // auto-discovered from instance metadata
}

const (
	auroraAutoDiscoveryConfigKey = "database_monitoring.autodiscovery.aurora"
	rdsAutoDiscoveryConfigKey    = "database_monitoring.autodiscovery.rds"
)

// NewAuroraAutodiscoveryConfig parses configuration and returns a built Config
func NewAuroraAutodiscoveryConfig() (Config, error) {
	var discoveryConfigs Config
	// defaults for all values are set in the config package
	discoveryConfigs.Enabled = pkgconfigsetup.Datadog().GetBool(auroraAutoDiscoveryConfigKey + ".enabled")
	discoveryConfigs.QueryTimeout = pkgconfigsetup.Datadog().GetInt(auroraAutoDiscoveryConfigKey + ".query_timeout")
	discoveryConfigs.DiscoveryInterval = pkgconfigsetup.Datadog().GetInt(auroraAutoDiscoveryConfigKey + ".discovery_interval")
	discoveryConfigs.Tags = pkgconfigsetup.Datadog().GetStringSlice(auroraAutoDiscoveryConfigKey + ".tags")
	discoveryConfigs.DbmTag = pkgconfigsetup.Datadog().GetString(auroraAutoDiscoveryConfigKey + ".dbm_tag")
	discoveryConfigs.GlobalViewDbTag = pkgconfigsetup.Datadog().GetString(auroraAutoDiscoveryConfigKey + ".global_view_db_tag")
	discoveryConfigs.Region = pkgconfigsetup.Datadog().GetString(auroraAutoDiscoveryConfigKey + ".region")
	return discoveryConfigs, nil
}

func NewRdsAutodiscoveryConfig() (Config, error) {
	var discoveryConfigs Config
	// defaults for all values are set in the config package
	discoveryConfigs.Enabled = pkgconfigsetup.Datadog().GetBool(rdsAutoDiscoveryConfigKey + ".enabled")
	discoveryConfigs.QueryTimeout = pkgconfigsetup.Datadog().GetInt(rdsAutoDiscoveryConfigKey + ".query_timeout")
	discoveryConfigs.DiscoveryInterval = pkgconfigsetup.Datadog().GetInt(rdsAutoDiscoveryConfigKey + ".discovery_interval")
	discoveryConfigs.Tags = pkgconfigsetup.Datadog().GetStringSlice(rdsAutoDiscoveryConfigKey + ".tags")
	discoveryConfigs.DbmTag = pkgconfigsetup.Datadog().GetString(rdsAutoDiscoveryConfigKey + ".dbm_tag")
	discoveryConfigs.GlobalViewDbTag = pkgconfigsetup.Datadog().GetString(rdsAutoDiscoveryConfigKey + ".global_view_db_tag")
	discoveryConfigs.Region = pkgconfigsetup.Datadog().GetString(rdsAutoDiscoveryConfigKey + ".region")
	return discoveryConfigs, nil
}
