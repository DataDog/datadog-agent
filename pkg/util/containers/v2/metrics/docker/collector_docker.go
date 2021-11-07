// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2021-present Datadog, Inc.

//go:build docker && (linux || windows)
// +build docker,linux docker,windows

package docker

import (
	"context"
	"fmt"
	"time"

	"github.com/docker/docker/api/types"

	"github.com/DataDog/datadog-agent/pkg/config"
	"github.com/DataDog/datadog-agent/pkg/util"
	"github.com/DataDog/datadog-agent/pkg/util/containers/v2/metrics"
	"github.com/DataDog/datadog-agent/pkg/util/docker"
	"github.com/DataDog/datadog-agent/pkg/util/retry"
)

const (
	dockerCollectorID = "docker"
)

func init() {
	metrics.GetProvider().RegisterCollector(metrics.CollectorMetadata{
		ID: dockerCollectorID,
		// This collector has a lower priority than the system collector
		Priority: 1,
		Runtimes: []string{metrics.RuntimeNameDocker},
		Factory: func() (metrics.Collector, error) {
			return newDockerCollector()
		},
	})
}

type dockerCollector struct {
	du *docker.DockerUtil
}

func newDockerCollector() (*dockerCollector, error) {
	if !config.IsFeaturePresent(config.Docker) {
		return nil, metrics.ErrPermaFail
	}

	du, err := docker.GetDockerUtil()
	if err != nil {
		if retry.IsErrPermaFail(err) {
			return nil, metrics.ErrPermaFail
		}
		return nil, metrics.ErrNothingYet
	}

	return &dockerCollector{
		du: du,
	}, nil
}

func (d *dockerCollector) ID() string {
	return dockerCollectorID
}

// GetContainerStats returns stats by container ID.
func (d *dockerCollector) GetContainerStats(containerID string, caccheValidity time.Duration) (*metrics.ContainerStats, error) {
	ctx := context.TODO()
	stats, err := d.du.GetContainerStats(ctx, containerID)
	if err != nil {
		return nil, fmt.Errorf("failed to get container stats for %s: %w", containerID, err)
	}

	return convertContainerStats(&stats.Stats), nil
}

// GetContainerNetworkStats returns network stats by container ID.
func (d *dockerCollector) GetContainerNetworkStats(containerID string, cacheValidity time.Duration, networks map[string]string) (*metrics.ContainerNetworkStats, error) {
	ctx := context.TODO()
	stats, err := d.du.GetContainerStats(ctx, containerID)
	if err == nil {
	}

	return convertNetworkStats(stats.Networks), nil
}

func convertNetworkStats(networkStats map[string]types.NetworkStats) *metrics.ContainerNetworkStats {
	containerNetworkStats := &metrics.ContainerNetworkStats{
		BytesSent:   util.Float64Ptr(0),
		BytesRcvd:   util.Float64Ptr(0),
		PacketsSent: util.Float64Ptr(0),
		PacketsRcvd: util.Float64Ptr(0),
		Interfaces:  make(map[string]metrics.InterfaceNetStats),
	}

	for ifname, netStats := range networkStats {
		*containerNetworkStats.BytesSent += float64(netStats.TxBytes)
		*containerNetworkStats.BytesRcvd += float64(netStats.RxBytes)
		*containerNetworkStats.PacketsSent += float64(netStats.TxPackets)
		*containerNetworkStats.PacketsRcvd += float64(netStats.RxPackets)

		ifNetStats := metrics.InterfaceNetStats{
			BytesSent:   util.UIntToFloatPtr(netStats.TxBytes),
			BytesRcvd:   util.UIntToFloatPtr(netStats.RxBytes),
			PacketsSent: util.UIntToFloatPtr(netStats.TxPackets),
			PacketsRcvd: util.UIntToFloatPtr(netStats.RxPackets),
		}
		containerNetworkStats.Interfaces[ifname] = ifNetStats
	}

	return containerNetworkStats
}
