// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-Present Datadog, Inc.

package cloudfoundry

import (
	"fmt"
	"net"
	"time"

	"code.cloudfoundry.org/garden"
	"github.com/DataDog/datadog-agent/pkg/util/containers"
	"github.com/DataDog/datadog-agent/pkg/util/containers/providers"
	"github.com/DataDog/datadog-agent/pkg/util/log"

	// Needs ContainerImpl, so make sure it is init
	_ "github.com/DataDog/datadog-agent/pkg/util/containers/providers/cgroup"
)

// GetGardenContainers returns the list of running containers from the local garden API
func (gu *GardenUtil) GetGardenContainers() ([]garden.Container, error) {
	return gu.cli.Containers(nil)
}

// ListContainers returns the list of running containers and populates their metrics and metadata
func (gu *GardenUtil) ListContainers() ([]*containers.Container, error) {
	gardenContainers, err := gu.GetGardenContainers()
	if err != nil {
		return nil, fmt.Errorf("error listing garden containers: %v", err)
	}

	var cList = make([]*containers.Container, len(gardenContainers))
	handles := make([]string, len(gardenContainers))
	for i, gardenContainer := range gardenContainers {
		handles[i] = gardenContainer.Handle()
	}
	gardenContainerInfo, err := gu.cli.BulkInfo(handles)
	if err != nil {
		return nil, fmt.Errorf("error getting info for garden containers: %v", err)
	}
	gardenContainerMetrics, err := gu.cli.BulkMetrics(handles)
	if err != nil {
		return nil, fmt.Errorf("error getting metrics for garden containers: %v", err)
	}

	for i, handle := range handles {
		infoEntry := gardenContainerInfo[handle]
		if err := infoEntry.Err; err != nil {
			log.Debugf("could not get info for container %s: %v", handle, err)
			continue
		}
		metricsEntry := gardenContainerMetrics[handle]
		if err := metricsEntry.Err; err != nil {
			log.Debugf("could not get metrics for container %s: %v", handle, err)
			continue
		}
		container := containers.Container{
			Type:        "garden",
			ID:          handle,
			EntityID:    containers.BuildTaggerEntityName(handle),
			State:       infoEntry.Info.State,
			Excluded:    false,
			Created:     time.Now().Add(-metricsEntry.Metrics.Age).Unix(),
			AddressList: parseContainerPorts(infoEntry.Info),
		}
		cList[i] = &container
	}

	if err := providers.ContainerImpl().Prefetch(); err != nil {
		return nil, fmt.Errorf("could not fetch container metrics: %s", err)
	}
	if err != nil {
		return nil, fmt.Errorf("could not get cgroups: %s", err)
	}
	for _, container := range cList {
		if container.State != containers.ContainerActiveState || !providers.ContainerImpl().ContainerExists(container.ID) {
			log.Debugf("Container %s not in state %s, skipping", container.ID[:12], containers.ContainerActiveState)
			continue
		}
		limits, err := providers.ContainerImpl().GetContainerLimits(container.ID)
		if err != nil {
			log.Debugf("Cannot get limits for container %s: %s, skipping", container.ID[:12], err)
			continue
		}
		container.SetLimits(limits)
	}
	err = gu.UpdateContainerMetrics(cList)
	return cList, err
}

// UpdateContainerMetrics updates the metric for a list of containers
func (gu *GardenUtil) UpdateContainerMetrics(cList []*containers.Container) error {
	if err := providers.ContainerImpl().Prefetch(); err != nil {
		return fmt.Errorf("could not fetch container metrics: %s", err)
	}
	for _, container := range cList {
		if container.State != containers.ContainerActiveState || !providers.ContainerImpl().ContainerExists(container.ID) {
			log.Debugf("Container %s not in state %s, skipping", container.ID[:12], containers.ContainerActiveState)
			continue
		}

		networkMetrics, err := providers.ContainerImpl().GetNetworkMetrics(container.ID, map[string]string{})
		if err != nil {
			log.Debugf("Cannot get metrics for container %s: %s", container.ID[:12], err)
			continue
		}
		container.Network = networkMetrics
		metrics, err := providers.ContainerImpl().GetContainerMetrics(container.ID)
		if err != nil {
			log.Debugf("Cannot get network metrics for container %s: %s", container.ID[:12], err)
			continue
		}
		container.SetMetrics(metrics)
	}
	return nil
}

func parseContainerPorts(info garden.ContainerInfo) []containers.NetworkAddress {
	var addresses = make([]containers.NetworkAddress, len(info.MappedPorts))
	for i, port := range info.MappedPorts {
		addresses[i] = containers.NetworkAddress{
			IP:       net.ParseIP(info.ExternalIP),
			Port:     int(port.HostPort),
			Protocol: "tcp",
		}
	}
	return addresses
}
