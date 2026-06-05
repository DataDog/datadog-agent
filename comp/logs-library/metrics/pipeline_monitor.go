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

// ComponentSnapshot holds the most-recently-reported utilization and capacity metrics
// for a single named component instance. Written by monitors on each report cycle;
// read by the status builder.
type ComponentSnapshot struct {
	Name     string
	Instance string
	// AvgRatio is the N=15 EWMA-smoothed utilization ratio (~15-second window).
	AvgRatio float64
	// RawRatio is the instantaneous ratio over the last 1-second sample window (pre-EWMA).
	RawRatio float64
	// AvgItems is the N=15 EWMA-smoothed count of items held in the component's buffers.
	AvgItems float64
	// RawItems is the raw item count at the last capacity sample (ingress - egress).
	RawItems int64
	// AvgBytes is the N=15 EWMA-smoothed byte count.
	AvgBytes float64
	// RawBytes is the raw byte count at the last capacity sample.
	RawBytes int64
	// Windows contains rolling-window statistics for backpressure diagnostics. When the
	// snapshot carries a live history (utilization components), Windows is recomputed at
	// read-time against the current clock so idle components decay correctly.
	Windows WindowStats

	// history, when non-nil, is the live rolling history for this component. Utilization is
	// sampled only when the component does work (Start/Stop), so an idle component's stored
	// stats would otherwise freeze. Recomputing Windows from history against the read-time
	// clock lets the windowed maxes, saturation counts, and CurrentlySaturated all decay.
	// Cleared on the copies returned to callers so the pointer never escapes this package.
	history *rollingHistory
}

var (
	globalSnapshotsMu sync.RWMutex
	globalSnapshots   = map[string]*ComponentSnapshot{}
)

// setComponentUtilization updates the utilization fields of the snapshot for name:instance.
// Called from TelemetryUtilizationMonitor on each report tick. The history pointer is stored
// (not a precomputed WindowStats) so window stats can be recomputed against the read-time
// clock — see globalComponentSnapshotsAt.
func setComponentUtilization(name, instance string, avgRatio, rawRatio float64, history *rollingHistory) {
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
	s.history = history
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

// GlobalComponentSnapshots returns a copy of all current component snapshots, with window
// stats recomputed against the current wall clock.
func GlobalComponentSnapshots() []ComponentSnapshot {
	return globalComponentSnapshotsAt(time.Now())
}

// globalComponentSnapshotsAt returns a copy of all current component snapshots. For snapshots
// carrying a live history the window stats are recomputed against now, so an idle component's
// windowed maxes, saturation counts, and CurrentlySaturated decay instead of freezing at the
// last sampled value. The history pointer is stripped from the returned copies.
func globalComponentSnapshotsAt(now time.Time) []ComponentSnapshot {
	globalSnapshotsMu.RLock()
	defer globalSnapshotsMu.RUnlock()
	result := make([]ComponentSnapshot, 0, len(globalSnapshots))
	for _, s := range globalSnapshots {
		snap := *s
		if s.history != nil {
			snap.Windows = s.history.allStats(now)
		}
		snap.history = nil
		result = append(result, snap)
	}
	return result
}

// lookupComponentSnapshot returns the current snapshot for a single component instance.
// Returns the zero value and false if no snapshot exists for the given name:instance pair.
func lookupComponentSnapshot(name, instance string) (ComponentSnapshot, bool) {
	globalSnapshotsMu.RLock()
	defer globalSnapshotsMu.RUnlock()
	s := globalSnapshots[name+":"+instance]
	if s == nil {
		return ComponentSnapshot{}, false
	}
	return *s, true
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
	// Start begins periodic sampling of the utilization monitors this pipeline monitor
	// created; Stop ends it. They bracket the pipeline's lifetime (wired into Sender).
	Start()
	Stop()
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

// utilizationSampleInterval is how often the pipeline monitor's ticker samples each
// utilization monitor so in-flight blocking operations are observed between Start/Stop.
const utilizationSampleInterval = 1 * time.Second

// TelemetryPipelineMonitor is a PipelineMonitor that reports capacity metrics to telemetry
type TelemetryPipelineMonitor struct {
	monitors map[string]*CapacityMonitor
	lock     sync.RWMutex

	// utilizationMonitors are the monitors created by MakeUtilizationMonitor; the sampleLoop
	// ticks each one so a component blocked between Start and Stop is still sampled.
	utilizationMonitors []*TelemetryUtilizationMonitor
	clock               clock.Clock
	sampleInterval      time.Duration
	stopCh              chan struct{}
	doneCh              chan struct{}
	started             bool
}

// NewTelemetryPipelineMonitor creates a new pipeline monitort that reports capacity and utiilization metrics as telemetry
func NewTelemetryPipelineMonitor() *TelemetryPipelineMonitor {
	return newTelemetryPipelineMonitorWithClock(clock.New(), utilizationSampleInterval)
}

func newTelemetryPipelineMonitorWithClock(clk clock.Clock, sampleInterval time.Duration) *TelemetryPipelineMonitor {
	return &TelemetryPipelineMonitor{
		monitors:       make(map[string]*CapacityMonitor),
		lock:           sync.RWMutex{},
		clock:          clk,
		sampleInterval: sampleInterval,
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

// MakeUtilizationMonitor creates a new utilization monitor for a component and registers it
// for periodic sampling by this pipeline monitor's ticker. The monitor shares the pipeline
// monitor's clock so the ticker and the monitor's own accounting advance consistently.
func (c *TelemetryPipelineMonitor) MakeUtilizationMonitor(name string, instanceID string) UtilizationMonitor {
	m := newTelemetryUtilizationMonitorWithSampleRateAndClock(name, instanceID, c.sampleInterval, c.clock)
	c.lock.Lock()
	c.utilizationMonitors = append(c.utilizationMonitors, m)
	c.lock.Unlock()
	return m
}

// Start launches the periodic sampling loop. Idempotent; safe to call once per pipeline
// lifetime (wired into Sender.Start).
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

// Stop ends the periodic sampling loop and blocks until the goroutine has exited so it never
// outlives the pipeline. Idempotent.
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
			// Snapshot the slice header under the lock; MakeUtilizationMonitor only appends,
			// so iterating the captured length never races with a concurrent registration.
			monitors := c.utilizationMonitors
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
