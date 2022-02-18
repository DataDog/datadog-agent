// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2021-present Datadog, Inc.

//go:build docker && (linux || windows)
// +build docker
// +build linux windows

package docker

import (
	"context"
	"fmt"
	"os"
	"strconv"
	"time"

	"github.com/docker/docker/api/types"

	"github.com/DataDog/datadog-agent/pkg/config"
	"github.com/DataDog/datadog-agent/pkg/util/containers/v2/metrics/provider"
	"github.com/DataDog/datadog-agent/pkg/util/docker"
	"github.com/DataDog/datadog-agent/pkg/util/pointer"
	"github.com/DataDog/datadog-agent/pkg/workloadmeta"
)

const (
	dockerCollectorID = "docker"

	pidCacheGCInterval     = 60 * time.Second
	pidCacheFullRefreshKey = "refreshTime"
)

func init() {
	provider.GetProvider().RegisterCollector(provider.CollectorMetadata{
		ID: dockerCollectorID,
		// This collector has a lower priority than the system collector
		Priority: 1,
		Runtimes: []string{provider.RuntimeNameDocker},
		Factory: func() (provider.Collector, error) {
			return newDockerCollector()
		},
		DelegateCache: true,
	})
}

type dockerCollector struct {
	du            *docker.DockerUtil
	pidCache      *provider.Cache
	metadataStore workloadmeta.Store
}

func newDockerCollector() (*dockerCollector, error) {
	if !config.IsFeaturePresent(config.Docker) {
		return nil, provider.ErrPermaFail
	}

	du, err := docker.GetDockerUtil()
	if err != nil {
		return nil, provider.ConvertRetrierErr(err)
	}

	return &dockerCollector{
		du:            du,
		pidCache:      provider.NewCache(pidCacheGCInterval),
		metadataStore: workloadmeta.GetGlobalStore(),
	}, nil
}

func (d *dockerCollector) ID() string {
	return dockerCollectorID
}

// GetContainerStats returns stats by container ID.
func (d *dockerCollector) GetContainerStats(containerID string, cacheValidity time.Duration) (*provider.ContainerStats, error) {
	stats, err := d.stats(containerID)
	if err != nil {
		return nil, err
	}

	return convertContainerStats(&stats.Stats), nil
}

// GetContainerNetworkStats returns network stats by container ID.
func (d *dockerCollector) GetContainerNetworkStats(containerID string, cacheValidity time.Duration) (*provider.ContainerNetworkStats, error) {
	stats, err := d.stats(containerID)
	if err != nil {
		return nil, err
	}

	return convertNetworkStats(stats.Networks), nil
}

// GetContainerIDForPID returns the container ID for given PID
func (d *dockerCollector) GetContainerIDForPID(pid int, cacheValidity time.Duration) (string, error) {
	currentTime := time.Now()
	strPid := strconv.Itoa(pid)

	cID, found, _ := d.pidCache.Get(currentTime, strPid, cacheValidity)
	if found {
		return cID.(string), nil
	}

	if err := d.refreshPIDCache(currentTime, cacheValidity); err != nil {
		return "", err
	}

	// Use harcoded cacheValidity as input one could be 0
	cID, found, _ = d.pidCache.Get(currentTime, strPid, time.Second)
	if found {
		return cID.(string), nil
	}

	return "", nil
}

// GetSelfContainerID returns current process container ID
func (d *dockerCollector) GetSelfContainerID() (string, error) {
	cID, err := d.GetContainerIDForPID(os.Getpid(), pidCacheGCInterval)
	if err == nil && cID != "" {
		return cID, err
	}

	cID, err = d.GetContainerIDForPID(os.Getppid(), pidCacheGCInterval)
	if err == nil && cID != "" {
		return cID, err
	}

	return "", nil
}

// stats returns stats by container ID
func (d *dockerCollector) stats(containerID string) (*types.StatsJSON, error) {
	stats, err := d.du.GetContainerStats(context.TODO(), containerID)
	if err != nil {
		return nil, fmt.Errorf("failed to get container stats for %s: %w", containerID, err)
	}

	return stats, nil
}

func (d *dockerCollector) refreshPIDCache(currentTime time.Time, cacheValidity time.Duration) error {
	// If we've done a full refresh within cacheValidity, we do not trigger another full refresh
	// We're using the cache itself with a dedicated key pidCacheFullRefreshKey to know if
	// we need to perform a full refresh or not to seamlessly handle cacheValidity and cache GC.
	_, found, err := d.pidCache.Get(currentTime, pidCacheFullRefreshKey, cacheValidity)
	if found {
		return err
	}

	// Full refresh
	containers, err := d.metadataStore.ListContainers()
	if err != nil {
		d.pidCache.Store(currentTime, pidCacheFullRefreshKey, struct{}{}, err)
		return err
	}

	for _, container := range containers {
		if container.Runtime == workloadmeta.ContainerRuntimeDocker && container.PID != 0 {
			d.pidCache.Store(currentTime, strconv.Itoa(container.PID), container.ID, nil)
		}
	}

	d.pidCache.Store(currentTime, pidCacheFullRefreshKey, struct{}{}, nil)
	return nil
}

func convertNetworkStats(networkStats map[string]types.NetworkStats) *provider.ContainerNetworkStats {
	containerNetworkStats := &provider.ContainerNetworkStats{
		BytesSent:   pointer.Float64Ptr(0),
		BytesRcvd:   pointer.Float64Ptr(0),
		PacketsSent: pointer.Float64Ptr(0),
		PacketsRcvd: pointer.Float64Ptr(0),
		Interfaces:  make(map[string]provider.InterfaceNetStats),
	}

	for ifname, netStats := range networkStats {
		*containerNetworkStats.BytesSent += float64(netStats.TxBytes)
		*containerNetworkStats.BytesRcvd += float64(netStats.RxBytes)
		*containerNetworkStats.PacketsSent += float64(netStats.TxPackets)
		*containerNetworkStats.PacketsRcvd += float64(netStats.RxPackets)

		ifNetStats := provider.InterfaceNetStats{
			BytesSent:   pointer.UIntToFloatPtr(netStats.TxBytes),
			BytesRcvd:   pointer.UIntToFloatPtr(netStats.RxBytes),
			PacketsSent: pointer.UIntToFloatPtr(netStats.TxPackets),
			PacketsRcvd: pointer.UIntToFloatPtr(netStats.RxPackets),
		}
		containerNetworkStats.Interfaces[ifname] = ifNetStats
	}

	return containerNetworkStats
}
