// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build docker
// +build docker

package metrics

import (
	"context"
	"fmt"
	"time"

	"github.com/DataDog/datadog-agent/pkg/config"
	"github.com/DataDog/datadog-agent/pkg/util/cache"
	"github.com/DataDog/datadog-agent/pkg/util/ecs/metadata"
	v2 "github.com/DataDog/datadog-agent/pkg/util/ecs/metadata/v2"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

const (
	ecsFargateCollectorID = "ecs_fargate"
	statsCacheKey         = "ecs-stats-%s"
	statsCacheExpiration  = 10 * time.Second
)

func init() {
	metricsProvider.registerCollector(collectorMetadata{
		id:       ecsFargateCollectorID,
		priority: 0,
		runtimes: allLinuxRuntimes,
		factory:  func() (Collector, error) { return newEcsFargateCollector() },
	})
}

type ecsFargateCollector struct {
	client         *v2.Client
	lastScrapeTime time.Time
}

// ecsStatsFunc allows mocking the ecs api for testing.
type ecsStatsFunc func(ctx context.Context, id string) (*v2.ContainerStats, error)

// newEcsFargateCollector returns a new *ecsFargateCollector.
func newEcsFargateCollector() (*ecsFargateCollector, error) {
	if !config.IsFeaturePresent(config.ECSFargate) {
		return nil, ErrPermaFail
	}

	client, err := metadata.V2()
	if err != nil {
		return nil, err
	}

	return &ecsFargateCollector{client: client}, nil
}

// ID returns the collector ID.
func (e *ecsFargateCollector) ID() string { return ecsFargateCollectorID }

// GetContainerStats returns stats by container ID.
func (e *ecsFargateCollector) GetContainerStats(containerID string, cacheValidity time.Duration) (*ContainerStats, error) {
	stats, err := e.stats(containerID, cacheValidity, e.client.GetContainerStats)
	if err != nil {
		return nil, err
	}

	return convertEcsStats(stats), nil
}

// GetContainerNetworkStats returns network stats by container ID.
func (e *ecsFargateCollector) GetContainerNetworkStats(containerID string, cacheValidity time.Duration, networks map[string]string) (*ContainerNetworkStats, error) {
	stats, err := e.stats(containerID, cacheValidity, e.client.GetContainerStats)
	if err != nil {
		return nil, err
	}

	return convertEcsNetworkStats(stats.Networks, networks), nil
}

// stats returns stats by container ID, it uses an in-memory cache to reduce the number of api calls.
// Cache expires every 2 minutes and can also be invalidated using the cacheValidity argument.
func (e *ecsFargateCollector) stats(containerID string, cacheValidity time.Duration, clientFunc ecsStatsFunc) (*v2.ContainerStats, error) {
	refreshRequired := e.lastScrapeTime.Add(cacheValidity).Before(time.Now())
	cacheKey := fmt.Sprintf(statsCacheKey, containerID)
	if cacheStats, found := cache.Cache.Get(cacheKey); found && !refreshRequired {
		stats := cacheStats.(*v2.ContainerStats)
		log.Debugf("Got ecs stats from cache for container %s", containerID)
		return stats, nil
	}

	stats, err := clientFunc(context.TODO(), containerID)
	if err != nil {
		return nil, fmt.Errorf("failed to get container stats for %s: %w", containerID, err)
	}

	log.Debugf("Got ecs stats from ECS API for container %s", containerID)
	e.lastScrapeTime = time.Now()
	cache.Cache.Set(cacheKey, stats, statsCacheExpiration)

	return stats, nil
}

func convertEcsStats(ecsStats *v2.ContainerStats) *ContainerStats {
	if ecsStats == nil {
		return nil
	}

	return &ContainerStats{
		Timestamp: time.Now(),
		CPU:       convertCPUStats(&ecsStats.CPU),
		Memory:    convertMemoryStats(&ecsStats.Memory),
		IO:        convertIOStats(&ecsStats.IO),
	}
}

func convertCPUStats(cpuStats *v2.CPUStats) *ContainerCPUStats {
	if cpuStats == nil {
		return nil
	}

	stats := &ContainerCPUStats{}
	convertField(&cpuStats.Usage.Total, &stats.Total)
	convertField(&cpuStats.Usage.Kernelmode, &stats.System)
	convertField(&cpuStats.Usage.Usermode, &stats.User)

	return stats
}

func convertMemoryStats(memStats *v2.MemStats) *ContainerMemStats {
	if memStats == nil {
		return nil
	}

	stats := &ContainerMemStats{}
	convertField(&memStats.Limit, &stats.Limit)
	convertField(&memStats.Usage, &stats.UsageTotal)
	convertField(&memStats.Details.RSS, &stats.RSS)
	convertField(&memStats.Details.Cache, &stats.Cache)

	return stats
}

func convertIOStats(ioStats *v2.IOStats) *ContainerIOStats {
	if ioStats == nil {
		return nil
	}

	stats := &ContainerIOStats{}
	var readBytes, writeBytes uint64
	for _, stat := range ioStats.BytesPerDeviceAndKind {
		switch stat.Kind {
		case "Read":
			readBytes += stat.Value
		case "Write":
			writeBytes += stat.Value
		default:
			continue
		}
	}

	convertField(&readBytes, &stats.ReadBytes)
	convertField(&writeBytes, &stats.WriteBytes)

	var readOp, writeOp uint64
	for _, stat := range ioStats.OPPerDeviceAndKind {
		switch stat.Kind {
		case "Read":
			readOp += stat.Value
		case "Write":
			writeOp += stat.Value
		default:
			continue
		}
	}

	convertField(&readOp, &stats.ReadOperations)
	convertField(&writeOp, &stats.WriteOperations)

	return stats
}

func convertEcsNetworkStats(netStats v2.NetStatsMap, networks map[string]string) *ContainerNetworkStats {
	stats := &ContainerNetworkStats{}
	stats.Interfaces = make(map[string]InterfaceNetStats)
	var totalPacketsRcvd, totalPacketsSent, totalBytesRcvd, totalBytesSent uint64
	for iface, statsPerInterface := range netStats {
		if new, found := networks[iface]; found {
			iface = new
		}

		iStats := InterfaceNetStats{}
		convertField(&statsPerInterface.TxBytes, &iStats.BytesSent)
		convertField(&statsPerInterface.TxPackets, &iStats.PacketsSent)
		convertField(&statsPerInterface.RxBytes, &iStats.BytesRcvd)
		convertField(&statsPerInterface.RxPackets, &iStats.PacketsRcvd)
		stats.Interfaces[iface] = iStats

		totalPacketsRcvd += statsPerInterface.RxPackets
		totalPacketsSent += statsPerInterface.TxPackets
		totalBytesRcvd += statsPerInterface.RxBytes
		totalBytesSent += statsPerInterface.TxBytes
	}

	convertField(&totalPacketsRcvd, &stats.PacketsRcvd)
	convertField(&totalPacketsSent, &stats.PacketsSent)
	convertField(&totalBytesRcvd, &stats.BytesRcvd)
	convertField(&totalBytesSent, &stats.BytesSent)

	return stats
}
