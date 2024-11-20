// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package metrics

import (
	"time"

	"github.com/benbjohnson/clock"
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
	inUse      time.Duration
	idle       time.Duration
	startIdle  time.Time
	startInUse time.Time
	avg        float64
	name       string
	instance   string
	started    bool
	tickChan   <-chan time.Time
	clock      clock.Clock
}

// NewTelemetryUtilizationMonitor creates a new TelemetryUtilizationMonitor.
func NewTelemetryUtilizationMonitor(name, instance string) *TelemetryUtilizationMonitor {
	return newTelemetryUtilizationMonitorWithTickAndClock(name, instance, time.NewTicker(1*time.Second).C, clock.New())
}

func newTelemetryUtilizationMonitorWithTickAndClock(name, instance string, tickChan <-chan time.Time, clock clock.Clock) *TelemetryUtilizationMonitor {
	return &TelemetryUtilizationMonitor{
		name:       name,
		instance:   instance,
		startIdle:  clock.Now(),
		startInUse: clock.Now(),
		avg:        0,
		started:    false,
		tickChan:   tickChan,
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
	select {
	case <-u.tickChan:
		u.avg = ewma(float64(u.inUse)/float64(u.idle+u.inUse), u.avg)
		TlmUtilizationRatio.Set(u.avg, u.name, u.instance)
		u.idle = 0
		u.inUse = 0
	default:
	}
}
