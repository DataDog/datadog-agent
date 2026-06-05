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
	// recoveryDebounce is how long the EWMA must stay below SaturationThreshold before
	// recovery is logged. Prevents false recoveries when the EWMA briefly dips below
	// the threshold between processing bursts.
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
	// mu guards all mutable state. Start/Stop are called on the component's own goroutine,
	// while sample is called from the pipeline monitor's periodic ticker goroutine.
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

	// Saturation episode tracking for log emission.
	isSaturated          bool
	saturatedSince       time.Time
	lastThrottleLog      time.Time
	episodeMaxUtil       float64
	episodeMaxItems      int64
	episodeMaxBytes      int64
	pendingRecoverySince time.Time // non-zero while EWMA is below threshold but debounce not yet met
}

// NewTelemetryUtilizationMonitor creates a new TelemetryUtilizationMonitor.
func NewTelemetryUtilizationMonitor(name, instance string) *TelemetryUtilizationMonitor {
	return newTelemetryUtilizationMonitorWithSampleRateAndClock(name, instance, 1*time.Second, clock.New())
}

func newTelemetryUtilizationMonitorWithSampleRateAndClock(name, instance string, sampleRate time.Duration, clock clock.Clock) *TelemetryUtilizationMonitor {
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

// sample is called periodically by the pipeline monitor's ticker so a component that is
// blocked mid-operation (e.g. on a full output channel) is still observed. Utilization is
// otherwise sampled only on Start/Stop, so a long in-flight block produces no sample until it
// completes — freezing the EWMA and suppressing saturation logs during the exact backpressure
// event we want to surface. settleLocked credits the in-progress interval up to now before
// reporting.
func (u *TelemetryUtilizationMonitor) sample(now time.Time) {
	u.mu.Lock()
	defer u.mu.Unlock()
	u.settleLocked(now)
	u.reportIfNeededLocked(now)
}

// settleLocked credits the time elapsed in the currently-open interval (in-use if started,
// idle otherwise) up to now, then advances the interval start so the same span is not
// double-counted by the eventual Start/Stop. Caller must hold u.mu.
func (u *TelemetryUtilizationMonitor) settleLocked(now time.Time) {
	if u.started {
		u.inUse += now.Sub(u.startInUse)
		u.startInUse = now
	} else {
		u.idle += now.Sub(u.startIdle)
		u.startIdle = now
	}
}

// reportIfNeededLocked computes and publishes a utilization sample if at least sampleRate has
// elapsed since the last one. Caller must hold u.mu. now is passed in (rather than read from
// the clock) so Start/Stop/sample all report against a single consistent instant.
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
	// Pass the live history rather than a precomputed WindowStats: window stats are
	// recomputed at read-time so an idle component's stats decay against the live clock.
	setComponentUtilization(u.name, u.instance, u.avg, rawRatio, u.history)
	u.idle = 0
	u.inUse = 0
	u.lastSample = now

	u.updateSaturationState(now)
}

// updateSaturationState drives the per-component saturation state machine and emits
// log events on transitions and on a throttled cadence during sustained saturation.
// Called only from reportIfNeededLocked with u.mu held, so its access to the episode
// tracking fields is serialized against Start/Stop and the periodic sample tick.
func (u *TelemetryUtilizationMonitor) updateSaturationState(now time.Time) {
	currentlySaturated := u.avg >= SaturationThreshold

	if currentlySaturated {
		// Any re-entry above threshold cancels a pending recovery.
		u.pendingRecoverySince = time.Time{}

		snap, _ := lookupComponentSnapshot(u.name, u.instance)

		if !u.isSaturated {
			// Entering saturation: log once and initialise episode tracking.
			u.isSaturated = true
			u.saturatedSince = now
			u.lastThrottleLog = now
			u.episodeMaxUtil = u.avg
			u.episodeMaxItems = snap.RawItems
			u.episodeMaxBytes = snap.RawBytes
			// Omit max_items/max_bytes at onset — the capacity snapshot may not have
			// ticked yet so values can be 0. They are tracked through the episode and
			// reported accurately at recovery and in throttled summaries.
			log.Warnf("Logs Agent pipeline component saturated component=%s instance=%s utilization=%.0f%%",
				u.name, u.instance, u.avg*100)
		} else {
			// Ongoing saturation: keep episode maxes up to date.
			if u.avg > u.episodeMaxUtil {
				u.episodeMaxUtil = u.avg
			}
			if snap.RawItems > u.episodeMaxItems {
				u.episodeMaxItems = snap.RawItems
			}
			if snap.RawBytes > u.episodeMaxBytes {
				u.episodeMaxBytes = snap.RawBytes
			}

			// Throttled summary every saturationThrottleDuration.
			if now.Sub(u.lastThrottleLog) >= saturationThrottleDuration {
				log.Warnf("Logs Agent pipeline component saturated component=%s instance=%s utilization=%.0f%% duration=%s max_utilization=%.0f%% max_items=%d max_bytes=%d",
					u.name, u.instance, u.avg*100, now.Sub(u.saturatedSince), u.episodeMaxUtil*100, u.episodeMaxItems, u.episodeMaxBytes)
				u.lastThrottleLog = now
			}
		}
	} else if u.isSaturated {
		// EWMA is below threshold. Start or advance the recovery debounce timer.
		if u.pendingRecoverySince.IsZero() {
			u.pendingRecoverySince = now
		} else if now.Sub(u.pendingRecoverySince) >= recoveryDebounce {
			// Confirmed recovery: EWMA has stayed below threshold for recoveryDebounce.
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
