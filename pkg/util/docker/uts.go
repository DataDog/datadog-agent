// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build docker
// +build docker

package docker

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/DataDog/datadog-agent/pkg/util/cache"
	"github.com/DataDog/datadog-agent/pkg/util/containers"
	"github.com/DataDog/datadog-agent/pkg/util/containers/providers"
	"github.com/DataDog/datadog-agent/pkg/util/log"
	"github.com/docker/docker/api/types/container"
)

// GetAgentUTSMode retrieves from Docker the UTS mode of the Agent container
func GetAgentUTSMode(ctx context.Context) (containers.UTSMode, error) {
	cacheUTSModeKey := cache.BuildAgentKey("utsMode")
	if cacheUTSMode, found := cache.Cache.Get(cacheUTSModeKey); found {
		return cacheUTSMode.(containers.UTSMode), nil
	}

	log.Debugf("GetAgentUTSMode trying docker")
	utsMode, err := getContainerUTSMode(ctx)
	cache.Cache.Set(cacheUTSModeKey, utsMode, cache.NoExpiration)
	if err != nil {
		return utsMode, fmt.Errorf("could not detect agent UTS mode: %v", err)
	}
	log.Debugf("GetAgentUTSMode: using UTS mode from Docker: %s", utsMode)
	return utsMode, nil
}

// getContainerUTSMode returns the UTS mode of a container
func getContainerUTSMode(ctx context.Context) (containers.UTSMode, error) {
	cid, _ := providers.ContainerImpl().GetAgentCID()

	du, err := GetDockerUtil()
	if err != nil {
		return containers.UnknownUTSMode, err
	}
	container, err := du.Inspect(ctx, cid, false)
	if err != nil {
		return containers.UnknownUTSMode, err
	}
	return parseContainerUTSMode(container.HostConfig)
}

// parseContainerUTSMode returns the UTS mode of a container
func parseContainerUTSMode(hostConfig *container.HostConfig) (containers.UTSMode, error) {
	if hostConfig == nil {
		return containers.UnknownUTSMode, errors.New("the HostConfig field is nil")
	}
	mode := containers.UTSMode(hostConfig.UTSMode)
	switch mode {
	case containers.DefaultUTSMode:
		return containers.DefaultUTSMode, nil
	case containers.HostUTSMode:
		return containers.HostUTSMode, nil
	}
	if strings.HasPrefix(string(mode), containerModePrefix) {
		return mode, nil
	}
	return containers.UnknownUTSMode, fmt.Errorf("unknown UTS mode: %s", mode)
}
