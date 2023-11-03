// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux

// Package bpfmap holds eBPF map monitoring related files
package bpfmap

import (
	"math"
	"sync"

	"github.com/DataDog/datadog-agent/pkg/security/metrics"
	"github.com/DataDog/datadog-agent/pkg/security/probe/managerhelper"
	"github.com/DataDog/datadog-go/v5/statsd"
	manager "github.com/DataDog/ebpf-manager"
	lib "github.com/cilium/ebpf"
	"golang.org/x/exp/maps"
)

// MapStats represents eBPF LRU cache stats
type MapStats struct {
	Hits uint32
	Miss uint32
}

// Monitor defines an eBPF map monitor
type Monitor struct {
	statsdClient  statsd.ClientInterface
	statsMap      *lib.Map
	lastStats     map[string]MapStats
	lastStatsLock sync.Mutex
}

func monitoredMapNames() []string {
	return []string{
		"proc_cache",
		"pid_cache",
	}
}

func (m *Monitor) collectStats() map[string]MapStats {
	mapNames := monitoredMapNames()
	diffStatsByMap := make(map[string]MapStats, len(mapNames))

	var mapIndex uint32
	var currStats MapStats
	var diffStats MapStats
	m.lastStatsLock.Lock()
	for i, name := range mapNames {
		mapIndex = uint32(i)
		if err := m.statsMap.Lookup(&mapIndex, &currStats); err != nil {
			continue
		}

		if currStats.Hits < m.lastStats[name].Hits {
			diffStats.Hits = (math.MaxUint32 - m.lastStats[name].Hits) + currStats.Hits + 1
		} else {
			diffStats.Hits = currStats.Hits - m.lastStats[name].Hits
		}

		if currStats.Miss < m.lastStats[name].Miss {
			diffStats.Miss = (math.MaxUint32 - m.lastStats[name].Miss) + currStats.Miss + 1
		} else {
			diffStats.Miss = currStats.Miss - m.lastStats[name].Miss
		}

		diffStatsByMap[name] = diffStats
		m.lastStats[name] = currStats
	}
	m.lastStatsLock.Unlock()

	return diffStatsByMap
}

// SendStats send stats
func (m *Monitor) SendStats() {
	diffStatsByMap := m.collectStats()
	for name, stats := range diffStatsByMap {
		tags := []string{
			"map_name:" + name,
		}
		_ = m.statsdClient.Count(metrics.MetricBPFMapHits, int64(stats.Hits), tags, 1.0)
		_ = m.statsdClient.Count(metrics.MetricBPFMapMiss, int64(stats.Miss), tags, 1.0)
	}
}

// GetMapsStats collects and returns MapStats for each eBPF map monitored
func (m *Monitor) GetMapsStats() map[string]MapStats {
	_ = m.collectStats()
	m.lastStatsLock.Lock()
	defer m.lastStatsLock.Unlock()

	return maps.Clone(m.lastStats)
}

// NewBPFMapMonitor instanciates a new BPF monitor
func NewBPFMapMonitor(manager *manager.Manager, statsdClient statsd.ClientInterface) (*Monitor, error) {
	mapNames := monitoredMapNames()
	monitor := &Monitor{
		statsdClient: statsdClient,
		lastStats:    make(map[string]MapStats, len(mapNames)),
	}

	for _, name := range mapNames {
		monitor.lastStats[name] = MapStats{}
	}

	statsMap, err := managerhelper.Map(manager, "bpf_lru_stats")
	if err != nil {
		return nil, err
	}
	monitor.statsMap = statsMap

	return monitor, nil
}

func makeConstName(mapName string) string {
	return mapName + "_telemetry_key"
}

// MonitoredMapConstants returns the list of constants describing eBPF LRU maps to be monitored
func MonitoredMapConstants() []manager.ConstantEditor {
	monitoredMapNames := monitoredMapNames()

	constants := make([]manager.ConstantEditor, 0, len(monitoredMapNames))
	for i, name := range monitoredMapNames {
		constants = append(constants, manager.ConstantEditor{
			Name:  makeConstName(name),
			Value: uint64(i),
		})
	}

	return constants
}
