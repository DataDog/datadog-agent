// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package metrics

import (
	"sync"
)

const (
	ewmaAlpha      = 2 / (float64(30) + 1) // ~0.0645 — 30-second smoothing window
	shortEwmaAlpha = 2 / (float64(15) + 1) // ~0.125 — 15-second smoothing window
)

// ComponentSnapshot holds the most-recently-reported utilization and capacity metrics
// for a single named component instance. Written by monitors on each report cycle;
// read by the status builder.
type ComponentSnapshot struct {
	Name     string
	Instance string
	// AvgRatio is the N=30 EWMA-smoothed utilization ratio (~30-second window).
	AvgRatio float64
	// RawRatio is the instantaneous ratio over the last 1-second sample window (pre-EWMA).
	RawRatio float64
	// ShortAvgRatio is the N=15 EWMA-smoothed utilization ratio (~15-second window).
	// Responds to sustained saturation within ~22 seconds, compared to ~45 seconds for AvgRatio.
	ShortAvgRatio float64
	// AvgItems is the N=30 EWMA-smoothed count of items held in the component's buffers.
	AvgItems float64
	// RawItems is the raw item count at the last capacity sample (ingress - egress).
	RawItems int64
	// AvgBytes is the N=30 EWMA-smoothed byte count.
	AvgBytes float64
	// RawBytes is the raw byte count at the last capacity sample.
	RawBytes int64
	// Windows contains rolling-window statistics for backpressure diagnostics.
	Windows WindowStats
}

var (
	globalSnapshotsMu sync.RWMutex
	globalSnapshots   = map[string]*ComponentSnapshot{}
)

// setComponentUtilization updates the utilization fields of the snapshot for name:instance.
// Called from TelemetryUtilizationMonitor on each report tick.
func setComponentUtilization(name, instance string, avgRatio, rawRatio, shortAvgRatio float64, ws WindowStats) {
	key := name + ":" + instance
	globalSnapshotsMu.Lock()
	defer globalSnapshotsMu.Unlock()
	s := globalSnapshots[key]
	if s == nil {
		s = &ComponentSnapshot{Name: name, Instance: instance}
		globalSnapshots[key] = s
	}
	s.AvgRatio = avgRatio
	s.RawRatio = rawRatio
	s.ShortAvgRatio = shortAvgRatio
	s.Windows = ws
}

// setComponentCapacity updates the capacity fields of the snapshot for name:instance.
// Called from CapacityMonitor on each report tick.
func setComponentCapacity(name, instance string, avgItems, avgBytes float64, rawItems, rawBytes int64) {
	key := name + ":" + instance
	globalSnapshotsMu.Lock()
	defer globalSnapshotsMu.Unlock()
	s := globalSnapshots[key]
	if s == nil {
		s = &ComponentSnapshot{Name: name, Instance: instance}
		globalSnapshots[key] = s
	}
	s.AvgItems = avgItems
	s.AvgBytes = avgBytes
	s.RawItems = rawItems
	s.RawBytes = rawBytes
}

// GlobalComponentSnapshots returns a copy of all current component snapshots.
func GlobalComponentSnapshots() []ComponentSnapshot {
	globalSnapshotsMu.RLock()
	defer globalSnapshotsMu.RUnlock()
	result := make([]ComponentSnapshot, 0, len(globalSnapshots))
	for _, s := range globalSnapshots {
		result = append(result, *s)
	}
	return result
}

// ClearComponentSnapshots removes all stored component snapshots.
// Called when the logs pipeline stops so stale entries don't outlive the pipeline.
func ClearComponentSnapshots() {
	globalSnapshotsMu.Lock()
	defer globalSnapshotsMu.Unlock()
	globalSnapshots = map[string]*ComponentSnapshot{}
}

const (

	// ProcessorTlmName is the telemetry name for processor components
	ProcessorTlmName = "processor"
	// StrategyTlmName is the telemetry name for strategy components
	StrategyTlmName = "strategy"
	// SenderTlmName is the telemetry name for sender components
	SenderTlmName = "sender"
	// WorkerTlmName is the telemetry name for worker components
	WorkerTlmName = "worker"
	// SenderTlmInstanceID is the default instance ID for sender components
	SenderTlmInstanceID = "0"
)

