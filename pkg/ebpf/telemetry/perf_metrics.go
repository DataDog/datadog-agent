// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux

package telemetry

import (
	"slices"
	"strconv"
	"sync"

	manager "github.com/DataDog/ebpf-manager"
	"github.com/cilium/ebpf"
	"github.com/prometheus/client_golang/prometheus"
)

var (
	perfCollector *perfUsageCollector
)

type perfUsageCollector struct {
	mtx      sync.Mutex
	usage    *prometheus.GaugeVec
	usagePct *prometheus.GaugeVec
	size     *prometheus.GaugeVec
	lost     *prometheus.CounterVec

	perfMaps    []*manager.PerfMap
	ringBuffers []*manager.RingBuffer
}

// NewPerfUsageCollector creates a prometheus.Collector for perf buffer and ring buffer metrics
func NewPerfUsageCollector() prometheus.Collector {
	perfCollector = &perfUsageCollector{
		usage: prometheus.NewGaugeVec(
			prometheus.GaugeOpts{
				Subsystem: "ebpf__perf",
				Name:      "_usage",
				Help:      "gauge tracking bytes usage of a perf buffer (per-cpu) or ring buffer",
			},
			[]string{"map_name", "map_type", "cpu_num"},
		),
		usagePct: prometheus.NewGaugeVec(
			prometheus.GaugeOpts{
				Subsystem: "ebpf__perf",
				Name:      "_usage_pct",
				Help:      "gauge tracking percentage usage of a perf buffer (per-cpu) or ring buffer",
			},
			[]string{"map_name", "map_type", "cpu_num"},
		),
		size: prometheus.NewGaugeVec(
			prometheus.GaugeOpts{
				Subsystem: "ebpf__perf",
				Name:      "_size",
				Help:      "gauge tracking total size of a perf buffer (per-cpu) or ring buffer",
			},
			[]string{"map_name", "map_type", "cpu_num"},
		),
		lost: prometheus.NewCounterVec(
			prometheus.CounterOpts{
				Subsystem: "ebpf__perf",
				Name:      "_lost",
				Help:      "counter tracking lost samples of a perf buffer (per-cpu)",
			},
			[]string{"map_name", "map_type", "cpu_num"},
		),
	}
	return perfCollector
}

// Describe implements prometheus.Collector.Describe
func (p *perfUsageCollector) Describe(descs chan<- *prometheus.Desc) {
	p.usage.Describe(descs)
	p.size.Describe(descs)
	p.usagePct.Describe(descs)
	p.lost.Describe(descs)
}

// Collect implements prometheus.Collector.Collect
func (p *perfUsageCollector) Collect(metrics chan<- prometheus.Metric) {
	p.mtx.Lock()
	defer p.mtx.Unlock()

	for _, pm := range p.perfMaps {
		mapName, mapType := pm.Name, ebpf.PerfEventArray.String()
		size := float64(pm.BufferSize())
		usage, lost := pm.Telemetry()
		if usage == nil || lost == nil {
			continue
		}

		for cpu := range usage {
			cpuString := strconv.Itoa(cpu)

			count := float64(usage[cpu])
			p.usage.WithLabelValues(mapName, mapType, cpuString).Set(count)
			p.usagePct.WithLabelValues(mapName, mapType, cpuString).Set(100 * (count / size))
			p.size.WithLabelValues(mapName, mapType, cpuString).Set(size)
			p.lost.WithLabelValues(mapName, mapType, cpuString).Add(float64(lost[cpu]))
		}
	}

	for _, rb := range p.ringBuffers {
		mapName, mapType := rb.Name, ebpf.RingBuf.String()
		size := float64(rb.BufferSize())
		usage, ok := rb.Telemetry()
		if !ok {
			continue
		}

		cpuString := "0"
		count := float64(usage)
		p.usage.WithLabelValues(mapName, mapType, cpuString).Set(count)
		p.usagePct.WithLabelValues(mapName, mapType, cpuString).Set(100 * (count / size))
		p.size.WithLabelValues(mapName, mapType, cpuString).Set(size)
	}

	p.usage.Collect(metrics)
	p.usagePct.Collect(metrics)
	p.size.Collect(metrics)
	p.lost.Collect(metrics)
}

// ReportPerfMapTelemetry starts reporting the telemetry for the provided PerfMap
func ReportPerfMapTelemetry(pm *manager.PerfMap) {
	if perfCollector == nil {
		return
	}
	perfCollector.registerPerfMap(pm)
}

// ReportRingBufferTelemetry starts reporting the telemetry for the provided RingBuffer
func ReportRingBufferTelemetry(rb *manager.RingBuffer) {
	if perfCollector == nil {
		return
	}
	perfCollector.registerRingBuffer(rb)
}

func (p *perfUsageCollector) registerPerfMap(pm *manager.PerfMap) {
	if !pm.TelemetryEnabled {
		return
	}
	p.mtx.Lock()
	defer p.mtx.Unlock()
	p.perfMaps = append(p.perfMaps, pm)
}

func (p *perfUsageCollector) registerRingBuffer(rb *manager.RingBuffer) {
	if !rb.TelemetryEnabled {
		return
	}
	p.mtx.Lock()
	defer p.mtx.Unlock()
	p.ringBuffers = append(p.ringBuffers, rb)
}

// UnregisterTelemetry unregisters the PerfMap and RingBuffers from telemetry
func UnregisterTelemetry(m *manager.Manager) {
	if perfCollector == nil {
		return
	}
	perfCollector.unregisterTelemetry(m)
}

func (p *perfUsageCollector) unregisterTelemetry(m *manager.Manager) {
	p.mtx.Lock()
	defer p.mtx.Unlock()
	p.perfMaps = slices.DeleteFunc(p.perfMaps, func(perfMap *manager.PerfMap) bool {
		return slices.Contains(m.PerfMaps, perfMap)
	})
	p.ringBuffers = slices.DeleteFunc(p.ringBuffers, func(ringBuf *manager.RingBuffer) bool {
		return slices.Contains(m.RingBuffers, ringBuf)
	})
}
