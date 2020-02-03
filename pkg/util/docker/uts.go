// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-2020 Datadog, Inc.

// +build docker

package docker

import (
	"errors"
	"fmt"
	"strings"

	"github.com/DataDog/datadog-agent/pkg/util/containers"
	"github.com/DataDog/datadog-agent/pkg/util/containers/providers"
	"github.com/docker/docker/api/types/container"
)

// GetAgentContainerUTSMode provides the UTS mode of the Agent container
// To get this info in an optimal way, consider calling util.GetAgentUTSMode instead to benefit from the cache
func GetAgentContainerUTSMode() (containers.UTSMode, error) {
	agentCID, _ := providers.ContainerImpl.GetAgentCID()
	return GetContainerUTSMode(agentCID)
}

// GetContainerUTSMode returns the UTS mode of a container
func GetContainerUTSMode(cid string) (containers.UTSMode, error) {
	du, err := GetDockerUtil()
	if err != nil {
		return containers.UnknownUTSMode, err
	}
	container, err := du.Inspect(cid, false)
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
