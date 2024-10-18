// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

package gpu

import (
	"github.com/DataDog/datadog-agent/pkg/collector/corechecks/gpu/model"
)

// statsGenerator connects to the active stream handlers and generates stats for the GPU monitoring.
type statsGenerator struct {
	streamHandlers      map[model.StreamKey]*StreamHandler // streamHandlers contains the map of active stream handlers.
	lastGenerationKTime int64                              // lastGenerationTime is the kernel time of the last stats generation.
	currGenerationKTime int64                              // currGenerationTime is the kernel time of the current stats generation.
	aggregators         map[uint32]*aggregator             // aggregators contains the map of aggregators
	sysCtx              *systemContext                     // sysCtx is the system context with global GPU-system data
}

func newStatsGenerator(sysCtx *systemContext, currKTime int64, streamHandlers map[model.StreamKey]*StreamHandler) *statsGenerator {
	return &statsGenerator{
		streamHandlers:      streamHandlers,
		aggregators:         make(map[uint32]*aggregator),
		lastGenerationKTime: currKTime,
		currGenerationKTime: currKTime,
		sysCtx:              sysCtx,
	}
}

func (g *statsGenerator) getStats(nowKtime int64) *model.GPUStats {
	g.currGenerationKTime = nowKtime

	for key, handler := range g.streamHandlers {
		aggr := g.getOrCreateAggregator(key)
		currData := handler.getCurrentData(uint64(nowKtime))
		pastData := handler.getPastData(true)

		if currData != nil {
			aggr.processCurrentData(currData)
		}

		if pastData != nil {
			aggr.processPastData(pastData)
		}

		if handler.processEnded {
			aggr.processEnded = true
		}
	}

	g.configureNormalizationFactor()

	stats := model.GPUStats{
		PIDStats: make(map[uint32]model.PIDStats),
	}

	for pid, aggregator := range g.aggregators {
		stats.PIDStats[pid] = aggregator.getStats()
	}

	g.lastGenerationKTime = g.currGenerationKTime

	return &stats
}

func (g *statsGenerator) getOrCreateAggregator(streamKey model.StreamKey) *aggregator {
	aggKey := streamKey.Pid
	if _, ok := g.aggregators[aggKey]; !ok {
		g.aggregators[aggKey] = newAggregator(g.sysCtx)
	}

	g.aggregators[aggKey].lastCheckKtime = g.lastGenerationKTime
	g.aggregators[aggKey].measuredIntervalNs = g.currGenerationKTime - g.lastGenerationKTime
	return g.aggregators[aggKey]
}

func (g *statsGenerator) configureNormalizationFactor() {
	// As we compute the utilization based on the number of threads launched by the kernel, we need to
	// normalize the utilization if we get above 100%, as the GPU can enqueue threads.
	totalGPUUtilization := 0.0
	for _, aggregator := range g.aggregators {
		// Only consider aggregators that received data this interval
		if aggregator.hasPendingData {
			totalGPUUtilization += aggregator.getGPUUtilization()
		}
	}

	normFactor := max(1.0, totalGPUUtilization)

	for _, aggregator := range g.aggregators {
		aggregator.setGPUUtilizationNormalizationFactor(normFactor)
	}
}

func (g *statsGenerator) cleanupFinishedAggregators() {
	for pid, aggregator := range g.aggregators {
		if aggregator.processEnded {
			delete(g.aggregators, pid)
		}
	}
}
