// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package metrics

import (
	"sync"
	"time"

	"github.com/benbjohnson/clock"
)

const ewmaAlpha = 2 / (float64(15) + 1) // ~0.125 — 15-second smoothing window

// ComponentSnapshot is the latest utilization/capacity sample for a component (Avg* are N=15 EWMA, Raw* instantaneous).
type ComponentSnapshot struct {
	Name     string
	Instance string
	AvgRatio float64
	RawRatio float64
	AvgItems float64
	RawItems int64
	AvgBytes float64
	RawBytes int64
	Windows  WindowStats

	// history is the live rolling history; Windows is recomputed from it at read time, and it is
	// cleared on returned copies so the pointer never escapes this package.
	history *rollingHistory
}

// snapshotRegistry holds the latest per-component snapshots for a single pipeline monitor.
// Each TelemetryPipelineMonitor owns one, so component keys (name:instance) from different
// pipelines can never collide, and the snapshots live and die with their owning monitor (no
// global clear step is needed). Pipelines whose telemetry shouldn't surface on the status page
// simply use a NoopPipelineMonitor, which owns no registry.
type snapshotRegistry struct {
	mu        sync.RWMutex
	snapshots map[string]*ComponentSnapshot
}

func newSnapshotRegistry() *snapshotRegistry {
	return &snapshotRegistry{snapshots: map[string]*ComponentSnapshot{}}
}

// setUtilization stores the utilization fields and live history for name:instance.
func (r *snapshotRegistry) setUtilization(name, instance string, avgRatio, rawRatio float64, history *rollingHistory) {
	key := name + ":" + instance
	r.mu.Lock()
	defer r.mu.Unlock()
	s := r.snapshots[key]
	if s == nil {
		s = &ComponentSnapshot{Name: name, Instance: instance}
		r.snapshots[key] = s
	}
	s.AvgRatio = avgRatio
	s.RawRatio = rawRatio
	s.history = history
}

// setCapacity stores the capacity fields for name:instance.
func (r *snapshotRegistry) setCapacity(name, instance string, avgItems, avgBytes float64, rawItems, rawBytes int64) {
	key := name + ":" + instance
	r.mu.Lock()
	defer r.mu.Unlock()
	s := r.snapshots[key]
	if s == nil {
		s = &ComponentSnapshot{Name: name, Instance: instance}
		r.snapshots[key] = s
	}
	s.AvgItems = avgItems
	s.AvgBytes = avgBytes
	s.RawItems = rawItems
	s.RawBytes = rawBytes
}

// at copies all snapshots, recomputing window stats against now (so idle components decay).
func (r *snapshotRegistry) at(now time.Time) []ComponentSnapshot {
	r.mu.RLock()
	defer r.mu.RUnlock()
	result := make([]ComponentSnapshot, 0, len(r.snapshots))
	for _, s := range r.snapshots {
		snap := *s
		if s.history != nil {
			snap.Windows = s.history.allStats(now)
		}
		snap.history = nil
		result = append(result, snap)
	}
	return result
}

