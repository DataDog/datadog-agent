// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package metrics

import (
	"sync"
	"time"

	"github.com/benbjohnson/clock"

	log "github.com/DataDog/datadog-agent/pkg/util/log"
)

const (
	saturationThrottleDuration = 10 * time.Minute
	// recoveryDebounce is how long the EWMA must stay below threshold before recovery is logged.
	recoveryDebounce = 10 * time.Second
)

// UtilizationMonitor is an interface for monitoring the utilization of a component.
type UtilizationMonitor interface {
	Start()
	Stop()
}

// NoopUtilizationMonitor is a no-op implementation of UtilizationMonitor.
type NoopUtilizationMonitor struct{}

// Start does nothing.
func (n *NoopUtilizationMonitor) Start() {}

// Stop does nothing.
func (n *NoopUtilizationMonitor) Stop() {}

// TelemetryUtilizationMonitor is a UtilizationMonitor that reports utilization metrics as telemetry.
type TelemetryUtilizationMonitor struct {
	// mu guards all mutable state: Start/Stop run on the component goroutine, sample on the ticker.
	mu sync.Mutex

	inUse      time.Duration
	idle       time.Duration
	startIdle  time.Time
	startInUse time.Time
	lastSample time.Time
	sampleRate time.Duration
	avg        float64 // EWMA utilization (N=15, α≈0.125)
	history    *rollingHistory
	name       string
	instance   string
	started    bool
	clock      clock.Clock
	// registry, when non-nil, receives utilization snapshots and supplies the capacity figures
	// used in saturation logs. It is owned by the pipeline monitor; standalone monitors leave it nil.
	registry *snapshotRegistry

	// Saturation episode tracking for log emission.
	isSaturated          bool
	saturatedSince       time.Time
	lastThrottleLog      time.Time
	episodeMaxUtil       float64
	episodeMaxItems      int64
	episodeMaxBytes      int64
	pendingRecoverySince time.Time // non-zero while EWMA is below threshold but debounce not yet met
}

// NewTelemetryUtilizationMonitor creates a new TelemetryUtilizationMonitor that reports to telemetry only.
func NewTelemetryUtilizationMonitor(name, instance string) *TelemetryUtilizationMonitor {
	return newTelemetryUtilizationMonitorWithSampleRateAndClock(name, instance, 1*time.Second, clock.New(), nil)
}

func newTelemetryUtilizationMonitorWithSampleRateAndClock(name, instance string, sampleRate time.Duration, clock clock.Clock, registry *snapshotRegistry) *TelemetryUtilizationMonitor {
	return &TelemetryUtilizationMonitor{
		name:       name,
		instance:   instance,
		startIdle:  clock.Now(),
		startInUse: clock.Now(),
		lastSample: clock.Now(),
		sampleRate: sampleRate,
		avg:        0,
		history:    newRollingHistory(),
		started:    false,
		clock:      clock,
		registry:   registry,
	}
}

// Start tracks a start event in the utilization tracker.
func (u *TelemetryUtilizationMonitor) Start() {
	u.mu.Lock()
	defer u.mu.Unlock()
	if u.started {
		return
	}
	u.started = true
	now := u.clock.Now()
	u.idle += now.Sub(u.startIdle)
	u.startInUse = now
	u.reportIfNeededLocked(now)
}

// Stop tracks a finish event in the utilization tracker.
func (u *TelemetryUtilizationMonitor) Stop() {
	u.mu.Lock()
	defer u.mu.Unlock()
	if !u.started {
		return
	}
	u.started = false
	now := u.clock.Now()
	u.inUse += now.Sub(u.startInUse)
	u.startIdle = now
	u.reportIfNeededLocked(now)
}

// sample is driven by the ticker so a component blocked mid-operation is observed, not frozen at its last EWMA.
func (u *TelemetryUtilizationMonitor) sample(now time.Time) {
	u.mu.Lock()
	defer u.mu.Unlock()
	u.settleLocked(now)
	u.reportIfNeededLocked(now)
}

