// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

//go:build linux && docker

// Package containers has utilities to work with GPU assignment to containers
package containers

import (
	"context"
	"fmt"
	"slices"
	"strconv"
	"strings"

	workloadmeta "github.com/DataDog/datadog-agent/comp/core/workloadmeta/def"
	"github.com/DataDog/datadog-agent/pkg/util/docker"
)

// getDockerVisibleDevicesEnvFromRuntime returns the value of the NVIDIA_VISIBLE_DEVICES environment variable by
// inspecting the container data
func getDockerVisibleDevicesEnvFromRuntime(container *workloadmeta.Container) (string, error) {
	dockerUtil, err := docker.GetDockerUtil()
	if err != nil {
		return "", fmt.Errorf("error getting docker util: %w", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), dockerInspectTimeout)
	defer cancel()

	containerInspect, err := dockerUtil.Inspect(ctx, container.ID, false)
	if err != nil {
		return "", fmt.Errorf("error inspecting container %s: %w", container.ID, err)
	}

	if containerInspect.HostConfig == nil {
		return "", fmt.Errorf("container %s has no host config", container.ID)
	}

	for _, device := range containerInspect.HostConfig.Resources.DeviceRequests {
		for _, capabilityGroup := range device.Capabilities {
			if slices.Contains(capabilityGroup, "gpu") {
				numGpus := device.Count
				if numGpus == -1 {
					return "all", nil
				}

				// return 0,1,...numGpus-1 as the assumed visible devices variable,
				// that's how Docker assigns devices to containers, there's no exclusive
				// allocation.
				visibleDevices := make([]string, numGpus)
				for i := 0; i < numGpus; i++ {
					visibleDevices[i] = strconv.Itoa(i)
				}
				return strings.Join(visibleDevices, ","), nil
			}
		}
	}

	return "", nil
}
