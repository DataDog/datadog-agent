// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package metrics

import (
	"time"

	"github.com/DataDog/datadog-agent/pkg/util/utilizationtracker"
)

// UtilizationMonitor is an interface for monitoring the utilization of a component.
type UtilizationMonitor interface {
	Start()
	Stop()
	Cancel()
}

// NoopUtilizationMonitor is a no-op implementation of UtilizationMonitor.
type NoopUtilizationMonitor struct{}

// Start does nothing.
func (n *NoopUtilizationMonitor) Start() {}

// Stop does nothing.
func (n *NoopUtilizationMonitor) Stop() {}

// Cancel does nothing.
func (n *NoopUtilizationMonitor) Cancel() {}

// TelemetryUtilizationMonitor is a UtilizationMonitor that reports utilization metrics as telemetry.
type TelemetryUtilizationMonitor struct {
	name     string
	instance string
	started  bool
	ut       *utilizationtracker.UtilizationTracker
	cancel   func()
}

// NewTelemetryUtilizationMonitor creates a new TelemetryUtilizationMonitor.
func NewTelemetryUtilizationMonitor(name, instance string) *TelemetryUtilizationMonitor {

	utilizationTracker := utilizationtracker.NewUtilizationTracker(1*time.Second, ewmaAlpha)
	cancel := startTrackerTicker(utilizationTracker, 1*time.Second)

	t := &TelemetryUtilizationMonitor{
		name:     name,
		instance: instance,
		started:  false,
		ut:       utilizationTracker,
		cancel:   cancel,
	}
	t.startUtilizationUpdater()
	return t
}

// Start tracks a start event in the utilization tracker.
func (u *TelemetryUtilizationMonitor) Start() {
	if u.started {
		return
	}
	u.started = true
	u.ut.Started()
}

// Stop tracks a finish event in the utilization tracker.
func (u *TelemetryUtilizationMonitor) Stop() {
	if !u.started {
		return
	}
	u.started = false
	u.ut.Finished()
}

// Cancel stops the monitor.
func (u *TelemetryUtilizationMonitor) Cancel() {
	u.cancel()
	u.ut.Stop()
}

func startTrackerTicker(ut *utilizationtracker.UtilizationTracker, interval time.Duration) func() {
	ticker := time.NewTicker(interval)
	cancel := make(chan struct{}, 1)
	done := make(chan struct{})
	go func() {
		defer ticker.Stop()
		defer close(done)
		for {
			select {
			case <-ticker.C:
				ut.Tick()
			case <-cancel:
				return
			}
		}
	}()

	return func() {
		cancel <- struct{}{}
		<-done // make sure Tick will not be called after we return.
	}
}

func (u *TelemetryUtilizationMonitor) startUtilizationUpdater() {
	TlmUtilizationRatio.Set(0, u.name, u.instance)
	go func() {
		for value := range u.ut.Output {
			TlmUtilizationRatio.Set(value, u.name, u.instance)
		}
	}()
}
