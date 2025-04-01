// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux

// Package dns holds dns receiver stats
package dns

import (
	"fmt"
	"github.com/DataDog/datadog-agent/pkg/security/seclog"

	"github.com/DataDog/datadog-go/v5/statsd"
	manager "github.com/DataDog/ebpf-manager"

	lib "github.com/cilium/ebpf"

	"github.com/DataDog/datadog-agent/pkg/security/ebpf"
	"github.com/DataDog/datadog-agent/pkg/security/metrics"
	"github.com/DataDog/datadog-agent/pkg/security/probe/managerhelper"
	"github.com/DataDog/datadog-agent/pkg/security/utils"
)

// Stats contains the stats of the DNS monitor
type Stats struct {
	FilteredDNSPackets  uint32
	SameIDDifferentSize uint32
}

// Monitor implements DNS response filtering statistics collection using a double-buffered
// approach to prevent data races between kernel and userspace. It maintains per-CPU
// statistics and aggregates them for StatsD reporting.
type Monitor struct {
	// stats holds two eBPF maps for double buffering statistics
	stats [2]*lib.Map
	// bufferSelector determines which buffer is currently active
	bufferSelector    *lib.Map
	activeStatsBuffer uint32
	numCPU            int
	statsZero         []Stats
	statsdClient      statsd.ClientInterface
}

func (d *Monitor) switchBuffer() error {
	newBuffer := 1 - d.activeStatsBuffer
	if err := d.bufferSelector.Put(ebpf.BufferSelectorDNSResponseFilteredMonitorKey, newBuffer); err != nil {
		return fmt.Errorf("failed to switch DNS stats buffer: %w", err)
	}
	d.activeStatsBuffer = newBuffer
	return nil
}

// SendStats send stats
func (d *Monitor) SendStats() error {
	buffer := d.stats[d.activeStatsBuffer]

	err := d.switchBuffer()
	if err != nil {
		return err
	}

	statsAcrossAllCPUs := make([]Stats, d.numCPU)
	var totalFilteredOnKernel int64
	var totalSameIDDifferentSize int64

	err = buffer.Lookup(uint32(0), statsAcrossAllCPUs)
	if err != nil {
		return fmt.Errorf("failed to lookup DNS stats: %w", err)
	}

	for _, val := range statsAcrossAllCPUs {
		totalFilteredOnKernel += int64(val.FilteredDNSPackets)
		totalSameIDDifferentSize += int64(val.SameIDDifferentSize)
	}

	var tags []string
	if err := d.statsdClient.Count(metrics.MetricRepeatedDNSResponsesFilteredOnKernel, totalFilteredOnKernel, tags, 1.0); err != nil {
		seclog.Tracef("couldn't set MetricRepeatedDNSResponsesFilteredOnKernel metric: %s", err)
		return err
	}

	if err := d.statsdClient.Count(metrics.MetricDNSSameIDDifferentSize, totalSameIDDifferentSize, tags, 1.0); err != nil {
		seclog.Tracef("couldn't set MetricDNSSameIDDifferentSize metric: %s", err)
		return err
	}

	err = buffer.Put(uint32(0), d.statsZero)
	if err != nil {
		return fmt.Errorf("failed to reset DNS stats buffer: %w", err)
	}

	return nil
}

// NewDNSMonitor returns a new Monitor
func NewDNSMonitor(manager *manager.Manager, statsdClient statsd.ClientInterface) (*Monitor, error) {
	numCPU, err := utils.NumCPU()
	if err != nil {
		return nil, fmt.Errorf("couldn't fetch the host CPU count: %w", err)
	}

	d := &Monitor{
		statsdClient: statsdClient,
		numCPU:       numCPU,
		statsZero:    make([]Stats, numCPU),
	}

	statsFB, err := managerhelper.Map(manager, "fb_dns_stats")
	if err != nil {
		return nil, fmt.Errorf("failed to get front buffer map: %w", err)
	}
	d.stats[0] = statsFB

	statsBB, err := managerhelper.Map(manager, "bb_dns_stats")
	if err != nil {
		return nil, fmt.Errorf("failed to get back buffer map: %w", err)
	}
	d.stats[1] = statsBB

	bufferSelector, err := managerhelper.Map(manager, "buffer_selector")
	if err != nil {
		return nil, fmt.Errorf("failed to get buffer selector map: %w", err)
	}
	d.bufferSelector = bufferSelector

	return d, nil
}
