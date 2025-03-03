// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

//go:build linux_bpf

package gpu

import (
	"fmt"

	"github.com/NVIDIA/go-nvml/pkg/nvml"

	"github.com/DataDog/datadog-agent/comp/core/telemetry"
	"github.com/DataDog/datadog-agent/pkg/collector/corechecks/gpu/model"
	ddebpf "github.com/DataDog/datadog-agent/pkg/ebpf"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

// statsGenerator connects to the active stream handlers and generates stats for the GPU monitoring, by distributing
// the data to the aggregators which are responsible for computing the metrics.
type statsGenerator struct {
	streamHandlers      map[streamKey]*StreamHandler   // streamHandlers contains the map of active stream handlers.
	lastGenerationKTime int64                          // lastGenerationTime is the kernel time of the last stats generation.
	currGenerationKTime int64                          // currGenerationTime is the kernel time of the current stats generation.
	aggregators         map[model.StatsKey]*aggregator // aggregators contains the map of aggregators
	sysCtx              *systemContext                 // sysCtx is the system context with global GPU-system data
	telemetry           *statsGeneratorTelemetry       // telemetry contains the telemetry component for the stats generator
}

type statsGeneratorTelemetry struct {
	aggregators telemetry.Gauge
}

func newStatsGenerator(sysCtx *systemContext, streamHandlers map[streamKey]*StreamHandler, tm telemetry.Component) *statsGenerator {
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
	subsystem := gpuTelemetryModule + "__stats_generator"
	return &statsGeneratorTelemetry{
		aggregators: tm.NewGauge(subsystem, "aggregators", nil, "Number of active GPU stats aggregators"),
	}
}

// getStats takes data from all active stream handlers, aggregates them and returns the per-process GPU stats.
// This function gets called by the Probe when it receives a data request in the GetAndFlush method
// TODO: consider removing this parameter and encapsulate it inside the function (will affect UTs as they rely on precise time intervals)
func (g *statsGenerator) getStats(nowKtime int64) *model.GPUStats {
	g.currGenerationKTime = nowKtime

	for key, handler := range g.streamHandlers {
		aggr, err := g.getOrCreateAggregator(key)
		if err != nil {
			log.Errorf("Error getting or creating aggregator for key %v: %s", key, err)
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

		if handler.processEnded {
			aggr.processTerminated = true
		}
	}

	normFactor := g.getNormalizationFactor()

	stats := &model.GPUStats{
		Metrics: make([]model.StatsTuple, 0, len(g.aggregators)),
	}

	for aggKey, aggr := range g.aggregators {
		entry := model.StatsTuple{
			Key:                aggKey,
			UtilizationMetrics: aggr.getStats(normFactor),
		}
		stats.Metrics = append(stats.Metrics, entry)
	}

	g.telemetry.aggregators.Set(float64(len(g.aggregators)))

	g.lastGenerationKTime = g.currGenerationKTime

	return stats
}

func (g *statsGenerator) getOrCreateAggregator(sKey streamKey) (*aggregator, error) {
	aggKey := model.StatsKey{
		PID:         sKey.pid,
		DeviceUUID:  sKey.gpuUUID,
		ContainerID: sKey.containerID,
	}

	if _, ok := g.aggregators[aggKey]; !ok {
		gpuDevice, err := g.sysCtx.getDeviceByUUID(sKey.gpuUUID)
		if err != nil {
			return nil, fmt.Errorf("Error getting device by UUID %s: %s", sKey.gpuUUID, err)
		}

		maxThreads, ret := gpuDevice.GetNumGpuCores()
		if ret != nvml.SUCCESS {
			return nil, fmt.Errorf("Error getting number of GPU cores: %s", nvml.ErrorString(ret))
		}

		memUsage, ret := gpuDevice.GetMemoryInfo()
		if ret != nvml.SUCCESS {
			return nil, fmt.Errorf("Error getting memory info: %s", nvml.ErrorString(ret))
		}

		g.aggregators[aggKey] = newAggregator(uint64(maxThreads), memUsage.Total)
	}

	// Update the last check time and the measured interval, as these change between check runs
	g.aggregators[aggKey].lastCheckKtime = uint64(g.lastGenerationKTime)
	g.aggregators[aggKey].measuredIntervalNs = g.currGenerationKTime - g.lastGenerationKTime
	return g.aggregators[aggKey], nil
}

// getNormalizationFactor returns the factor to use for utilization
// normalization. Because we compute the utilization based on the number of
// threads launched by the kernel, we need to normalize the utilization if we
// get above 100%, as the GPU can enqueue threads.
func (g *statsGenerator) getNormalizationFactor() float64 {
	totalGPUUtilization := 0.0
	for _, aggr := range g.aggregators {
		totalGPUUtilization += aggr.getGPUUtilization()
	}

	return max(1.0, totalGPUUtilization)
}

func (g *statsGenerator) cleanupFinishedAggregators() {
	for pid, aggr := range g.aggregators {
		if aggr.processTerminated {
			delete(g.aggregators, pid)
		}
	}

	g.telemetry.aggregators.Set(float64(len(g.aggregators)))
}
