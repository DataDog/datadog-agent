// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package metrics

import (
	"time"

	"github.com/benbjohnson/clock"
	"go.uber.org/atomic"

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
//
// Start/Stop run on the component goroutine (a hot path); sample() runs on the pipeline sampler
// goroutine. They share state only through atomics, so the hot path takes no lock: Start/Stop
// publish raw accumulators and the sampler owns all derived state.
type TelemetryUtilizationMonitor struct {
	// Accumulators written by the hot path and read by the sampler. cumulativeBusyNanos plus the
	// open interval (now - startInUseNanos, while started) is the effective busy time at any instant.
	cumulativeBusyNanos atomic.Int64
	startInUseNanos     atomic.Int64
	started             atomic.Bool

	// avg is the EWMA utilization (N=15, α≈0.125), written by the sampler and read atomically so
	// subscribers can poll it without blocking the hot path.
	avg atomic.Float64

	// Sampler-owned state.
	name              string
	instance          string
	sampleRate        time.Duration
	clock             clock.Clock
	history           *rollingHistory
	lastSample        time.Time
	lastEffectiveBusy int64
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

func newTelemetryUtilizationMonitorWithSampleRateAndClock(name, instance string, sampleRate time.Duration, clock clock.Clock, registry *snapshotRegistry) *TelemetryUtilizationMonitor {
	return &TelemetryUtilizationMonitor{
		name:       name,
		instance:   instance,
		sampleRate: sampleRate,
		clock:      clock,
		history:    newRollingHistory(),
		lastSample: clock.Now(),
		registry:   registry,
	}
}

// Start tracks a start event in the utilization tracker. startInUseNanos is stored before
// flipping started so the sampler never observes started with a stale start timestamp.
func (u *TelemetryUtilizationMonitor) Start() {
	if u.started.Load() {
		return
	}
	u.startInUseNanos.Store(u.clock.Now().UnixNano())
	u.started.Store(true)
}

// Stop tracks a finish event in the utilization tracker. The closed interval is credited to the
// cumulative counter before clearing started, so a sampler that observes started=false also
// observes the credited time.
func (u *TelemetryUtilizationMonitor) Stop() {
	if !u.started.Load() {
		return
	}
	if busy := u.clock.Now().UnixNano() - u.startInUseNanos.Load(); busy > 0 {
		u.cumulativeBusyNanos.Add(busy)
	}
	u.started.Store(false)
}

// sample is driven by the ticker so a component blocked mid-operation is observed, not frozen at
// its last EWMA.
func (u *TelemetryUtilizationMonitor) sample(now time.Time) {
	if now.Sub(u.lastSample) < u.sampleRate {
		return
	}

	// Differencing the monotonic effective-busy across the window credits the open interval once;
	// a torn read against the hot path only shifts time between adjacent windows and self-corrects.
	effBusy := u.effectiveBusyNanos(now)
	windowBusy := effBusy - u.lastEffectiveBusy
	windowElapsed := now.UnixNano() - u.lastSample.UnixNano()

	rawRatio := 0.0
	if windowElapsed > 0 {
		rawRatio = clamp01(float64(windowBusy) / float64(windowElapsed))
	}

	avg := ewma(rawRatio, u.avg.Load())
	u.avg.Store(avg)
	u.history.add(now, avg)

	TlmUtilizationRatio.Set(avg, u.name, u.instance)
	if u.registry != nil {
		u.registry.setUtilization(u.name, u.instance, avg, rawRatio, u.history)
	}

	u.lastEffectiveBusy = effBusy
	u.lastSample = now

	u.updateSaturationState(now, avg)
}

// effectiveBusyNanos returns total in-use time as of now: completed intervals plus any open one.
func (u *TelemetryUtilizationMonitor) effectiveBusyNanos(now time.Time) int64 {
	busy := u.cumulativeBusyNanos.Load()
	if u.started.Load() {
		if open := now.UnixNano() - u.startInUseNanos.Load(); open > 0 {
			busy += open
		}
	}
	return busy
}

// updateSaturationState drives the saturation state machine, emitting transition and throttled logs.
func (u *TelemetryUtilizationMonitor) updateSaturationState(now time.Time, avg float64) {
	currentlySaturated := avg >= SaturationThreshold

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
			u.episodeMaxUtil = avg
			u.episodeMaxItems = snap.RawItems
			u.episodeMaxBytes = snap.RawBytes
			// max_items/max_bytes are omitted at onset (capacity snapshot may not have ticked yet).
			log.Warnf("Logs Agent pipeline component saturated component=%s instance=%s utilization=%.0f%%",
				u.name, u.instance, avg*100)
		} else {
			if avg > u.episodeMaxUtil {
				u.episodeMaxUtil = avg
			}
			if snap.RawItems > u.episodeMaxItems {
				u.episodeMaxItems = snap.RawItems
			}
			if snap.RawBytes > u.episodeMaxBytes {
				u.episodeMaxBytes = snap.RawBytes
			}

			if now.Sub(u.lastThrottleLog) >= saturationThrottleDuration {
				log.Warnf("Logs Agent pipeline component saturated component=%s instance=%s utilization=%.0f%% duration=%s max_utilization=%.0f%% max_items=%d max_bytes=%d",
					u.name, u.instance, avg*100, now.Sub(u.saturatedSince), u.episodeMaxUtil*100, u.episodeMaxItems, u.episodeMaxBytes)
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

// clamp01 bounds a ratio to [0, 1]; timing skew can push the raw ratio slightly outside it.
func clamp01(v float64) float64 {
	if v < 0 {
		return 0
	}
	if v > 1 {
		return 1
	}
	return v
}
