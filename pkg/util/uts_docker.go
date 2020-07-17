// +build docker

package util

import (
	"fmt"

	"github.com/DataDog/datadog-agent/pkg/util/cache"
	"github.com/DataDog/datadog-agent/pkg/util/containers"
	"github.com/DataDog/datadog-agent/pkg/util/docker"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

// GetAgentUTSMode retrieves from Docker the UTS mode of the Agent container
func GetAgentUTSMode() (containers.UTSMode, error) {
	cacheUTSModeKey := cache.BuildAgentKey("utsMode")
	if cacheUTSMode, found := cache.Cache.Get(cacheUTSModeKey); found {
		return cacheUTSMode.(containers.UTSMode), nil
	}

	log.Debugf("GetAgentUTSMode trying docker")
	utsMode, err := docker.GetAgentContainerUTSMode()
	cache.Cache.Set(cacheUTSModeKey, utsMode, cache.NoExpiration)
	if err != nil {
		return utsMode, fmt.Errorf("could not detect agent UTS mode: %v", err)
	}
	log.Debugf("GetAgentUTSMode: using UTS mode from Docker: %s", utsMode)
	return utsMode, nil
}