// MeasurablePayload represents a payload that can be measured in bytes and count
type MeasurablePayload interface {
	Size() int64
	Count() int64
}

// PipelineMonitor is an interface for monitoring the capacity of a pipeline.
// Pipeline monitors are used to measure both capacity and utilization of components.
type PipelineMonitor interface {
	GetCapacityMonitor(name string, instanceID string) *CapacityMonitor
	ReportComponentIngress(size MeasurablePayload, name string, instanceID string)
	ReportComponentEgress(size MeasurablePayload, name string, instanceID string)
	MakeUtilizationMonitor(name string, instanceID string) UtilizationMonitor
}

// NoopPipelineMonitor is a no-op implementation of PipelineMonitor.
// Some instances of logs components do not need to report capacity metrics and
// should use this implementation.
type NoopPipelineMonitor struct {
	instanceID string
}

// NewNoopPipelineMonitor creates a new no-op pipeline monitor
func NewNoopPipelineMonitor(instanceID string) *NoopPipelineMonitor {
	return &NoopPipelineMonitor{
		instanceID: instanceID,
	}
}

// GetCapacityMonitor returns the capacity monitor for the given name and instance ID.
// Returns nil for NoopPipelineMonitor as it doesn't track capacity.
func (n *NoopPipelineMonitor) GetCapacityMonitor(_ string, _ string) *CapacityMonitor {
	return nil
}

// ReportComponentIngress does nothing.
func (n *NoopPipelineMonitor) ReportComponentIngress(_ MeasurablePayload, _ string, _ string) {}

// ReportComponentEgress does nothing.
func (n *NoopPipelineMonitor) ReportComponentEgress(_ MeasurablePayload, _ string, _ string) {}

// MakeUtilizationMonitor returns a no-op utilization monitor.
func (n *NoopPipelineMonitor) MakeUtilizationMonitor(_ string, _ string) UtilizationMonitor {
	return &NoopUtilizationMonitor{}
}

// TelemetryPipelineMonitor is a PipelineMonitor that reports capacity metrics to telemetry
type TelemetryPipelineMonitor struct {
	monitors map[string]*CapacityMonitor
	lock     sync.RWMutex
}

// NewTelemetryPipelineMonitor creates a new pipeline monitort that reports capacity and utiilization metrics as telemetry
func NewTelemetryPipelineMonitor() *TelemetryPipelineMonitor {
	return &TelemetryPipelineMonitor{
		monitors: make(map[string]*CapacityMonitor),
		lock:     sync.RWMutex{},
	}
}

func (c *TelemetryPipelineMonitor) getMonitor(name string, instanceID string) *CapacityMonitor {
	key := name + instanceID

	c.lock.RLock()
	monitor, exists := c.monitors[key]
	c.lock.RUnlock()

	if !exists {
		c.lock.Lock()
		if c.monitors[key] == nil {
			c.monitors[key] = NewCapacityMonitor(name, instanceID)
		}
		monitor = c.monitors[key]
		c.lock.Unlock()
	}

	return monitor
}

// MakeUtilizationMonitor creates a new utilization monitor for a component.
func (c *TelemetryPipelineMonitor) MakeUtilizationMonitor(name string, instanceID string) UtilizationMonitor {
	return NewTelemetryUtilizationMonitor(name, instanceID)
}

// GetCapacityMonitor returns the capacity monitor for the given name and instance ID.
func (c *TelemetryPipelineMonitor) GetCapacityMonitor(name string, instanceID string) *CapacityMonitor {
	return c.getMonitor(name, instanceID)
}

// ReportComponentIngress reports the ingress of a payload to a component.
func (c *TelemetryPipelineMonitor) ReportComponentIngress(pl MeasurablePayload, name string, instanceID string) {
	c.getMonitor(name, instanceID).AddIngress(pl)
}

// ReportComponentEgress reports the egress of a payload from a component.
func (c *TelemetryPipelineMonitor) ReportComponentEgress(pl MeasurablePayload, name string, instanceID string) {
	c.getMonitor(name, instanceID).AddEgress(pl)
}
