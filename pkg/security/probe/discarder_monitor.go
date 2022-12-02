// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux
// +build linux

package probe

import (
	"fmt"

	"github.com/DataDog/datadog-go/v5/statsd"
	manager "github.com/DataDog/ebpf-manager"

	lib "github.com/cilium/ebpf"

	"github.com/DataDog/datadog-agent/pkg/security/ebpf"
	"github.com/DataDog/datadog-agent/pkg/security/metrics"
	"github.com/DataDog/datadog-agent/pkg/security/probe/managerhelper"
	"github.com/DataDog/datadog-agent/pkg/security/secl/model"
	"github.com/DataDog/datadog-agent/pkg/security/utils"
)

// DiscarderMonitor defines a discarder monitor
type DiscarderMonitor struct {
	statsdClient      statsd.ClientInterface
	stats             [2]*lib.Map
	bufferSelector    *lib.Map
	statsZero         []DiscarderStats
	activeStatsBuffer uint32
	numCPU            int
}

// SendStats send stats
func (d *DiscarderMonitor) SendStats() error {
	buffer := d.stats[1-d.activeStatsBuffer]
	iterator := buffer.Iterate()
	stats := make([]DiscarderStats, d.numCPU)
	globalStats := make([]DiscarderStats, model.LastDiscarderEventType)

	var eventType uint32
	for iterator.Next(&eventType, &stats) {
		if int(eventType) >= cap(globalStats) {
			// this should never happen
			continue
		}

		// aggregate all cpu stats
		for _, stat := range stats {
			globalStats[eventType].DiscarderAdded += stat.DiscarderAdded
			globalStats[eventType].EventDiscarded += stat.EventDiscarded
		}
	}

	for eventType, stats := range globalStats {
		if stats.DiscarderAdded == 0 && stats.EventDiscarded == 0 {
			continue
		}

		var tags []string
		if eventType == 0 {
			tags = []string{"discarder_type:pid"}
		} else {
			tags = []string{
				"discarder_type:event",
				fmt.Sprintf("event_type:%s", model.EventType(eventType).String()),
			}
		}

		_ = d.statsdClient.Count(metrics.MetricDiscarderAdded, int64(stats.DiscarderAdded), tags, 1.0)
		_ = d.statsdClient.Count(metrics.MetricEventDiscarded, int64(stats.EventDiscarded), tags, 1.0)

	}
	for i := uint32(0); i != uint32(model.LastDiscarderEventType); i++ {
		_ = buffer.Put(i, d.statsZero)
	}

	d.activeStatsBuffer = 1 - d.activeStatsBuffer
	return d.bufferSelector.Put(ebpf.BufferSelectorERPCMonitorKey, d.activeStatsBuffer)
}

// NewDiscarderMonitor returns a new DiscarderMonitor
func NewDiscarderMonitor(manager *manager.Manager, statsdClient statsd.ClientInterface) (*DiscarderMonitor, error) {
	numCPU, err := utils.NumCPU()
	if err != nil {
		return nil, fmt.Errorf("couldn't fetch the host CPU count: %w", err)
	}

	d := &DiscarderMonitor{
		statsdClient: statsdClient,
		statsZero:    make([]DiscarderStats, numCPU),
		numCPU:       numCPU,
	}

	statsFB, err := managerhelper.Map(manager, "discarder_stats_fb")
	if err != nil {
		return nil, err
	}
	d.stats[0] = statsFB

	statsBB, err := managerhelper.Map(manager, "discarder_stats_bb")
	if err != nil {
		return nil, err
	}
	d.stats[1] = statsBB

	bufferSelector, err := managerhelper.Map(manager, "buffer_selector")
	if err != nil {
		return nil, err
	}
	d.bufferSelector = bufferSelector

	return d, nil
}
