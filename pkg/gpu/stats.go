// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

package gpu

import (
	"fmt"
	"time"

	"github.com/DataDog/datadog-agent/pkg/collector/corechecks/gpu/model"
	ddebpf "github.com/DataDog/datadog-agent/pkg/ebpf"
)

// statsGenerator connects to the active stream handlers and generates stats for the GPU monitoring.
type statsGenerator struct {
	streamHandlers     map[model.StreamKey]*StreamHandler // streamHandlers contains the map of active stream handlers.
	lastGenerationTime time.Time                          // lastGenerationTime is the time of the last stats generation.
	currGenerationTime time.Time                          // currGenerationTime is the time of the current stats generation.
	aggregators        map[uint32]*aggregator             // aggregators contains the map of aggregators
	sysCtx             *systemContext                     // sysCtx is the system context with global GPU-system data
}

func newStatsGenerator(sysCtx *systemContext, streamHandlers map[model.StreamKey]*StreamHandler) *statsGenerator {
	return &statsGenerator{
		streamHandlers:     streamHandlers,
		aggregators:        make(map[uint32]*aggregator),
		lastGenerationTime: time.Now(),
		sysCtx:             sysCtx,
	}
}

func (g *statsGenerator) getStats() (*model.GPUStats, error) {
	now, err := ddebpf.NowNanoseconds()
	if err != nil {
		return nil, fmt.Errorf("getting current time: %w", err)
	}

	g.lastGenerationTime = g.currGenerationTime
	g.currGenerationTime = time.Now()

	for key, handler := range g.streamHandlers {
		aggr := g.getOrCreateAggregator(key)
		currData := handler.getCurrentData(uint64(now))
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

	return &stats, nil
}

func (g *statsGenerator) getOrCreateAggregator(streamKey model.StreamKey) *aggregator {
	aggKey := streamKey.Pid
	if _, ok := g.aggregators[aggKey]; !ok {
		g.aggregators[aggKey] = newAggregator(g.sysCtx)
	}

	g.aggregators[aggKey].lastCheck = g.lastGenerationTime
	g.aggregators[aggKey].measuredInterval = g.currGenerationTime.Sub(g.lastGenerationTime)
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
