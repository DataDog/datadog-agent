// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build docker

package ecsfargate

import (
	"context"
	"fmt"
	"math"
	"sync"
	"time"

	"github.com/DataDog/datadog-agent/pkg/config"
	"github.com/DataDog/datadog-agent/pkg/util/containers/metrics/provider"
	"github.com/DataDog/datadog-agent/pkg/util/ecs/metadata"
	v2 "github.com/DataDog/datadog-agent/pkg/util/ecs/metadata/v2"
	"github.com/DataDog/datadog-agent/pkg/util/log"
	"github.com/DataDog/datadog-agent/pkg/util/pointer"
)

const (
	ecsFargateCollectorID = "ecs_fargate"
	ecsTaskTimeout        = 2 * time.Second
	// cpuKey represents the cpu key used in the resource limits map returned by the ECS API
	cpuKey = "CPU"
	// memoryKey represents the memory key used in the resource limits map returned by the ECS API
	memoryKey = "Memory"
)

var ecsUnsetMemoryLimit = uint64(math.Pow(2, 62))

func init() {
	provider.GetProvider().RegisterCollector(provider.CollectorMetadata{
		ID:            ecsFargateCollectorID,
		Priority:      0,
		Runtimes:      []string{provider.RuntimeNameECSFargate},
		Factory:       func() (provider.Collector, error) { return newEcsFargateCollector() },
		DelegateCache: true,
	})
}