// settleLocked credits the open interval up to now and advances its start so Start/Stop won't double-count it. Caller holds u.mu.
func (u *TelemetryUtilizationMonitor) settleLocked(now time.Time) {
	if u.started {
		u.inUse += now.Sub(u.startInUse)
		u.startInUse = now
	} else {
		u.idle += now.Sub(u.startIdle)
		u.startIdle = now
	}
}

// reportIfNeededLocked publishes a sample if sampleRate has elapsed; now is passed for a consistent instant. Caller holds u.mu.
func (u *TelemetryUtilizationMonitor) reportIfNeededLocked(now time.Time) {
	if now.Sub(u.lastSample) < u.sampleRate {
		return
	}
	rawRatio := 0.0
	if total := u.idle + u.inUse; total > 0 {
		rawRatio = float64(u.inUse) / float64(total)
	}
	u.avg = ewma(rawRatio, u.avg)

	u.history.add(now, u.avg)

	TlmUtilizationRatio.Set(u.avg, u.name, u.instance)
	if u.registry != nil {
		u.registry.setUtilization(u.name, u.instance, u.avg, rawRatio, u.history)
	}
	u.idle = 0
	u.inUse = 0
	u.lastSample = now

	u.updateSaturationState(now)
}

// updateSaturationState drives the saturation state machine, emitting transition and throttled logs. Caller holds u.mu.
func (u *TelemetryUtilizationMonitor) updateSaturationState(now time.Time) {
	currentlySaturated := u.avg >= SaturationThreshold

	if currentlySaturated {
		u.pendingRecoverySince = time.Time{}

		var snap ComponentSnapshot
		if u.registry != nil {
			snap, _ = u.registry.lookup(u.name, u.instance)
		}

		if !u.isSaturated {
			u.isSaturated = true
			u.saturatedSince = now
			u.lastThrottleLog = now
			u.episodeMaxUtil = u.avg
			u.episodeMaxItems = snap.RawItems
			u.episodeMaxBytes = snap.RawBytes
			// max_items/max_bytes are omitted at onset (capacity snapshot may not have ticked yet).
			log.Warnf("Logs Agent pipeline component saturated component=%s instance=%s utilization=%.0f%%",
				u.name, u.instance, u.avg*100)
		} else {
			if u.avg > u.episodeMaxUtil {
				u.episodeMaxUtil = u.avg
			}
			if snap.RawItems > u.episodeMaxItems {
				u.episodeMaxItems = snap.RawItems
			}
			if snap.RawBytes > u.episodeMaxBytes {
				u.episodeMaxBytes = snap.RawBytes
			}

			if now.Sub(u.lastThrottleLog) >= saturationThrottleDuration {
				log.Warnf("Logs Agent pipeline component saturated component=%s instance=%s utilization=%.0f%% duration=%s max_utilization=%.0f%% max_items=%d max_bytes=%d",
					u.name, u.instance, u.avg*100, now.Sub(u.saturatedSince), u.episodeMaxUtil*100, u.episodeMaxItems, u.episodeMaxBytes)
				u.lastThrottleLog = now
			}
		}
	} else if u.isSaturated {
		// Below threshold: start or advance the recovery debounce timer.
		if u.pendingRecoverySince.IsZero() {
			u.pendingRecoverySince = now
		} else if now.Sub(u.pendingRecoverySince) >= recoveryDebounce {
			log.Infof("Logs Agent pipeline component recovered component=%s instance=%s saturated_duration=%s max_utilization=%.0f%% max_items=%d max_bytes=%d",
				u.name, u.instance, now.Sub(u.saturatedSince), u.episodeMaxUtil*100, u.episodeMaxItems, u.episodeMaxBytes)
			u.isSaturated = false
			u.pendingRecoverySince = time.Time{}
			u.episodeMaxUtil = 0
			u.episodeMaxItems = 0
			u.episodeMaxBytes = 0
		}
	}
}
