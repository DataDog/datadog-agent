// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package util

import (
	"time"

	"github.com/DataDog/datadog-agent/pkg/telemetrycapture"
)

// InventoryCaptureTiming is embedded only in inventory payloads which expose
// a safe capture projection. Its fields are private and never change the wire.
type InventoryCaptureTiming struct {
	collectedAt time.Time
	cadence     time.Duration
}

// SetCaptureInventorySchedule is called only by the actual provider submission.
// Flare and cached API payloads never acquire a capture collection boundary.
func (s *InventoryCaptureTiming) SetCaptureInventorySchedule(at time.Time, cadence time.Duration) {
	s.collectedAt, s.cadence = at, cadence
}

// CaptureInventorySchedule returns the producing provider's schedule.
func (s *InventoryCaptureTiming) CaptureInventorySchedule() (time.Time, time.Duration) {
	return s.collectedAt, s.cadence
}

// ConfigureCapture attaches the daemon-owned manager before collection starts.
// Construction alone never advertises a ready inventory provider.
func (i *InventoryPayload) ConfigureCapture(manager *telemetrycapture.Manager, stream telemetrycapture.Stream) {
	i.captureManager, i.captureStream = manager, stream
}

// StopCapture prevents a completing collection from re-advertising readiness
// after shutdown. It does not wait on or change the normal collection lifecycle.
func (i *InventoryPayload) StopCapture() {
	i.captureMu.Lock()
	defer i.captureMu.Unlock()
	i.captureStopped = true
	if i.captureManager != nil {
		i.captureManager.Unregister(i.captureStream)
	}
}

func (i *InventoryPayload) captureReady(ready bool) {
	if i.captureManager == nil {
		return
	}
	i.captureMu.Lock()
	defer i.captureMu.Unlock()
	if i.captureStopped {
		return
	}
	if ready {
		_ = i.captureManager.Register(telemetrycapture.Capability{Stream: i.captureStream, Cadence: i.captureCadence()})
	} else {
		i.captureManager.Unregister(i.captureStream)
	}
}

func (i *InventoryPayload) captureCadence() time.Duration {
	// Providers poll at MinInterval but ordinarily submit only after MaxInterval.
	// Round to the next poll when configured intervals do not divide evenly.
	if i.MinInterval <= 0 {
		return i.MaxInterval
	}
	intervals := i.MaxInterval / i.MinInterval
	if i.MaxInterval%i.MinInterval != 0 {
		intervals++
	}
	if intervals < 1 {
		intervals = 1
	}
	// Invalid extreme configurations must not overflow to an advertised cadence.
	if intervals > time.Duration(1<<63-1)/i.MinInterval {
		return i.MaxInterval
	}
	return intervals * i.MinInterval
}
