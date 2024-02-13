// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2020-present Datadog, Inc.

// Package config contains database-monitoring auto-discovery configuration
package config

import (
	coreconfig "github.com/DataDog/datadog-agent/pkg/config"
	"github.com/DataDog/viper"
	"reflect"
)

const (
	autoDiscoveryAuroraConfigKey = "database_monitoring.autodiscovery.aurora"
	defaultDiscoveryInterval     = 300
	defaultQueryTimeout          = 10
	defaultEnabled               = false
	defaultClusterTag            = "datadoghq.com/scrape:true"
)

// AutodiscoveryConfig represents the auto-discovery configuration for database-monitoring
type AutodiscoveryConfig struct {
	AuroraConfig AuroraConfig `mapstructure:"aurora"`
}

// AuroraConfig represents the configuration for auto-discovering database clusters
type AuroraConfig struct {
	Enabled           bool     `mapstructure:"enabled"`
	DiscoveryInterval int      `mapstructure:"discovery_interval"`
	QueryTimeout      int      `mapstructure:"query_timeout"`
	Tags              []string `mapstructure:"tags"`
	Region            string   // auto-discovered from instance metadata
}

// NewAuroraAutodiscoveryConfig parses configuration and returns a built AuroraConfig
func NewAuroraAutodiscoveryConfig() (AuroraConfig, error) {
	var discoveryConfigs AuroraConfig
	opt := viper.DecodeHook(
		func(rf reflect.Kind, rt reflect.Kind, data interface{}) (interface{}, error) {
			// Turn an array into a map for ignored addresses
			if rf != reflect.Slice {
				return data, nil
			}
			if rt != reflect.Map {
				return data, nil
			}
			newData := map[interface{}]bool{}
			for _, i := range data.([]interface{}) {
				newData[i] = true
			}
			return newData, nil
		},
	)
	discoveryConfigs.DiscoveryInterval = defaultDiscoveryInterval
	discoveryConfigs.QueryTimeout = defaultQueryTimeout
	discoveryConfigs.Enabled = defaultEnabled
	if err := coreconfig.Datadog.UnmarshalKey(autoDiscoveryAuroraConfigKey, &discoveryConfigs, opt); err != nil {
		return AuroraConfig{}, err
	}
	if len(discoveryConfigs.Tags) == 0 {
		discoveryConfigs.Tags = []string{defaultClusterTag}
	}

	return discoveryConfigs, nil
}
