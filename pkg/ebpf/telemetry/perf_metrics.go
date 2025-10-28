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

	pkgconfigsetup "github.com/DataDog/datadog-agent/pkg/config/setup"
)

var (
	perfCollector *perfUsageCollector
)

type perfUsageCollector struct {
	emitPerCPU bool
	mtx        sync.Mutex
	usage      *prometheus.GaugeVec
	usagePct   *prometheus.GaugeVec
	size       *prometheus.GaugeVec
	lost       *prometheus.CounterVec
	channelLen *prometheus.GaugeVec

	perfMaps            []*manager.PerfMap
	perfChannelLenFuncs map[*manager.PerfMap]func() int

	ringBuffers         []*manager.RingBuffer
	ringChannelLenFuncs map[*manager.RingBuffer]func() int
}

// NewPerfUsageCollector creates a prometheus.Collector for perf buffer and ring buffer metrics
func NewPerfUsageCollector() prometheus.Collector {
	emitPerCPU := pkgconfigsetup.SystemProbe().GetBool("system_probe_config.telemetry_perf_buffer_emit_per_cpu")

	labels := []string{"map_name", "map_type"}
	if emitPerCPU {
		labels = append(labels, "cpu_num")
	}

	perfCollector = &perfUsageCollector{
		emitPerCPU:          emitPerCPU,
		perfChannelLenFuncs: make(map[*manager.PerfMap]func() int),
		ringChannelLenFuncs: make(map[*manager.RingBuffer]func() int),
		usage: prometheus.NewGaugeVec(
			prometheus.GaugeOpts{
				Subsystem: "ebpf__perf",
				Name:      "_usage",
				Help:      "gauge tracking bytes usage of a perf buffer (per-cpu, if enabled) or ring buffer",
			},
			labels,
		),
		usagePct: prometheus.NewGaugeVec(
			prometheus.GaugeOpts{
				Subsystem: "ebpf__perf",
				Name:      "_usage_pct",
				Help:      "gauge tracking percentage usage of a perf buffer (per-cpu, if enabled) or ring buffer",
			},
			labels,
		),
		size: prometheus.NewGaugeVec(
			prometheus.GaugeOpts{
				Subsystem: "ebpf__perf",
				Name:      "_size",
				Help:      "gauge tracking total size of a perf buffer (per-cpu, if enabled) or ring buffer",
			},
			labels,
		),
		lost: prometheus.NewCounterVec(
			prometheus.CounterOpts{
				Subsystem: "ebpf__perf",
				Name:      "_lost",
				Help:      "counter tracking lost samples of a perf buffer (per-cpu, if enabled)",
			},
			labels,
		),
		channelLen: prometheus.NewGaugeVec(
			prometheus.GaugeOpts{
				Subsystem: "ebpf__perf",
				Name:      "_channel_len",
				Help:      "gauge tracking number of elements in buffer channel",
			},
			[]string{"map_name", "map_type"},
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

		if p.emitPerCPU {
			for cpu := range usage {
				cpuString := strconv.Itoa(cpu)

				count := float64(usage[cpu])
				p.usage.WithLabelValues(mapName, mapType, cpuString).Set(count)
				p.usagePct.WithLabelValues(mapName, mapType, cpuString).Set(100 * (count / size))
				p.size.WithLabelValues(mapName, mapType, cpuString).Set(size)
				p.lost.WithLabelValues(mapName, mapType, cpuString).Add(float64(lost[cpu]))
			}
		} else {
			totalCount, totalLost := uint64(0), uint64(0)
			for cpu := range usage {
				totalCount += usage[cpu]
				totalLost += lost[cpu]
			}

			p.usage.WithLabelValues(mapName, mapType).Set(float64(totalCount))
			p.usagePct.WithLabelValues(mapName, mapType).Set(100 * (float64(totalCount) / size))
			p.size.WithLabelValues(mapName, mapType).Set(size)
			p.lost.WithLabelValues(mapName, mapType).Add(float64(totalLost))
		}
	}

	for pm, chFunc := range p.perfChannelLenFuncs {
		mapName, mapType := pm.Name, ebpf.PerfEventArray.String()
		p.channelLen.WithLabelValues(mapName, mapType).Set(float64(chFunc()))
	}

	for _, rb := range p.ringBuffers {
		mapName, mapType := rb.Name, ebpf.RingBuf.String()
		size := float64(rb.BufferSize())
		usage, ok := rb.Telemetry()
		if !ok {
			continue
		}

		labels := []string{mapName, mapType}
		if p.emitPerCPU {
			labels = append(labels, "0")
		}

		count := float64(usage)
		p.usage.WithLabelValues(labels...).Set(count)
		p.usagePct.WithLabelValues(labels...).Set(100 * (count / size))
		p.size.WithLabelValues(labels...).Set(size)
	}

	for rb, chFunc := range p.ringChannelLenFuncs {
		mapName, mapType := rb.Name, ebpf.RingBuf.String()
		p.channelLen.WithLabelValues(mapName, mapType).Set(float64(chFunc()))
	}

	p.usage.Collect(metrics)
	p.usagePct.Collect(metrics)
	p.size.Collect(metrics)
	p.lost.Collect(metrics)
	p.channelLen.Collect(metrics)
}

// ReportPerfMapTelemetry starts reporting the telemetry for the provided PerfMap
func ReportPerfMapTelemetry(pm *manager.PerfMap) {
	if perfCollector == nil {
		return
	}
	perfCollector.registerPerfMap(pm)
}

// ReportPerfMapChannelLenTelemetry starts reporting the telemetry for the provided PerfMap's buffer channel
func ReportPerfMapChannelLenTelemetry(pm *manager.PerfMap, channelLenFunc func() int) {
	if perfCollector == nil {
		return
	}
	perfCollector.registerPerfMapChannel(pm, channelLenFunc)
}

// ReportRingBufferTelemetry starts reporting the telemetry for the provided RingBuffer
func ReportRingBufferTelemetry(rb *manager.RingBuffer) {
	if perfCollector == nil {
		return
	}
	perfCollector.registerRingBuffer(rb)
}

// ReportRingBufferChannelLenTelemetry starts reporting the telemetry for the provided RingBuffer's buffer channel
func ReportRingBufferChannelLenTelemetry(rb *manager.RingBuffer, channelLenFunc func() int) {
	if perfCollector == nil {
		return
	}
	perfCollector.registerRingBufferChannel(rb, channelLenFunc)
}

func (p *perfUsageCollector) registerPerfMap(pm *manager.PerfMap) {
	if !pm.TelemetryEnabled {
		return
	}
	p.mtx.Lock()
	defer p.mtx.Unlock()
	p.perfMaps = append(p.perfMaps, pm)
}

func (p *perfUsageCollector) registerPerfMapChannel(pm *manager.PerfMap, channelLenFunc func() int) {
	if !pm.TelemetryEnabled {
		return
	}
	p.mtx.Lock()
	defer p.mtx.Unlock()
	p.perfChannelLenFuncs[pm] = channelLenFunc
}

func (p *perfUsageCollector) registerRingBuffer(rb *manager.RingBuffer) {
	if !rb.TelemetryEnabled {
		return
	}
	p.mtx.Lock()
	defer p.mtx.Unlock()
	p.ringBuffers = append(p.ringBuffers, rb)
}

func (p *perfUsageCollector) registerRingBufferChannel(rb *manager.RingBuffer, channelLenFunc func() int) {
	if !rb.TelemetryEnabled {
		return
	}
	p.mtx.Lock()
	defer p.mtx.Unlock()
	p.ringChannelLenFuncs[rb] = channelLenFunc
}

// UnregisterTelemetry unregisters the PerfMap and RingBuffers from telemetry
func UnregisterTelemetry(m *manager.Manager) {
	if perfCollector != nil {
		perfCollector.unregisterTelemetry(m)
	}
}

func (p *perfUsageCollector) unregisterTelemetry(m *manager.Manager) {
	p.mtx.Lock()
	defer p.mtx.Unlock()
	p.perfMaps = slices.DeleteFunc(p.perfMaps, func(perfMap *manager.PerfMap) bool {
		return slices.Contains(m.PerfMaps, perfMap)
	})
	for _, pm := range m.PerfMaps {
		delete(p.perfChannelLenFuncs, pm)
	}
	p.ringBuffers = slices.DeleteFunc(p.ringBuffers, func(ringBuf *manager.RingBuffer) bool {
		return slices.Contains(m.RingBuffers, ringBuf)
	})
	for _, rb := range m.RingBuffers {
		delete(p.ringChannelLenFuncs, rb)
	}
}
