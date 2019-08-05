package util

import (
	"fmt"

	"github.com/DataDog/datadog-agent/pkg/config"
	"github.com/DataDog/datadog-agent/pkg/util/cache"
	"github.com/DataDog/datadog-agent/pkg/util/ec2"
	"github.com/DataDog/datadog-agent/pkg/util/gce"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

// GetNetworkID retrieves the network_id which can be used to improve network
// connection resolution. This can be configured or detected.  The
// following sources will be queried:
// * configuration
// * GCE
// * EC2
func GetNetworkID() (string, error) {
	cacheNetworkIDKey := cache.BuildAgentKey("networkID")
	if cacheNetworkID, found := cache.Cache.Get(cacheNetworkIDKey); found {
		return cacheNetworkID.(string), nil
	}

	// the the id from configuration
	if networkID := config.Datadog.GetString("network.id"); networkID != "" {
		cache.Cache.Set(cacheNetworkIDKey, networkID, cache.NoExpiration)
		return networkID, nil
	}

	log.Debugf("GetNetworkID trying GCE")
	if networkID, err := gce.GetNetworkID(); err != nil {
		cache.Cache.Set(cacheNetworkIDKey, networkID, cache.NoExpiration)
		return networkID, nil
	}

	log.Debugf("GetNetworkID trying EC2")
	if networkID, err := ec2.GetNetworkID(); err != nil {
		cache.Cache.Set(cacheNetworkIDKey, networkID, cache.NoExpiration)
		return networkID, nil
	}

	return "", fmt.Errorf("could not detect network ID")
}
