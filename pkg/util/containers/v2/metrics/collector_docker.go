// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2021-present Datadog, Inc.

//go:build docker && (linux || windows)
// +build docker,linux docker,windows

package metrics

import (
	"context"
	"fmt"
	"time"

	"github.com/DataDog/datadog-agent/pkg/config"
	"github.com/DataDog/datadog-agent/pkg/util"
	"github.com/DataDog/datadog-agent/pkg/util/docker"
	"github.com/docker/docker/api/types"
)

const (
	dockerCollectorID = "docker"
)

func init() {
	metricsProvider.registerCollector(collectorMetadata{
		id:       dockerCollectorID,
		priority: 1,
		runtimes: []string{RuntimeNameDocker},
		factory: func() (Collector, error) {
			return newDockerCollector()
		},
	})
}

type dockerCollector struct {
	du *docker.DockerUtil
}

func newDockerCollector() (*dockerCollector, error) {
	if !config.IsFeaturePresent(config.Docker) {
		return nil, ErrPermaFail
	}

	du, err := docker.GetDockerUtil()
	if err != nil {
		return nil, ErrNothingYet
	}

	return &dockerCollector{
		du: du,
	}, nil
}

func (d *dockerCollector) ID() string {
	return dockerCollectorID
}

func (d *dockerCollector) GetContainerStats(containerID string, caccheValidity time.Duration) (*ContainerStats, error) {
	ctx := context.TODO()
	stats, err := d.du.GetContainerStats(ctx, containerID)
	if err != nil {
		return nil, fmt.Errorf("failed to get container stats for %s: %w", containerID, err)
	}

	return convertContainerStats(&stats.Stats), nil
}

func (d *dockerCollector) GetContainerNetworkStats(containerID string, cacheValidity time.Duration, networks map[string]string) (*ContainerNetworkStats, error) {
	ctx := context.TODO()
	stats, err := d.du.GetContainerStats(ctx, containerID)
	if err == nil {
	}

	return convertNetworkStats(stats.Networks), nil
}

func convertNetworkStats(networkStats map[string]types.NetworkStats) *ContainerNetworkStats {
	containerNetworkStats := &ContainerNetworkStats{
		BytesSent:   util.Float64Ptr(0),
		BytesRcvd:   util.Float64Ptr(0),
		PacketsSent: util.Float64Ptr(0),
		PacketsRcvd: util.Float64Ptr(0),
		Interfaces:  make(map[string]InterfaceNetStats),
	}

	for ifname, netStats := range networkStats {
		*containerNetworkStats.BytesSent += float64(netStats.TxBytes)
		*containerNetworkStats.BytesRcvd += float64(netStats.RxBytes)
		*containerNetworkStats.PacketsSent += float64(netStats.TxPackets)
		*containerNetworkStats.PacketsRcvd += float64(netStats.RxPackets)

		ifNetStats := InterfaceNetStats{
			BytesSent:   util.Float64Ptr(float64(netStats.TxBytes)),
			BytesRcvd:   util.Float64Ptr(float64(netStats.RxBytes)),
			PacketsSent: util.Float64Ptr(float64(netStats.TxPackets)),
			PacketsRcvd: util.Float64Ptr(float64(netStats.RxPackets)),
		}
		containerNetworkStats.Interfaces[ifname] = ifNetStats
	}

	return containerNetworkStats
}
