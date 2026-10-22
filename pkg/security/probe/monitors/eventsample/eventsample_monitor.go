// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux

// Package eventsample holds event sample monitor related files
package eventsample

import (
	"fmt"

	manager "github.com/DataDog/ebpf-manager"
	lib "github.com/cilium/ebpf"

	"github.com/DataDog/datadog-agent/pkg/security/probe/managerhelper"
	"github.com/DataDog/datadog-agent/pkg/security/utils"

	"github.com/DataDog/datadog-go/v5/statsd"

	"github.com/DataDog/datadog-agent/pkg/security/ebpf"
	"github.com/DataDog/datadog-agent/pkg/security/metrics"
	"github.com/DataDog/datadog-agent/pkg/security/secl/model"
)

// Stats is used to collect kernel space metrics about event sampling.
type Stats struct {
	EventsTotal          uint64
	EventsSampled        uint64
	MaxPressure          uint64
	SentBelowThreshold   uint64
	AllowedBetween       uint64
	DroppedBetween       uint64
	DroppedAboveCritical uint64
}

// Monitor defines an event sample monitor
type Monitor struct {
	statsdClient      statsd.ClientInterface
	stats             [2]*lib.Map
	bufferSelector    *lib.Map
	statsZero         []Stats
	activeStatsBuffer uint32
	numCPU            int
}

// SendStats sends stats to the statsd client
func (m *Monitor) SendStats() error {
	buffer := m.stats[m.activeStatsBuffer]
	newBuffer := 1 - m.activeStatsBuffer
	if err := m.bufferSelector.Put(ebpf.BufferSelectorSampleMonitorKey, newBuffer); err != nil {
		return err
	}
	m.activeStatsBuffer = newBuffer

	iterator := buffer.Iterate()
	statsAcrossAllCPUs := make([]Stats, m.numCPU)
	statsByEventType := make([]Stats, model.MaxKernelEventType)
	var nonZeroEventTypes []uint32

	var maxPressure uint64
	var eventType uint32
	for iterator.Next(&eventType, &statsAcrossAllCPUs) {
		if int(eventType) >= len(statsByEventType) {
			continue
		}

		var hasData bool
		// aggregate all cpu stats
		for _, stat := range statsAcrossAllCPUs {
			if stat != (Stats{}) {
				hasData = true
			}
			statsByEventType[eventType].EventsTotal += stat.EventsTotal
			statsByEventType[eventType].EventsSampled += stat.EventsSampled
			statsByEventType[eventType].SentBelowThreshold += stat.SentBelowThreshold
			statsByEventType[eventType].AllowedBetween += stat.AllowedBetween
			statsByEventType[eventType].DroppedBetween += stat.DroppedBetween
			statsByEventType[eventType].DroppedAboveCritical += stat.DroppedAboveCritical
			if stat.MaxPressure > maxPressure {
				maxPressure = stat.MaxPressure
			}
		}
		if hasData {
			nonZeroEventTypes = append(nonZeroEventTypes, eventType)
		}
	}

	_ = m.statsdClient.Gauge(metrics.MetricSamplingPressureLevel, float64(maxPressure), []string{}, 1.0)

	for eventType, stats := range statsByEventType {
		if stats == (Stats{}) {
			continue
		}

		eventTypeTag := "event_type:" + model.EventType(eventType).String()
		tags := []string{eventTypeTag}

		if stats.EventsTotal != 0 {
			_ = m.statsdClient.Count(metrics.MetricEventSampleTotal, int64(stats.EventsTotal), tags, 1.0)
		}

		if stats.EventsSampled != 0 {
			_ = m.statsdClient.Count(metrics.MetricEventSampleSampled, int64(stats.EventsSampled), tags, 1.0)
		}

		if stats.SentBelowThreshold != 0 {
			_ = m.statsdClient.Count(metrics.MetricEventSampleSentBelowThreshold, int64(stats.SentBelowThreshold), tags, 1.0)
		}

		if stats.AllowedBetween != 0 {
			_ = m.statsdClient.Count(metrics.MetricEventSampleAllowedBetween, int64(stats.AllowedBetween), tags, 1.0)
		}

		if stats.DroppedBetween != 0 {
			_ = m.statsdClient.Count(metrics.MetricEventSampleDroppedBetween, int64(stats.DroppedBetween), tags, 1.0)
		}

		if stats.DroppedAboveCritical != 0 {
			_ = m.statsdClient.Count(metrics.MetricEventSampleDroppedAboveCritical, int64(stats.DroppedAboveCritical), tags, 1.0)
		}
	}

	// Zero out the buffer keys we just read
	for _, i := range nonZeroEventTypes {
		_ = buffer.Put(i, m.statsZero)
	}

	return nil
}

// NewEventSampleMonitor returns a new Monitor
func NewEventSampleMonitor(manager *manager.Manager, statsdClient statsd.ClientInterface) (*Monitor, error) {
	numCPU, err := utils.NumCPU()
	if err != nil {
		return nil, fmt.Errorf("couldn't fetch the host CPU count: %w", err)
	}

	monitor := &Monitor{
		statsdClient: statsdClient,
		statsZero:    make([]Stats, numCPU),
		numCPU:       numCPU,
	}

	statsFrontBuffer, err := managerhelper.Map(manager, "fb_event_sample_stats")
	if err != nil {
		return nil, err
	}
	monitor.stats[0] = statsFrontBuffer

	statsBackBuffer, err := managerhelper.Map(manager, "bb_event_sample_stats")
	if err != nil {
		return nil, err
	}
	monitor.stats[1] = statsBackBuffer

	bufferSelector, err := managerhelper.Map(manager, "buffer_selector")
	if err != nil {
		return nil, err
	}
	monitor.bufferSelector = bufferSelector

	if err := monitor.bufferSelector.Put(ebpf.BufferSelectorSampleMonitorKey, monitor.activeStatsBuffer); err != nil {
		return nil, fmt.Errorf("failed to initialize event sample buffer selector: %w", err)
	}

	return monitor, nil
}
