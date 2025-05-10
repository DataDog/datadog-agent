// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

//go:build linux_bpf && nvml

package gpu

import (
	"fmt"

	"github.com/DataDog/datadog-agent/comp/core/telemetry"
	"github.com/DataDog/datadog-agent/pkg/collector/corechecks/gpu/model"
	ddebpf "github.com/DataDog/datadog-agent/pkg/ebpf"
	"github.com/DataDog/datadog-agent/pkg/gpu/config/consts"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

// statsGenerator connects to the active stream handlers and generates stats for the GPU monitoring, by distributing
// the data to the aggregators which are responsible for computing the metrics.
type statsGenerator struct {
	streamHandlers      *streamCollection              // streamHandlers contains the map of active stream handlers.
	lastGenerationKTime int64                          // lastGenerationTime is the kernel time of the last stats generation.
	currGenerationKTime int64                          // currGenerationTime is the kernel time of the current stats generation.
	aggregators         map[model.StatsKey]*aggregator // aggregators contains the map of aggregators
	sysCtx              *systemContext                 // sysCtx is the system context with global GPU-system data
	telemetry           *statsGeneratorTelemetry       // telemetry contains the telemetry component for the stats generator
}

type statsGeneratorTelemetry struct {
	aggregators telemetry.Gauge
}

func newStatsGenerator(sysCtx *systemContext, streamHandlers *streamCollection, tm telemetry.Component) *statsGenerator {
	currKTime, _ := ddebpf.NowNanoseconds()
	return &statsGenerator{
		streamHandlers:      streamHandlers,
		aggregators:         make(map[model.StatsKey]*aggregator),
		lastGenerationKTime: currKTime,
		currGenerationKTime: currKTime,
		sysCtx:              sysCtx,
		telemetry:           newStatsGeneratorTelemetry(tm),
	}
}

func newStatsGeneratorTelemetry(tm telemetry.Component) *statsGeneratorTelemetry {
	subsystem := consts.GpuTelemetryModule + "__stats_generator"
	return &statsGeneratorTelemetry{
		aggregators: tm.NewGauge(subsystem, "aggregators", nil, "Number of active GPU stats aggregators"),
	}
}

// getStats takes data from all active stream handlers, aggregates them and returns the per-process GPU stats.
// This function gets called by the Probe when it receives a data request in the GetAndFlush method
// TODO: consider removing this parameter and encapsulate it inside the function (will affect UTs as they rely on precise time intervals)
func (g *statsGenerator) getStats(nowKtime int64) (*model.GPUStats, error) {
	g.currGenerationKTime = nowKtime
	// mark all aggregators as inactive
	for _, aggr := range g.aggregators {
		aggr.isActive = false
	}

	for handler := range g.streamHandlers.allStreams() {
		aggr, err := g.getOrCreateAggregator(handler.metadata)
		if err != nil {
			log.Errorf("Error getting or creating aggregator for handler metadata %v: %s", handler.metadata, err)
			continue
		}

		currData := handler.getCurrentData(uint64(nowKtime))
		pastData := handler.getPastData(true)

		if currData != nil {
			aggr.processCurrentData(currData)
		}

		if pastData != nil {
			aggr.processPastData(pastData)
		}
	}

	// Compute unnormalized stats first for each aggregator
	rawStats := make([]model.StatsTuple, 0, len(g.aggregators))

	for aggKey, aggr := range g.aggregators {
		entry := model.StatsTuple{
			Key:                aggKey,
			UtilizationMetrics: aggr.getRawStats(),
		}
		rawStats = append(rawStats, entry)
	}

	// Now get the normalization factors for each device
	normFactors, err := g.getNormalizationFactors(rawStats)
	if err != nil {
		return nil, err
	}

	// Apply normalization to each stats entry
	stats := &model.GPUStats{
		Metrics: make([]model.StatsTuple, 0, len(rawStats)),
	}
	for _, entry := range rawStats {
		factors, ok := normFactors[entry.Key.DeviceUUID]
		if !ok {
			// Shouldn't happen, as the normalization factors are computed based
			// on the device UUIDs present in rawStats.
			return nil, fmt.Errorf("cannot find normalization factors for device %s", entry.Key.DeviceUUID)
		}

		entry.UtilizationMetrics.UsedCores /= factors.cores
		entry.UtilizationMetrics.Memory.CurrentBytes = uint64(float64(entry.UtilizationMetrics.Memory.CurrentBytes) / factors.memory)
		entry.UtilizationMetrics.Memory.MaxBytes = uint64(float64(entry.UtilizationMetrics.Memory.MaxBytes) / factors.memory)

		stats.Metrics = append(stats.Metrics, entry)
	}

	g.telemetry.aggregators.Set(float64(len(g.aggregators)))

	g.lastGenerationKTime = g.currGenerationKTime

	return stats, nil
}

func (g *statsGenerator) getOrCreateAggregator(sKey streamMetadata) (*aggregator, error) {
	aggKey := model.StatsKey{
		PID:         sKey.pid,
		DeviceUUID:  sKey.gpuUUID,
		ContainerID: sKey.containerID,
	}

	if _, ok := g.aggregators[aggKey]; !ok {
		deviceCores, err := g.sysCtx.deviceCache.Cores(sKey.gpuUUID)
		if err != nil {
			return nil, err
		}

		g.aggregators[aggKey] = newAggregator(deviceCores)
	}

	// Update the last check time and the measured interval, as these change between check runs
	// also mark the aggregator as active
	g.aggregators[aggKey].lastCheckKtime = uint64(g.lastGenerationKTime)
	g.aggregators[aggKey].measuredIntervalNs = g.currGenerationKTime - g.lastGenerationKTime
	g.aggregators[aggKey].isActive = true

	return g.aggregators[aggKey], nil
}

type normalizationFactors struct {
	cores  float64
	memory float64
}

// getNormalizationFactors returns the factor to use for utilization
// normalization per GPU device. Because we compute the utilization based on the
// number of threads launched by the kernel, we need to normalize the
// utilization if we get above 100%, as the GPU can enqueue threads. We need to
// use factors instead of clamping, as we might have multiple processes on the
// same GPU adding up to more than 100%, so we need to scale all of them back.
// It is guaranteed that the normalization factors are always equal to or
// greater than 1
func (g *statsGenerator) getNormalizationFactors(stats []model.StatsTuple) (map[string]normalizationFactors, error) {
	usages := make(map[string]*normalizationFactors) // reuse the normalizationFactors struct to keep track of the total usage

	for _, entry := range stats {
		usage := usages[entry.Key.DeviceUUID]
		if usage == nil {
			usage = &normalizationFactors{}
			usages[entry.Key.DeviceUUID] = usage
		}

		usage.cores += entry.UtilizationMetrics.UsedCores
		usage.memory += float64(entry.UtilizationMetrics.Memory.MaxBytes)
	}

	normFactors := make(map[string]normalizationFactors)
	for uuid, usage := range usages {
		device, ok := g.sysCtx.deviceCache.GetByUUID(uuid)
		if !ok {
			return nil, fmt.Errorf("cannot find device for UUID %s", uuid)
		}

		var deviceFactors normalizationFactors

		// This factor guarantees that usage[uuid] / normFactor <= maxThreads
		if usage.cores > float64(device.GetDeviceInfo().CoreCount) {
			deviceFactors.cores = usage.cores / float64(device.GetDeviceInfo().CoreCount)
		} else {
			deviceFactors.cores = 1
		}

		if usage.memory > float64(device.GetDeviceInfo().Memory) {
			deviceFactors.memory = usage.memory / float64(device.GetDeviceInfo().Memory)
		} else {
			deviceFactors.memory = 1
		}

		normFactors[device.GetDeviceInfo().UUID] = deviceFactors
	}

	return normFactors, nil
}

// cleanupFinishedAggregators cleans up the aggregators that are no longer
// active. An aggregator is no longer needed if it was not active in the last
// check, in other words, if all the corresponding streams have been removed.
// This allows us to centralize the logic of "termination" in the streams
// themselves.
//
// The downside is that we will cleanup the aggregators one getStats() cycle
// after all the streams have finished. That is, all the streams need to be
// deleted, then getStats() needs to be called to update the activity map, and
// then cleanupFinishedAggregators will actually remove the aggregators. This
// should not affect functionality, as the reported values from those
// aggregators without streams will be zero.
//
// TODO: Test this behavior and remove the corresponding logic in the core
// check.
func (g *statsGenerator) cleanupFinishedAggregators() {
	for key, aggr := range g.aggregators {
		if !aggr.isActive {
			delete(g.aggregators, key)
		}
	}

	g.telemetry.aggregators.Set(float64(len(g.aggregators)))
}