type ecsFargateCollector struct {
	client *v2.Client

	taskSpec *v2.Task
	taskLock sync.Mutex
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
func (e *ecsFargateCollector) GetContainerStats(containerNS, containerID string, cacheValidity time.Duration) (*provider.ContainerStats, error) {
	stats, err := e.stats(containerID)
	if err != nil {
		return nil, err
	}
	containerStats := convertEcsStats(stats)

	// Data from Task spec are not mandatory, do not return an error
	if err := e.getTask(); err == nil {
		fillFromSpec(containerStats, e.taskSpec)
	} else {
		log.Warnf("Unable to get ECS Fargate task metadata, err: %v", err)
	}

	return containerStats, nil
}

// GetContainerPIDStats returns pid stats by container ID.
func (e *ecsFargateCollector) GetContainerPIDStats(containerNS, containerID string, cacheValidity time.Duration) (*provider.ContainerPIDStats, error) {
	// Not available
	return nil, nil
}

// GetContainerOpenFilesCount returns open files count by container ID.
func (e *ecsFargateCollector) GetContainerOpenFilesCount(containerNS, containerID string, cacheValidity time.Duration) (*uint64, error) {
	// Not available
	return nil, nil
}

// GetContainerNetworkStats returns network stats by container ID.
func (e *ecsFargateCollector) GetContainerNetworkStats(containerNS, containerID string, cacheValidity time.Duration) (*provider.ContainerNetworkStats, error) {
	stats, err := e.stats(containerID)
	if err != nil {
		return nil, err
	}

	return convertNetworkStats(stats), nil
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

	dataTimestamp, err := time.Parse(time.RFC3339Nano, ecsStats.Timestamp)
	if err != nil {
		dataTimestamp = time.Now()
	}

	return &provider.ContainerStats{
		Timestamp: dataTimestamp,
		CPU:       convertCPUStats(&ecsStats.CPU),
		Memory:    convertMemoryStats(&ecsStats.Memory),
		IO:        convertIOStats(&ecsStats.IO),
	}
}

func (e *ecsFargateCollector) getTask() error {
	if e.taskSpec != nil {
		return nil
	}

	// We can observe a nil task and refresh multiple times, which is not optimal
	// but better than paying the lock cost forever.
	e.taskLock.Lock()
	defer e.taskLock.Unlock()

	ctx, cancel := context.WithTimeout(context.Background(), ecsTaskTimeout)
	defer cancel()

	task, err := e.client.GetTask(ctx)
	if err != nil {
		return err
	}

	e.taskSpec = task
	return nil
}

func convertNetworkStats(ecsStats *v2.ContainerStats) *provider.ContainerNetworkStats {
	if ecsStats == nil {
		return nil
	}

	dataTimestamp, err := time.Parse(time.RFC3339Nano, ecsStats.Timestamp)
	if err != nil {
		dataTimestamp = time.Now()
	}

	// networks is not useful for ECS Fargate as the Fargate endpoint
	// already reports a name (like `eth0`)
	stats := &provider.ContainerNetworkStats{
		Timestamp: dataTimestamp,
	}
	stats.Interfaces = make(map[string]provider.InterfaceNetStats)
	var totalPacketsRcvd, totalPacketsSent, totalBytesRcvd, totalBytesSent uint64
	for iface, statsPerInterface := range ecsStats.Networks {
		iStats := provider.InterfaceNetStats{
			BytesSent:   pointer.Ptr(float64(statsPerInterface.TxBytes)),
			PacketsSent: pointer.Ptr(float64(statsPerInterface.TxPackets)),
			BytesRcvd:   pointer.Ptr(float64(statsPerInterface.RxBytes)),
			PacketsRcvd: pointer.Ptr(float64(statsPerInterface.RxPackets)),
		}
		stats.Interfaces[iface] = iStats

		totalPacketsRcvd += statsPerInterface.RxPackets
		totalPacketsSent += statsPerInterface.TxPackets
		totalBytesRcvd += statsPerInterface.RxBytes
		totalBytesSent += statsPerInterface.TxBytes
	}

	stats.PacketsRcvd = pointer.Ptr(float64(totalPacketsRcvd))
	stats.PacketsSent = pointer.Ptr(float64(totalPacketsSent))
	stats.BytesRcvd = pointer.Ptr(float64(totalBytesRcvd))
	stats.BytesSent = pointer.Ptr(float64(totalBytesSent))

	return stats
}

func convertCPUStats(cpuStats *v2.CPUStats) *provider.ContainerCPUStats {
	if cpuStats == nil {
		return nil
	}

	return &provider.ContainerCPUStats{
		Total:  pointer.Ptr(float64(cpuStats.Usage.Total)),
		System: pointer.Ptr(float64(cpuStats.Usage.Kernelmode)),
		User:   pointer.Ptr(float64(cpuStats.Usage.Usermode)),
	}
}

func convertMemoryStats(memStats *v2.MemStats) *provider.ContainerMemStats {
	if memStats == nil {
		return nil
	}

	cMemStats := &provider.ContainerMemStats{
		UsageTotal: pointer.Ptr(float64(memStats.Usage)),
		RSS:        pointer.Ptr(float64(memStats.Details.RSS)),
		Cache:      pointer.Ptr(float64(memStats.Details.Cache)),
	}

	if memStats.Limit > 0 && memStats.Limit < ecsUnsetMemoryLimit {
		cMemStats.Limit = pointer.Ptr(float64(memStats.Limit))
	}

	return cMemStats
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
		ReadBytes:       pointer.Ptr(float64(readBytes)),
		WriteBytes:      pointer.Ptr(float64(writeBytes)),
		ReadOperations:  pointer.Ptr(float64(readOp)),
		WriteOperations: pointer.Ptr(float64(writeOp)),
	}
}

func fillFromSpec(containerStats *provider.ContainerStats, taskSpec *v2.Task) {
	// Handling Task CPU/Memory Limit (cannot be empty, mandatory on ECS Fargate)
	taskCPULimit := taskSpec.Limits[cpuKey]
	if taskCPULimit != 0 && containerStats.CPU != nil {
		containerStats.CPU.Limit = pointer.Ptr(taskCPULimit * 100) // vCPU to percentage (0-N00%)
	}

	taskMemoryLimit := taskSpec.Limits[memoryKey]
	if taskMemoryLimit != 0 && containerStats.Memory != nil && containerStats.Memory.Limit == nil {
		containerStats.Memory.Limit = pointer.Ptr(taskMemoryLimit * 1024 * 1024) // Megabytes to bytes
	}
}
