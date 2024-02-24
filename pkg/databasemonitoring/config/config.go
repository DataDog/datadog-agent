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
	autoDiscoveryConfigKey       = "database_monitoring.autodiscovery"
	autoDiscoveryAuroraConfigKey = "database_monitoring.autodiscovery.aurora"
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
	var discoveryConfigs AutodiscoveryConfig
	var auroraConfig AuroraConfig
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
	if err := coreconfig.Datadog.UnmarshalKey(autoDiscoveryConfigKey, &discoveryConfigs, opt); err != nil {
		return AuroraConfig{}, err
	}
	discoveryConfigs.AuroraConfig = auroraConfig
	if auroraConfig.RoleArn == "" {
		return auroraConfig, fmt.Errorf("invalid %s configuration, a role_arn must be set", autoDiscoveryAuroraConfigKey)
	}
	if auroraConfig.Region == "" {
		return auroraConfig, fmt.Errorf("invalid %s configuration configuration, a region must set", autoDiscoveryAuroraConfigKey)
	}
	// check all types are valid
	for i := range auroraConfig.Clusters {
		intType := auroraConfig.Clusters[i].Type
		if !IsValidIntegrationType(intType) {
			return auroraConfig, fmt.Errorf("invalid integration type in %s.clusters configuration: %s", autoDiscoveryAuroraConfigKey, intType)
		}
	}

	return auroraConfig, nil
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
