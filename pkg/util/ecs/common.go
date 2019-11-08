// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2017 Datadog, Inc.

// +build docker

package ecs

import (
	"encoding/json"
	"net/http"
	"time"

	"github.com/DataDog/datadog-agent/pkg/util/containers"
	"github.com/DataDog/datadog-agent/pkg/util/containers/metrics"
	"github.com/DataDog/datadog-agent/pkg/util/ecs/metadata"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

const (
	metadataURL string = "http://169.254.170.2/v2/metadata"
	statsURL    string = "http://169.254.170.2/v2/stats"
	timeout            = 500 * time.Millisecond
)

// GetTaskMetadata extracts the metadata payload for the task the agent is in.
func GetTaskMetadata() (metadata.TaskMetadata, error) {
	return GetTaskMetadataWithURL(metadataURL)
}

// GetTaskMetadataWithURL extracts the metadata payload for a task given a metadata URL.
func GetTaskMetadataWithURL(url string) (metadata.TaskMetadata, error) {
	var meta metadata.TaskMetadata
	client := http.Client{
		Timeout: timeout,
	}
	resp, err := client.Get(url)
	if err != nil {
		return meta, err
	}
	defer resp.Body.Close()

	decoder := json.NewDecoder(resp.Body)
	err = decoder.Decode(&meta)
	if err != nil {
		log.Errorf("Decoding task metadata failed - %s", err)
	}
	return meta, err
}

// ListContainers returns all containers exposed by the ECS API and their metrics
func ListContainers() ([]*containers.Container, error) {
	var cList []*containers.Container

	ecsContainers, err := getECSContainers()
	if err != nil {
		log.Error("Unable to get the container list from ecs")
		return cList, err
	}
	for _, c := range ecsContainers {
		entityID := containers.BuildTaggerEntityName(c.DockerID)
		ctr := &containers.Container{
			Type:        "ECS",
			ID:          c.DockerID,
			EntityID:    entityID,
			Name:        c.DockerName,
			Image:       c.Image,
			ImageID:     c.ImageID,
			AddressList: metadata.ParseECSContainerNetworkAddresses(c.Ports, c.Networks, c.DockerName),
		}

		createdAt, err := time.Parse(time.RFC3339, c.CreatedAt)
		if err != nil {
			log.Errorf("Unable to determine creation time for container %s - %s", c.DockerID, err)
		} else {
			ctr.Created = createdAt.Unix()
		}
		startedAt, err := time.Parse(time.RFC3339, c.StartedAt)
		if err != nil {
			log.Errorf("Unable to determine creation time for container %s - %s", c.DockerID, err)
		} else {
			ctr.StartedAt = startedAt.Unix()
		}

		if l, found := c.Limits["cpu"]; found && l > 0 {
			ctr.CPULimit = float64(l)
		} else {
			ctr.CPULimit = 100
		}
		if l, found := c.Limits["memory"]; found && l > 0 {
			ctr.MemLimit = l
		}
		cList = append(cList, ctr)
	}
	err = UpdateContainerMetrics(cList)
	return cList, err
}

// UpdateContainerMetrics updates performance metrics for a provided list of Container objects
func UpdateContainerMetrics(cList []*containers.Container) error {
	for _, ctr := range cList {
		stats, err := getContainerStats(ctr.ID)
		if err != nil {
			log.Debugf("Unable to get stats from ECS for container %s: %s", ctr.ID, err)
			continue
		}
		// TODO: add metrics - complete for https://github.com/DataDog/datadog-process-agent/blob/970729924e6b2b6fe3a912b62657c297621723cc/checks/container_rt.go#L110-L128
		// start with a hack (translate ecs stats to docker cgroup stuff)
		// then support ecs stats natively
		cpu, mem, io, memLimit := convertECSStats(stats)
		ctr.CPU = &cpu
		ctr.Memory = &mem
		ctr.IO = &io
		if ctr.MemLimit == 0 {
			ctr.MemLimit = memLimit
		}
	}
	return nil
}

// getECSContainers returns all containers exposed by the ECS API as plain ECSContainers
func getECSContainers() ([]metadata.Container, error) {
	meta, err := GetTaskMetadata()
	if err != nil || len(meta.Containers) == 0 {
		log.Errorf("Unable to retrieve task metadata")
		return nil, err
	}
	return meta.Containers, nil
}

// getContainerStats retrives stats about a container from the ECS stats endpoint
func getContainerStats(id string) (ContainerStats, error) {
	var stats ContainerStats
	client := http.Client{
		Timeout: timeout,
	}
	resp, err := client.Get(statsURL + "/" + id)
	if err != nil {
		return stats, err
	}
	defer resp.Body.Close()

	decoder := json.NewDecoder(resp.Body)
	err = decoder.Decode(&stats)
	if err != nil {
		return stats, err
	}
	stats.IO.ReadBytes = computeIOStats(stats.IO.BytesPerDeviceAndKind, "Read")
	stats.IO.WriteBytes = computeIOStats(stats.IO.BytesPerDeviceAndKind, "Write")
	return stats, nil
}

// computeIOStats sums all values across devices for an operation kind.
func computeIOStats(ops []OPStat, kind string) uint64 {
	var res uint64
	for _, op := range ops {
		if op.Kind == kind {
			res += op.Value
		}
	}
	return res
}

// convertECSStats is responsible for converting ecs stats structs to docker style stats
// TODO: get rid of this by supporting ECS stats everywhere we use docker stats only.
func convertECSStats(stats ContainerStats) (metrics.CgroupTimesStat, metrics.CgroupMemStat, metrics.CgroupIOStat, uint64) {
	cpu := metrics.CgroupTimesStat{
		System:      stats.CPU.Usage.Kernelmode,
		User:        stats.CPU.Usage.Usermode,
		SystemUsage: stats.CPU.System,
	}
	mem := metrics.CgroupMemStat{
		RSS:             stats.Memory.Details.RSS,
		Cache:           stats.Memory.Details.Cache,
		Pgfault:         stats.Memory.Details.PgFault,
		MemUsageInBytes: stats.Memory.Details.Usage,
	}
	io := metrics.CgroupIOStat{
		ReadBytes:  stats.IO.ReadBytes,
		WriteBytes: stats.IO.WriteBytes,
	}
	return cpu, mem, io, stats.Memory.Details.Limit
}
