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
	"github.com/DataDog/datadog-agent/pkg/util/containers/v2/metrics/provider"
	"github.com/DataDog/datadog-agent/pkg/util/ecs/metadata"
	v2 "github.com/DataDog/datadog-agent/pkg/util/ecs/metadata/v2"
	"github.com/DataDog/datadog-agent/pkg/util/pointer"
)

const (
	ecsFargateCollectorID = "ecs_fargate"
)

func init() {
	provider.GetProvider().RegisterCollector(provider.CollectorMetadata{
		ID:            ecsFargateCollectorID,
		Priority:      0,
		Runtimes:      provider.AllLinuxRuntimes,
		Factory:       func() (provider.Collector, error) { return newEcsFargateCollector() },
		DelegateCache: true,
	})
}

type ecsFargateCollector struct {
	client *v2.Client
}

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
	stats, err := e.stats(containerID)
	if err != nil {
		return nil, err
	}

	return convertEcsStats(stats), nil
}

// GetContainerNetworkStats returns network stats by container ID.
func (e *ecsFargateCollector) GetContainerNetworkStats(containerID string, cacheValidity time.Duration) (*provider.ContainerNetworkStats, error) {
	stats, err := e.stats(containerID)
	if err != nil {
		return nil, err
	}

	return convertNetworkStats(stats.Networks), nil
}

// GetContainerIDForPID returns the container ID for given PID
func (e *ecsFargateCollector) GetContainerIDForPID(pid int, cacheValidity time.Duration) (string, error) {
	// Not available
	return "", nil
}

// GetSelfContainerID returns current process container ID
func (e *ecsFargateCollector) GetSelfContainerID() (string, error) {
	// Not available
	return "", nil
}

// stats returns stats by container ID, it uses an in-memory cache to reduce the number of api calls.
// Cache expires every 2 minutes and can also be invalidated using the cacheValidity argument.
func (e *ecsFargateCollector) stats(containerID string) (*v2.ContainerStats, error) {
	stats, err := e.client.GetContainerStats(context.TODO(), containerID)
	if err != nil {
		return nil, fmt.Errorf("failed to get container stats for %s: %w", containerID, err)
	}

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
		Total:  pointer.UIntToFloatPtr(cpuStats.Usage.Total),
		System: pointer.UIntToFloatPtr(cpuStats.Usage.Kernelmode),
		User:   pointer.UIntToFloatPtr(cpuStats.Usage.Usermode),
	}
}

func convertMemoryStats(memStats *v2.MemStats) *provider.ContainerMemStats {
	if memStats == nil {
		return nil
	}

	return &provider.ContainerMemStats{
		Limit:      pointer.UIntToFloatPtr(memStats.Limit),
		UsageTotal: pointer.UIntToFloatPtr(memStats.Usage),
		RSS:        pointer.UIntToFloatPtr(memStats.Details.RSS),
		Cache:      pointer.UIntToFloatPtr(memStats.Details.Cache),
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
		ReadBytes:       pointer.UIntToFloatPtr(readBytes),
		WriteBytes:      pointer.UIntToFloatPtr(writeBytes),
		ReadOperations:  pointer.UIntToFloatPtr(readOp),
		WriteOperations: pointer.UIntToFloatPtr(writeOp),
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
			BytesSent:   pointer.UIntToFloatPtr(statsPerInterface.TxBytes),
			PacketsSent: pointer.UIntToFloatPtr(statsPerInterface.TxPackets),
			BytesRcvd:   pointer.UIntToFloatPtr(statsPerInterface.RxBytes),
			PacketsRcvd: pointer.UIntToFloatPtr(statsPerInterface.RxPackets),
		}
		stats.Interfaces[iface] = iStats

		totalPacketsRcvd += statsPerInterface.RxPackets
		totalPacketsSent += statsPerInterface.TxPackets
		totalBytesRcvd += statsPerInterface.RxBytes
		totalBytesSent += statsPerInterface.TxBytes
	}

	stats.PacketsRcvd = pointer.UIntToFloatPtr(totalPacketsRcvd)
	stats.PacketsSent = pointer.UIntToFloatPtr(totalPacketsSent)
	stats.BytesRcvd = pointer.UIntToFloatPtr(totalBytesRcvd)
	stats.BytesSent = pointer.UIntToFloatPtr(totalBytesSent)

	return stats
}
