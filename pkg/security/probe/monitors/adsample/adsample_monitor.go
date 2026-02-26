// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux

// Package adsample holds activity dump sample monitor related files
package adsample

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

// Stats is used to collect kernel space metrics about activity dump sampling.
type Stats struct {
	EventsTotal   uint64
	EventsSampled uint64
}

// Monitor defines an activity dump sample monitor
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
	buffer := m.stats[1-m.activeStatsBuffer]
	iterator := buffer.Iterate()
	statsAcrossAllCPUs := make([]Stats, m.numCPU)
	statsByEventType := make([]Stats, model.MaxAllEventType)

	var eventType uint32
	for iterator.Next(&eventType, &statsAcrossAllCPUs) {
		if int(eventType) >= len(statsByEventType) {
			continue
		}

		for _, stat := range statsAcrossAllCPUs {
			statsByEventType[eventType].EventsTotal += stat.EventsTotal
			statsByEventType[eventType].EventsSampled += stat.EventsSampled
		}
	}

	for eventType, stats := range statsByEventType {
		if stats.EventsTotal == 0 && stats.EventsSampled == 0 {
			continue
		}

		eventTypeTag := "event_type:" + model.EventType(eventType).String()

		if stats.EventsTotal != 0 {
			tags := []string{eventTypeTag}
			_ = m.statsdClient.Count(metrics.MetricSecurityProfileV2ADSampleTotal, int64(stats.EventsTotal), tags, 1.0)
		}

		if stats.EventsSampled != 0 {
			tags := []string{eventTypeTag}
			_ = m.statsdClient.Count(metrics.MetricSecurityProfileV2ADSampleSampled, int64(stats.EventsSampled), tags, 1.0)
		}
	}

	for i := uint32(0); i < uint32(model.MaxAllEventType); i++ {
		_ = buffer.Put(i, m.statsZero)
	}

	m.activeStatsBuffer = 1 - m.activeStatsBuffer
	return m.bufferSelector.Put(ebpf.BufferSelectorADSampleMonitorKey, m.activeStatsBuffer)
}

// NewADSampleMonitor returns a new Monitor
func NewADSampleMonitor(manager *manager.Manager, statsdClient statsd.ClientInterface) (*Monitor, error) {
	numCPU, err := utils.NumCPU()
	if err != nil {
		return nil, fmt.Errorf("couldn't fetch the host CPU count: %w", err)
	}

	monitor := &Monitor{
		statsdClient: statsdClient,
		statsZero:    make([]Stats, numCPU),
		numCPU:       numCPU,
	}

	statsFrontBuffer, err := managerhelper.Map(manager, "fb_ad_sample_stats")
	if err != nil {
		return nil, err
	}
	monitor.stats[0] = statsFrontBuffer

	statsBackBuffer, err := managerhelper.Map(manager, "bb_ad_sample_stats")
	if err != nil {
		return nil, err
	}
	monitor.stats[1] = statsBackBuffer

	bufferSelector, err := managerhelper.Map(manager, "buffer_selector")
	if err != nil {
		return nil, err
	}
	monitor.bufferSelector = bufferSelector

	if err := monitor.bufferSelector.Put(ebpf.BufferSelectorADSampleMonitorKey, monitor.activeStatsBuffer); err != nil {
		return nil, fmt.Errorf("failed to initialize AD sample buffer selector: %w", err)
	}

	return monitor, nil
}
