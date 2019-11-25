// +build docker

package util

import (
	"fmt"

	"github.com/DataDog/datadog-agent/pkg/util/cache"
	"github.com/DataDog/datadog-agent/pkg/util/docker"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

// GetAgentNetworkMode retrieves from Docker the network mode of the Agent container
func GetAgentNetworkMode() (string, error) {
	cacheNetworkModeKey := cache.BuildAgentKey("networkMode")
	if cacheNetworkMode, found := cache.Cache.Get(cacheNetworkModeKey); found {
		return cacheNetworkMode.(string), nil
	}

	log.Debugf("GetAgentNetworkMode trying Docker")
	networkMode, err := docker.GetAgentContainerNetworkMode()
	cache.Cache.Set(cacheNetworkModeKey, networkMode, cache.NoExpiration)
	if err != nil {
		return networkMode, fmt.Errorf("could not detect agent network mode: %v", err)
	}
	log.Debugf("GetAgentNetworkMode: using network mode from Docker: %s", networkMode)
	return networkMode, nil
}
