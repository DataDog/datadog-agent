// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux
// +build linux

package probe

import (
	"fmt"
	lib "github.com/cilium/ebpf"

	"github.com/DataDog/datadog-agent/pkg/security/ebpf"
	"github.com/DataDog/datadog-agent/pkg/security/metrics"
	"github.com/DataDog/datadog-agent/pkg/security/secl/model"
	"github.com/DataDog/datadog-go/v5/statsd"
)

// ApproverMonitor defines an approver monitor
type ApproverMonitor struct {
	statsdClient      statsd.ClientInterface
	stats             [2]*lib.Map
	bufferSelector    *lib.Map
	statsZero         []ApproverStats
	activeStatsBuffer uint32
	numCPU            int
}

// SendStats send stats
func (d *ApproverMonitor) SendStats() error {
	buffer := d.stats[1-d.activeStatsBuffer]
	iterator := buffer.Iterate()
	stats := make([]ApproverStats, d.numCPU)
	globalStats := make([]ApproverStats, model.LastApproverEventType)

	var eventType uint32
	for iterator.Next(&eventType, &stats) {
		if int(eventType) >= cap(globalStats) {
			// this should never happen
			continue
		}

		// aggregate all cpu stats
		for _, stat := range stats {
			globalStats[eventType].EventApproved += stat.EventApproved
		}
	}

	for eventType, stats := range globalStats {
		if stats.EventApproved == 0 {
			continue
		}

		var tags []string
		approverType := "undefined"
		if stats.IsBasenameApprover > 0 {
			approverType = "basename"
		} else if stats.IsFlagApprover > 0 {
			approverType = "flag"
		}

		tags = []string{
			fmt.Sprintf("approver_type:%s", approverType),
			fmt.Sprintf("event_type:%s", model.EventType(eventType).String()),
		}

		_ = d.statsdClient.Count(metrics.MetricEventApproved, int64(stats.EventApproved), tags, 1.0)

	}
	for i := uint32(0); i != uint32(model.LastApproverEventType); i++ {
		_ = buffer.Put(i, d.statsZero)
	}

	d.activeStatsBuffer = 1 - d.activeStatsBuffer
	return d.bufferSelector.Put(ebpf.BufferSelectorERPCMonitorKey, d.activeStatsBuffer)
}

// NewApproverMonitor returns a new ApproverMonitor
func NewApproverMonitor(statsdClient statsd.ClientInterface) *ApproverMonitor {
	monitor := &ApproverMonitor{
		statsdClient: statsdClient,
	}

	return monitor
}
