// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2017-2020 Datadog, Inc.

// +build docker

package ecs

import (
	"net"
	"time"

	"github.com/DataDog/datadog-agent/pkg/util/containers"
	"github.com/DataDog/datadog-agent/pkg/util/containers/metrics"
	"github.com/DataDog/datadog-agent/pkg/util/ecs/metadata"
	"github.com/DataDog/datadog-agent/pkg/util/log"

	v2 "github.com/DataDog/datadog-agent/pkg/util/ecs/metadata/v2"
)

const (
	// cpuKey represents the cpu key used in the resource limits map returned by the ECS API
	cpuKey = "CPU"
	// memoryKey represents the memory key used in the resource limits map returned by the ECS API
	memoryKey = "Memory"
)

// ListContainersInCurrentTask returns internal container representations (with
// their metrics) for the current task by collecting that information from the
// ECS metadata v2 API.
func ListContainersInCurrentTask() ([]*containers.Container, error) {
	var cList []*containers.Container

	client, err := metadata.V2()
	if err != nil {
		log.Debugf("error while initializing ECS metadata V2 client: %s", err)
		return cList, err
	}

	task, err := client.GetTask()
	if err != nil || len(task.Containers) == 0 {
		log.Error("Unable to get the container list from ecs")
		return cList, err
	}

	filter, err := containers.GetSharedMetricFilter()
	if err != nil {
		log.Warnf("Unable to get container filter. All containers in ECS Task will be processed, err: %v", err)
	}

	for _, c := range task.Containers {
		// Not using c.DockerName as it's generated with ecs task name, thus probably not easy to match
		if filter == nil || !filter.IsExcluded(c.Name, c.Image, "") {
			cList = append(cList, convertMetaV2Container(c, task.Limits))
		}
	}

	err = UpdateContainerMetrics(cList)
	return cList, err
}

// UpdateContainerMetrics updates performance metrics for a list of internal
// container representations based on stats collected from the ECS metadata v2 API
func UpdateContainerMetrics(cList []*containers.Container) error {
	for _, ctr := range cList {
		client, err := metadata.V2()
		if err != nil {
			log.Debugf("error while initializing ECS metadata V2 client: %s", err)
			return err
		}

		stats, err := client.GetContainerStats(ctr.ID)
		if err != nil {
			log.Debugf("Unable to get stats from ECS for container %s: %s", ctr.ID, err)
			continue
		}

		stats.IO.ReadBytes = sumStats(stats.IO.BytesPerDeviceAndKind, "Read")
		stats.IO.WriteBytes = sumStats(stats.IO.BytesPerDeviceAndKind, "Write")

		// TODO: add metrics - complete for https://github.com/DataDog/datadog-process-agent/blob/970729924e6b2b6fe3a912b62657c297621723cc/checks/container_rt.go#L110-L128
		// start with a hack (translate ecs stats to docker cgroup stuff)
		// then support ecs stats natively
		cm, memLimit := convertMetaV2ContainerStats(stats)
		ctr.SetMetrics(&cm)
		if ctr.Limits.MemLimit == 0 {
			ctr.Limits.MemLimit = memLimit
		}
	}
	return nil
}

// convertMetaV2Container returns an internal container representation from an
// ECS metadata v2 container object.
func convertMetaV2Container(c v2.Container, taskLimits map[string]float64) *containers.Container {
	container := &containers.Container{
		Type:        "ECS",
		ID:          c.DockerID,
		EntityID:    containers.BuildTaggerEntityName(c.DockerID),
		Name:        c.DockerName,
		Image:       c.Image,
		ImageID:     c.ImageID,
		AddressList: parseContainerNetworkAddresses(c.Ports, c.Networks, c.DockerName),
	}

	createdAt, err := time.Parse(time.RFC3339, c.CreatedAt)
	if err != nil {
		log.Errorf("Unable to determine creation time for container %s - %s", c.DockerID, err)
	} else {
		container.Created = createdAt.Unix()
	}
	startedAt, err := time.Parse(time.RFC3339, c.StartedAt)
	if err != nil {
		log.Errorf("Unable to determine creation time for container %s - %s", c.DockerID, err)
	} else {
		container.StartedAt = startedAt.Unix()
	}

	if l, found := c.Limits[cpuKey]; found && l > 0 {
		container.Limits.CPULimit = formatContainerCPULimit(float64(l))
	} else if l, found := taskLimits[cpuKey]; found && l > 0 {
		container.Limits.CPULimit = formatTaskCPULimit(l)
	} else {
		container.Limits.CPULimit = 100
	}

	if l, found := c.Limits[memoryKey]; found && l > 0 {
		container.Limits.MemLimit = formatMemoryLimit(l)
	} else if l, found := taskLimits[memoryKey]; found && l > 0 {
		container.Limits.MemLimit = formatMemoryLimit(uint64(l))
	}

	return container
}

func formatContainerCPULimit(val float64) float64 {
	// The ECS API exposes the container CPU limit in CPU units
	// Value is reported in Hz
	return val / 1024 * 100
}

func formatTaskCPULimit(val float64) float64 {
	// The ECS API exposes the task CPU limit with the format: 0.25, 0.5, 1, 2, 4
	// Value is reported in Hz
	return val * 100
}

func formatMemoryLimit(val uint64) uint64 {
	// The ECS API exposes the memory limit is in MB
	return val * 1000000
}

// convertMetaV2Container returns internal metrics representations from an ECS
// metadata v2 container stats object.
func convertMetaV2ContainerStats(s *v2.ContainerStats) (metrics.ContainerMetrics, uint64) {
	return metrics.ContainerMetrics{
		CPU: &metrics.ContainerCPUStats{
			User:        s.CPU.Usage.Usermode,
			System:      s.CPU.Usage.Kernelmode,
			SystemUsage: s.CPU.System,
		},
		Memory: &metrics.ContainerMemStats{
			Cache:           s.Memory.Details.Cache,
			MemUsageInBytes: s.Memory.Usage,
			Pgfault:         s.Memory.Details.PgFault,
			RSS:             s.Memory.Details.RSS,
		},
		IO: &metrics.ContainerIOStats{
			ReadBytes:  s.IO.ReadBytes,
			WriteBytes: s.IO.WriteBytes,
		},
	}, s.Memory.Limit
}

// parseContainerNetworkAddresses converts ECS container ports
// and networks into a list of NetworkAddress
func parseContainerNetworkAddresses(ports []v2.Port, networks []v2.Network, container string) []containers.NetworkAddress {
	addrList := []containers.NetworkAddress{}
	if networks == nil {
		log.Debugf("No network settings available in ECS metadata")
		return addrList
	}
	for _, network := range networks {
		for _, addr := range network.IPv4Addresses { // one-element list
			IP := net.ParseIP(addr)
			if IP == nil {
				log.Warnf("Unable to parse IP: %v for container: %s", addr, container)
				continue
			}
			if len(ports) > 0 {
				// Ports is not nil, get ports and protocols
				for _, port := range ports {
					addrList = append(addrList, containers.NetworkAddress{
						IP:       IP,
						Port:     int(port.ContainerPort),
						Protocol: port.Protocol,
					})
				}
			} else {
				// Ports is nil (omitted by the ecs api if there are no ports exposed).
				// Keep the container IP anyway.
				addrList = append(addrList, containers.NetworkAddress{
					IP: IP,
				})
			}
		}
	}
	return addrList
}

// sumStats adds up values across devices for an operation kind.
func sumStats(ops []v2.OPStat, kind string) uint64 {
	var res uint64
	for _, op := range ops {
		if op.Kind == kind {
			res += op.Value
		}
	}
	return res
}
