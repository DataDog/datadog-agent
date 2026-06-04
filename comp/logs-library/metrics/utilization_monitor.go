// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package metrics

import (
	"time"

	"github.com/benbjohnson/clock"

	log "github.com/DataDog/datadog-agent/pkg/util/log"
)

const saturationThrottleDuration = 10 * time.Minute

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
	inUse      time.Duration
	idle       time.Duration
	startIdle  time.Time
	startInUse time.Time
	lastSample time.Time
	sampleRate time.Duration
	avg        float64 // N=30 EWMA
	shortAvg   float64 // N=15 EWMA
	history    *rollingHistory
	name       string
	instance   string
	started    bool
	clock      clock.Clock

	// Saturation episode tracking for log emission.
	isSaturated     bool
	saturatedSince  time.Time
	lastThrottleLog time.Time
	episodeMaxUtil  float64
	episodeMaxItems int64
	episodeMaxBytes int64
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
		shortAvg:   0,
		history:    newRollingHistory(),
		started:    false,
		clock:      clock,
	}
}

// Start tracks a start event in the utilization tracker.
func (u *TelemetryUtilizationMonitor) Start() {
	if u.started {
		return
	}
	u.started = true
	u.idle += u.clock.Since(u.startIdle)
	u.startInUse = u.clock.Now()
	u.reportIfNeeded()
}

// Stop tracks a finish event in the utilization tracker.
func (u *TelemetryUtilizationMonitor) Stop() {
	if !u.started {
		return
	}
	u.started = false
	u.inUse += u.clock.Since(u.startInUse)
	u.startIdle = u.clock.Now()
	u.reportIfNeeded()
}

func (u *TelemetryUtilizationMonitor) reportIfNeeded() {
	if u.clock.Since(u.lastSample) >= u.sampleRate {
		rawRatio := 0.0
		if total := u.idle + u.inUse; total > 0 {
			rawRatio = float64(u.inUse) / float64(total)
		}
		u.avg = ewma(rawRatio, u.avg)
		u.shortAvg = shortEwma(rawRatio, u.shortAvg)

		now := u.clock.Now()
		u.history.add(now, u.shortAvg)
		ws := u.history.allStats(now)

		TlmUtilizationRatio.Set(u.avg, u.name, u.instance)
		TlmUtilizationShortRatio.Set(u.shortAvg, u.name, u.instance)
		setComponentUtilization(u.name, u.instance, u.avg, rawRatio, u.shortAvg, ws)
		u.idle = 0
		u.inUse = 0
		u.lastSample = now

		u.updateSaturationState(now)
	}
}

// updateSaturationState drives the per-component saturation state machine and emits
// log events on transitions and on a throttled cadence during sustained saturation.
// Must be called with the sample lock held (i.e. from inside reportIfNeeded).
func (u *TelemetryUtilizationMonitor) updateSaturationState(now time.Time) {
	currentlySaturated := u.shortAvg >= SaturationThreshold

	if currentlySaturated {
		snap, _ := lookupComponentSnapshot(u.name, u.instance)

		if !u.isSaturated {
			// Entering saturation: log once and initialise episode tracking.
			u.isSaturated = true
			u.saturatedSince = now
			u.lastThrottleLog = now
			u.episodeMaxUtil = u.shortAvg
			u.episodeMaxItems = snap.RawItems
			u.episodeMaxBytes = snap.RawBytes
			log.Warnf("Logs Agent pipeline component saturated component=%s instance=%s utilization=%.0f%% duration=0s max_utilization=%.0f%% max_items=%d max_bytes=%d",
				u.name, u.instance, u.shortAvg*100, u.episodeMaxUtil*100, u.episodeMaxItems, u.episodeMaxBytes)
		} else {
			// Ongoing saturation: keep episode maxes up to date.
			if u.shortAvg > u.episodeMaxUtil {
				u.episodeMaxUtil = u.shortAvg
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
					u.name, u.instance, u.shortAvg*100, now.Sub(u.saturatedSince), u.episodeMaxUtil*100, u.episodeMaxItems, u.episodeMaxBytes)
				u.lastThrottleLog = now
			}
		}
	} else if u.isSaturated {
		// Recovery: log once with episode summary.
		log.Infof("Logs Agent pipeline component recovered component=%s instance=%s saturated_duration=%s max_utilization=%.0f%% max_items=%d max_bytes=%d",
			u.name, u.instance, now.Sub(u.saturatedSince), u.episodeMaxUtil*100, u.episodeMaxItems, u.episodeMaxBytes)
		u.isSaturated = false
		u.episodeMaxUtil = 0
		u.episodeMaxItems = 0
		u.episodeMaxBytes = 0
	}
}

func shortEwma(newValue, oldValue float64) float64 {
	return newValue*shortEwmaAlpha + oldValue*(1-shortEwmaAlpha)
}
