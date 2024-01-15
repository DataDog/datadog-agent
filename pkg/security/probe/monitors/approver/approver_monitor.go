// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux

// Package approver holds approver related files
package approver

import (
	"fmt"

	"github.com/DataDog/datadog-agent/pkg/security/probe/managerhelper"
	"github.com/DataDog/datadog-agent/pkg/security/utils"
	manager "github.com/DataDog/ebpf-manager"
	lib "github.com/cilium/ebpf"

	"github.com/DataDog/datadog-agent/pkg/security/ebpf"
	"github.com/DataDog/datadog-agent/pkg/security/metrics"
	"github.com/DataDog/datadog-agent/pkg/security/secl/model"
	"github.com/DataDog/datadog-go/v5/statsd"
)

// Stats is used to collect kernel space metrics about approvers. Stats about added approvers are sent from userspace.
type Stats struct {
	EventApprovedByBasename uint64
	EventApprovedByFlag     uint64
}

// Monitor defines an approver monitor
type Monitor struct {
	statsdClient      statsd.ClientInterface
	stats             [2]*lib.Map
	bufferSelector    *lib.Map
	statsZero         []Stats
	activeStatsBuffer uint32
	numCPU            int
}

// SendStats send stats
func (d *Monitor) SendStats() error {
	buffer := d.stats[1-d.activeStatsBuffer]
	iterator := buffer.Iterate()
	statsAcrossAllCPUs := make([]Stats, d.numCPU)
	statsByEventType := make([]Stats, model.LastApproverEventType)

	var eventType uint32
	for iterator.Next(&eventType, &statsAcrossAllCPUs) {
		if int(eventType) >= cap(statsByEventType) {
			// this should never happen
			continue
		}

		// aggregate all cpu stats
		for _, stat := range statsAcrossAllCPUs {
			statsByEventType[eventType].EventApprovedByBasename += stat.EventApprovedByBasename
			statsByEventType[eventType].EventApprovedByFlag += stat.EventApprovedByFlag
		}
	}

	for eventType, stats := range statsByEventType {
		if stats.EventApprovedByBasename == 0 && stats.EventApprovedByFlag == 0 {
			continue
		}

		eventTypeTag := fmt.Sprintf("event_type:%s", model.EventType(eventType).String())
		tagsForBasenameApprovedEvents := []string{
			"approver_type:basename",
			eventTypeTag,
		}
		tagsForFlagApprovedEvents := []string{
			"approver_type:flag",
			eventTypeTag,
		}

		_ = d.statsdClient.Count(metrics.MetricEventApproved, int64(stats.EventApprovedByBasename), tagsForBasenameApprovedEvents, 1.0)
		_ = d.statsdClient.Count(metrics.MetricEventApproved, int64(stats.EventApprovedByFlag), tagsForFlagApprovedEvents, 1.0)
	}
	for i := uint32(0); i != uint32(model.LastApproverEventType); i++ {
		_ = buffer.Put(i, d.statsZero)
	}

	d.activeStatsBuffer = 1 - d.activeStatsBuffer
	return d.bufferSelector.Put(ebpf.BufferSelectorApproverMonitorKey, d.activeStatsBuffer)
}

// NewApproverMonitor returns a new Monitor
func NewApproverMonitor(manager *manager.Manager, statsdClient statsd.ClientInterface) (*Monitor, error) {
	numCPU, err := utils.NumCPU()
	if err != nil {
		return nil, fmt.Errorf("couldn't fetch the host CPU count: %w", err)
	}

	monitor := &Monitor{
		statsdClient: statsdClient,
		statsZero:    make([]Stats, numCPU),
		numCPU:       numCPU,
	}

	statsFrontBuffer, err := managerhelper.Map(manager, "fb_approver_stats")
	if err != nil {
		return nil, err
	}
	monitor.stats[0] = statsFrontBuffer

	statsBackBuffer, err := managerhelper.Map(manager, "bb_approver_stats")
	if err != nil {
		return nil, err
	}
	monitor.stats[1] = statsBackBuffer

	bufferSelector, err := managerhelper.Map(manager, "buffer_selector")
	if err != nil {
		return nil, err
	}
	monitor.bufferSelector = bufferSelector

	return monitor, nil
}
