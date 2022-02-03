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
	"time"

	"github.com/docker/docker/api/types"

	"github.com/DataDog/datadog-agent/pkg/config"
	"github.com/DataDog/datadog-agent/pkg/util"
	"github.com/DataDog/datadog-agent/pkg/util/containers/v2/metrics/provider"
	"github.com/DataDog/datadog-agent/pkg/util/docker"
)

const (
	dockerCollectorID = "docker"
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
	du *docker.DockerUtil
}

func newDockerCollector() (*dockerCollector, error) {
	if !config.IsFeaturePresent(config.Docker) {
		return nil, provider.ErrPermaFail
	}

	du, err := docker.GetDockerUtil()
	if err != nil {
		return nil, provider.ConvertRetrierErr(err)
	}

	return &dockerCollector{du: du}, nil
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

// stats returns stats by container ID
func (d *dockerCollector) stats(containerID string) (*types.StatsJSON, error) {
	stats, err := d.du.GetContainerStats(context.TODO(), containerID)
	if err != nil {
		return nil, fmt.Errorf("failed to get container stats for %s: %w", containerID, err)
	}

	return stats, nil
}

func convertNetworkStats(networkStats map[string]types.NetworkStats) *provider.ContainerNetworkStats {
	containerNetworkStats := &provider.ContainerNetworkStats{
		BytesSent:   util.Float64Ptr(0),
		BytesRcvd:   util.Float64Ptr(0),
		PacketsSent: util.Float64Ptr(0),
		PacketsRcvd: util.Float64Ptr(0),
		Interfaces:  make(map[string]provider.InterfaceNetStats),
	}

	for ifname, netStats := range networkStats {
		*containerNetworkStats.BytesSent += float64(netStats.TxBytes)
		*containerNetworkStats.BytesRcvd += float64(netStats.RxBytes)
		*containerNetworkStats.PacketsSent += float64(netStats.TxPackets)
		*containerNetworkStats.PacketsRcvd += float64(netStats.RxPackets)

		ifNetStats := provider.InterfaceNetStats{
			BytesSent:   util.UIntToFloatPtr(netStats.TxBytes),
			BytesRcvd:   util.UIntToFloatPtr(netStats.RxBytes),
			PacketsSent: util.UIntToFloatPtr(netStats.TxPackets),
			PacketsRcvd: util.UIntToFloatPtr(netStats.RxPackets),
		}
		containerNetworkStats.Interfaces[ifname] = ifNetStats
	}

	return containerNetworkStats
}
