// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2020-present Datadog, Inc.

// Package config contains database-monitoring auto-discovery configuration
package config

import (
	"fmt"
	coreconfig "github.com/DataDog/datadog-agent/pkg/config"
	"github.com/DataDog/viper"
	"reflect"
)

const (
	autoDiscoveryAuroraConfigKey = "database_monitoring.autodiscovery.aurora"
	defaultDiscoveryInterval     = 300
	defaultQueryTimeout          = 10
)

// IntegrationType represents the type of database-monitoring integration
type IntegrationType string

const (
	// Postgres represents the Postgres database-monitoring integration type
	Postgres IntegrationType = "postgres"

	// MySQL represents the MySQL database-monitoring integration type
	MySQL IntegrationType = "mysql"
	// SQLServer represents the SQLServer database-monitoring integration type
	SQLServer IntegrationType = "sqlserver"
)

// AutodiscoveryConfig represents the auto-discovery configuration for database-monitoring
type AutodiscoveryConfig struct {
	AuroraConfig AuroraConfig `mapstructure:"aurora"`
}

// AuroraConfig represents the configuration for auto-discovering database clusters
type AuroraConfig struct {
	DiscoveryInterval int              `mapstructure:"discovery_interval"`
	QueryTimeout      int              `mapstructure:"query_timeout"`
	Region            string           `mapstructure:"region"`
	RoleArn           string           `mapstructure:"role_arn"`
	Clusters          []ClustersConfig `mapstructure:"clusters"`
}

// ClustersConfig represents the list of clusters for a specific integration type
type ClustersConfig struct {
	Type       IntegrationType `mapstructure:"type"`
	ClusterIds []string        `mapstructure:"db-cluster-ids,omitempty"`
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
	if err := coreconfig.Datadog.UnmarshalKey(autoDiscoveryAuroraConfigKey, &discoveryConfigs, opt); err != nil {
		return AuroraConfig{}, err
	}
	if discoveryConfigs.RoleArn == "" {
		return discoveryConfigs, fmt.Errorf("invalid %s configuration, a role_arn must be set", autoDiscoveryAuroraConfigKey)
	}
	if discoveryConfigs.Region == "" {
		return discoveryConfigs, fmt.Errorf("invalid %s configuration configuration, a region must set", autoDiscoveryAuroraConfigKey)
	}
	// check all types are valid
	for i := range discoveryConfigs.Clusters {
		intType := discoveryConfigs.Clusters[i].Type
		if !IsValidIntegrationType(intType) {
			return discoveryConfigs, fmt.Errorf("invalid integration type in %s.clusters configuration: %s", autoDiscoveryAuroraConfigKey, intType)
		}
	}

	return discoveryConfigs, nil
}

// IsValidIntegrationType checks if the given database type is valid
func IsValidIntegrationType(dbType IntegrationType) bool {
	switch dbType {
	case Postgres:
		return true
	case MySQL:
		return true
	case SQLServer:
		return true
	default:
		return false
	}
}
