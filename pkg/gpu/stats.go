// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

//go:build linux_bpf

package gpu

import (
	"github.com/DataDog/datadog-agent/pkg/collector/corechecks/gpu/model"
	ddebpf "github.com/DataDog/datadog-agent/pkg/ebpf"
)

// statsGenerator connects to the active stream handlers and generates stats for the GPU monitoring, by distributing
// the data to the aggregators which are responsible for computing the metrics.
type statsGenerator struct {
	streamHandlers      map[streamKey]*StreamHandler // streamHandlers contains the map of active stream handlers.
	lastGenerationKTime int64                        // lastGenerationTime is the kernel time of the last stats generation.
	currGenerationKTime int64                        // currGenerationTime is the kernel time of the current stats generation.
	aggregators         map[uint32]*aggregator       // aggregators contains the map of aggregators
	sysCtx              *systemContext               // sysCtx is the system context with global GPU-system data
}

func newStatsGenerator(sysCtx *systemContext, streamHandlers map[streamKey]*StreamHandler) *statsGenerator {
	currKTime, _ := ddebpf.NowNanoseconds()
	return &statsGenerator{
		streamHandlers:      streamHandlers,
		aggregators:         make(map[uint32]*aggregator),
		lastGenerationKTime: currKTime,
		currGenerationKTime: currKTime,
		sysCtx:              sysCtx,
	}
}

// getStats takes data from all active stream handlers, aggregates them and returns the per-process GPU stats.
// This function gets called by the Probe when it receives a data request in the GetAndFlush method
// TODO: consider removing this parameter and encapsulate it inside the function (will affect UTs as they rely on precise time intervals)
func (g *statsGenerator) getStats(nowKtime int64) *model.GPUStats {
	g.currGenerationKTime = nowKtime

	for key, handler := range g.streamHandlers {
		aggr := g.getOrCreateAggregator(key.pid)
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
		ProcessStats: make(map[uint32]model.ProcessStats),
	}

	for pid, aggr := range g.aggregators {
		stats.ProcessStats[pid] = aggr.getStats(normFactor)
	}

	g.lastGenerationKTime = g.currGenerationKTime

	return stats
}

func (g *statsGenerator) getOrCreateAggregator(pid uint32) *aggregator {
	if _, ok := g.aggregators[pid]; !ok {
		g.aggregators[pid] = newAggregator(g.sysCtx)
	}

	// Update the last check time and the measured interval, as these change between check runs
	g.aggregators[pid].lastCheckKtime = uint64(g.lastGenerationKTime)
	g.aggregators[pid].measuredIntervalNs = g.currGenerationKTime - g.lastGenerationKTime
	return g.aggregators[pid]
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
}