// lookup returns the snapshot for name:instance, or false if none exists.
func (r *snapshotRegistry) lookup(name, instance string) (ComponentSnapshot, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	s := r.snapshots[name+":"+instance]
	if s == nil {
		return ComponentSnapshot{}, false
	}
	return *s, true
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
	// Start/Stop bracket periodic sampling of this monitor's utilization monitors.
	Start()
	Stop()
	// Snapshots returns the current per-component utilization/capacity snapshots owned by
	// this monitor. NoopPipelineMonitor returns nil, so pipelines that should not surface on
	// the status page contribute nothing.
	Snapshots() []ComponentSnapshot
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

// Start does nothing.
func (n *NoopPipelineMonitor) Start() {}

// Stop does nothing.
func (n *NoopPipelineMonitor) Stop() {}

// Snapshots returns nil; a no-op monitor owns no registry.
func (n *NoopPipelineMonitor) Snapshots() []ComponentSnapshot { return nil }

// utilizationSampleInterval is how often the ticker samples each monitor, so mid-operation blocks are observed.
const utilizationSampleInterval = 1 * time.Second

// TelemetryPipelineMonitor is a PipelineMonitor that reports capacity metrics to telemetry
type TelemetryPipelineMonitor struct {
	monitors map[string]*CapacityMonitor
	lock     sync.RWMutex

	// registry holds the snapshots for the components created by this monitor. It is owned here
	// so its keys never collide with another pipeline's, and it is read back via Snapshots().
	registry *snapshotRegistry

	// utilizationMonitors are ticked by sampleLoop so a component blocked between Start and Stop is still sampled.
	utilizationMonitors map[string]*TelemetryUtilizationMonitor
	clock               clock.Clock
	sampleInterval      time.Duration
	stopCh              chan struct{}
	doneCh              chan struct{}
	started             bool
}

// NewTelemetryPipelineMonitor creates a new pipeline monitor that reports capacity and utilization metrics as telemetry
func NewTelemetryPipelineMonitor() *TelemetryPipelineMonitor {
	return newTelemetryPipelineMonitorWithClock(clock.New(), utilizationSampleInterval)
}

func newTelemetryPipelineMonitorWithClock(clk clock.Clock, sampleInterval time.Duration) *TelemetryPipelineMonitor {
	return &TelemetryPipelineMonitor{
		monitors:            make(map[string]*CapacityMonitor),
		utilizationMonitors: make(map[string]*TelemetryUtilizationMonitor),
		lock:                sync.RWMutex{},
		registry:            newSnapshotRegistry(),
		clock:               clk,
		sampleInterval:      sampleInterval,
	}
}

// Snapshots returns a copy of this monitor's current component snapshots, window stats
// recomputed against the current time so idle components decay.
func (c *TelemetryPipelineMonitor) Snapshots() []ComponentSnapshot {
	return c.registry.at(time.Now())
}

func (c *TelemetryPipelineMonitor) getMonitor(name string, instanceID string) *CapacityMonitor {
	key := name + instanceID

	c.lock.RLock()
	monitor, exists := c.monitors[key]
	c.lock.RUnlock()

	if !exists {
		c.lock.Lock()
		if c.monitors[key] == nil {
			c.monitors[key] = newCapacityMonitor(name, instanceID, c.registry)
		}
		monitor = c.monitors[key]
		c.lock.Unlock()
	}

	return monitor
}

// MakeUtilizationMonitor creates a utilization monitor and registers it for periodic sampling.
func (c *TelemetryPipelineMonitor) MakeUtilizationMonitor(name string, instanceID string) UtilizationMonitor {
	m := newTelemetryUtilizationMonitorWithSampleRateAndClock(name, instanceID, c.sampleInterval, c.clock, c.registry)
	c.lock.Lock()
	c.utilizationMonitors[name+":"+instanceID] = m
	c.lock.Unlock()
	return m
}

// Start launches the periodic sampling loop. Idempotent.
func (c *TelemetryPipelineMonitor) Start() {
	c.lock.Lock()
	defer c.lock.Unlock()
	if c.started {
		return
	}
	c.started = true
	c.stopCh = make(chan struct{})
	c.doneCh = make(chan struct{})
	go c.sampleLoop(c.stopCh, c.doneCh)
}

// Stop ends the sampling loop and blocks until the goroutine has exited. Idempotent.
func (c *TelemetryPipelineMonitor) Stop() {
	c.lock.Lock()
	if !c.started {
		c.lock.Unlock()
		return
	}
	c.started = false
	close(c.stopCh)
	done := c.doneCh
	c.lock.Unlock()
	<-done
}

func (c *TelemetryPipelineMonitor) sampleLoop(stop, done chan struct{}) {
	defer close(done)
	ticker := c.clock.Ticker(c.sampleInterval)
	defer ticker.Stop()
	for {
		select {
		case <-stop:
			return
		case <-ticker.C:
			now := c.clock.Now()
			c.lock.RLock()
			monitors := make([]*TelemetryUtilizationMonitor, 0, len(c.utilizationMonitors))
			for _, m := range c.utilizationMonitors {
				monitors = append(monitors, m)
			}
			c.lock.RUnlock()
			for _, m := range monitors {
				m.sample(now)
			}
		}
	}
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
