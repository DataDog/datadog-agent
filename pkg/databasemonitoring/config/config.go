package config

import (
	"fmt"
	coreconfig "github.com/DataDog/datadog-agent/pkg/config"
	"github.com/DataDog/viper"
	"reflect"
)

const (
	defaultDiscoveryIntervalSeconds = 300
	autoDiscoveryConfigKey          = "database_monitoring.autodiscover_aurora_clusters"
)

// IntegrationType represents the type of database-monitoring integration
type IntegrationType string

const (
	Postgres IntegrationType = "postgres"
)

// AutodiscoverClustersConfig represents the configuration for auto-discovering database clusters
type AutodiscoverClustersConfig struct {
	DiscoveryInterval int              `mapstructure:"discovery_interval"`
	RoleArn           string           `mapstructure:"role_arn"`
	Clusters          []ClustersConfig `mapstructure:"clusters"`
}

// ClustersConfig represents the list of clusters for a specific integration type
type ClustersConfig struct {
	Type       IntegrationType `mapstructure:"type"`
	Region     string          `mapstructure:"region"`
	ClusterIds []string        `mapstructure:"db-cluster-ids,omitempty"`
}

// NewAutodiscoverClustersConfig parses configuration and returns a built AutodiscoverClustersConfig
func NewAutodiscoverClustersConfig() (AutodiscoverClustersConfig, error) {
	var discoveryConfigs AutodiscoverClustersConfig
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
	// Set defaults before unmarshalling
	discoveryConfigs.DiscoveryInterval = defaultDiscoveryIntervalSeconds
	if err := coreconfig.Datadog.UnmarshalKey(autoDiscoveryConfigKey, &discoveryConfigs, opt); err != nil {
		return discoveryConfigs, err
	}
	if discoveryConfigs.RoleArn == "" {
		return discoveryConfigs, fmt.Errorf("invalid %s configuration, a role_arn must be set", autoDiscoveryConfigKey)
	}
	// check all types are valid
	for i := range discoveryConfigs.Clusters {
		intType := discoveryConfigs.Clusters[i].Type
		if !IsValidIntegrationType(intType) {
			return discoveryConfigs, fmt.Errorf("invalid integration type in %s configuration: %s", autoDiscoveryConfigKey, intType)
		}
		if discoveryConfigs.Clusters[i].Region == "" {
			return discoveryConfigs, fmt.Errorf("invalid %s.clusters configuration, a region must set", autoDiscoveryConfigKey)
		}
	}

	return discoveryConfigs, nil
}

// IsValidIntegrationType checks if the given database type is valid
func IsValidIntegrationType(dbType IntegrationType) bool {
	switch dbType {
	case Postgres:
		return true
	default:
		return false
	}
}
