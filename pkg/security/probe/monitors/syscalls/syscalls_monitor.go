// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux

package syscalls

import (
	"fmt"

	"github.com/DataDog/datadog-agent/pkg/security/probe/managerhelper"
	"github.com/DataDog/datadog-agent/pkg/security/utils"
	manager "github.com/DataDog/ebpf-manager"
	lib "github.com/cilium/ebpf"

	"github.com/DataDog/datadog-agent/pkg/security/metrics"
	"github.com/DataDog/datadog-agent/pkg/security/secl/model"
	"github.com/DataDog/datadog-go/v5/statsd"
)

// Monitor defines an approver monitor
type Monitor struct {
	statsdClient statsd.ClientInterface
	stats        *lib.Map
	prevStats    [model.MaxAllEventType]uint32
	collected    [model.MaxAllEventType]bool
	numCPU       int
}

// SendStats send stats
func (d *Monitor) SendStats() error {
	iterator := d.stats.Iterate()
	statsAcrossAllCPUs := make([]uint32, d.numCPU)
	statsByEventType := make([]uint32, model.MaxAllEventType)

	var eventType uint32
	for iterator.Next(&eventType, &statsAcrossAllCPUs) {
		if int(eventType) >= cap(statsByEventType) {
			// this should never happen
			continue
		}

		// aggregate all cpu stats
		for _, stat := range statsAcrossAllCPUs {
			statsByEventType[eventType] += stat
		}
	}

	for eventType, inflight := range statsByEventType {
		eventTypeTag := fmt.Sprintf("event_type:%s", model.EventType(eventType).String())
		tagsEvents := []string{
			eventTypeTag,
		}

		if d.collected[eventType] {
			value := float64(int32(inflight) - int32(d.prevStats[eventType]))
			if value > 0 {
				_ = d.statsdClient.Gauge(metrics.MetricSyscallsInFlight, value, tagsEvents, 1.0)
			}
		}
		d.prevStats[eventType] = inflight
		d.collected[eventType] = true
	}

	return nil
}

// NewSyscallsMonitor returns a new Monitor
func NewSyscallsMonitor(manager *manager.Manager, statsdClient statsd.ClientInterface) (*Monitor, error) {
	numCPU, err := utils.NumCPU()
	if err != nil {
		return nil, fmt.Errorf("couldn't fetch the host CPU count: %w", err)
	}

	monitor := &Monitor{
		statsdClient: statsdClient,
		numCPU:       numCPU,
	}

	stats, err := managerhelper.Map(manager, "syscalls_stats")
	if err != nil {
		return nil, err
	}
	monitor.stats = stats

	return monitor, nil
}
