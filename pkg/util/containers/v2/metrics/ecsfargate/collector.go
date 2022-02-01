// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build docker
// +build docker

package ecsfargate

import (
	"context"
	"fmt"
	"time"

	"github.com/DataDog/datadog-agent/pkg/config"
	"github.com/DataDog/datadog-agent/pkg/util"
	"github.com/DataDog/datadog-agent/pkg/util/cache"
	"github.com/DataDog/datadog-agent/pkg/util/containers/v2/metrics/provider"
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
	provider.GetProvider().RegisterCollector(provider.CollectorMetadata{
		ID:       ecsFargateCollectorID,
		Priority: 0,
		Runtimes: provider.AllLinuxRuntimes,
		Factory:  func() (provider.Collector, error) { return newEcsFargateCollector() },
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
		return nil, provider.ErrPermaFail
	}

	client, err := metadata.V2()
	if err != nil {
		return nil, provider.ConvertRetrierErr(err)
	}

	return &ecsFargateCollector{client: client}, nil
}

// ID returns the collector ID.
func (e *ecsFargateCollector) ID() string { return ecsFargateCollectorID }

// GetContainerStats returns stats by container ID.
func (e *ecsFargateCollector) GetContainerStats(containerID string, cacheValidity time.Duration) (*provider.ContainerStats, error) {
	stats, err := e.stats(containerID, cacheValidity, e.client.GetContainerStats)
	if err != nil {
		return nil, err
	}

	return convertEcsStats(stats), nil
}

// GetContainerNetworkStats returns network stats by container ID.
func (e *ecsFargateCollector) GetContainerNetworkStats(containerID string, cacheValidity time.Duration) (*provider.ContainerNetworkStats, error) {
	stats, err := e.stats(containerID, cacheValidity, e.client.GetContainerStats)
	if err != nil {
		return nil, err
	}

	return convertNetworkStats(stats.Networks), nil
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

func convertEcsStats(ecsStats *v2.ContainerStats) *provider.ContainerStats {
	if ecsStats == nil {
		return nil
	}

	return &provider.ContainerStats{
		Timestamp: time.Now(),
		CPU:       convertCPUStats(&ecsStats.CPU),
		Memory:    convertMemoryStats(&ecsStats.Memory),
		IO:        convertIOStats(&ecsStats.IO),
	}
}

func convertCPUStats(cpuStats *v2.CPUStats) *provider.ContainerCPUStats {
	if cpuStats == nil {
		return nil
	}

	return &provider.ContainerCPUStats{
		Total:  util.UIntToFloatPtr(cpuStats.Usage.Total),
		System: util.UIntToFloatPtr(cpuStats.Usage.Kernelmode),
		User:   util.UIntToFloatPtr(cpuStats.Usage.Usermode),
	}
}

func convertMemoryStats(memStats *v2.MemStats) *provider.ContainerMemStats {
	if memStats == nil {
		return nil
	}

	return &provider.ContainerMemStats{
		Limit:      util.UIntToFloatPtr(memStats.Limit),
		UsageTotal: util.UIntToFloatPtr(memStats.Usage),
		RSS:        util.UIntToFloatPtr(memStats.Details.RSS),
		Cache:      util.UIntToFloatPtr(memStats.Details.Cache),
	}
}

func convertIOStats(ioStats *v2.IOStats) *provider.ContainerIOStats {
	if ioStats == nil {
		return nil
	}

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

	return &provider.ContainerIOStats{
		ReadBytes:       util.UIntToFloatPtr(readBytes),
		WriteBytes:      util.UIntToFloatPtr(writeBytes),
		ReadOperations:  util.UIntToFloatPtr(readOp),
		WriteOperations: util.UIntToFloatPtr(writeOp),
	}
}

func convertNetworkStats(netStats v2.NetStatsMap) *provider.ContainerNetworkStats {
	// networks is not useful for ECS Fargate as the Fargate endpoint
	// already reports a name (like `eth0`)
	stats := &provider.ContainerNetworkStats{}
	stats.Interfaces = make(map[string]provider.InterfaceNetStats)
	var totalPacketsRcvd, totalPacketsSent, totalBytesRcvd, totalBytesSent uint64
	for iface, statsPerInterface := range netStats {
		iStats := provider.InterfaceNetStats{
			BytesSent:   util.UIntToFloatPtr(statsPerInterface.TxBytes),
			PacketsSent: util.UIntToFloatPtr(statsPerInterface.TxPackets),
			BytesRcvd:   util.UIntToFloatPtr(statsPerInterface.RxBytes),
			PacketsRcvd: util.UIntToFloatPtr(statsPerInterface.RxPackets),
		}
		stats.Interfaces[iface] = iStats

		totalPacketsRcvd += statsPerInterface.RxPackets
		totalPacketsSent += statsPerInterface.TxPackets
		totalBytesRcvd += statsPerInterface.RxBytes
		totalBytesSent += statsPerInterface.TxBytes
	}

	stats.PacketsRcvd = util.UIntToFloatPtr(totalPacketsRcvd)
	stats.PacketsSent = util.UIntToFloatPtr(totalPacketsSent)
	stats.BytesRcvd = util.UIntToFloatPtr(totalBytesRcvd)
	stats.BytesSent = util.UIntToFloatPtr(totalBytesSent)

	return stats
}
