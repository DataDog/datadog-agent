// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package runner

import (
	"sync"

	"github.com/DataDog/datadog-agent/pkg/process/checks"
	"github.com/DataDog/datadog-agent/pkg/telemetrycapture"
)

// Shared by the production submitter and its host-local encoder views. This
// state describes running producers; constructing a component is insufficient.
type captureLifecycle struct {
	mu         sync.Mutex
	running    bool
	checks     map[string]captureCheckState
	registered map[telemetrycapture.Stream]bool
}

type captureCheckState struct {
	running bool
	healthy bool
}

// SetCaptureCheckRunning follows the existing scheduler's startup and shutdown.
func (s *CheckSubmitter) SetCaptureCheckRunning(name string, running bool) {
	s.updateCaptureCheck(name, func(state *captureCheckState) {
		state.running = running
		if !running {
			state.healthy = false
		}
	})
}

// SetCaptureCheckHealthy consumes an existing standard collection result. A
// configured connection check is not ready until its system-probe call succeeds.
func (s *CheckSubmitter) SetCaptureCheckHealthy(name string, healthy bool) {
	s.updateCaptureCheck(name, func(state *captureCheckState) { state.healthy = healthy })
}

func (s *CheckSubmitter) updateCaptureCheck(name string, update func(*captureCheckState)) {
	if s.captureLifecycle == nil || s.CaptureManager == nil || (name != checks.ProcessCheckName && name != checks.ConnectionsCheckName) {
		return
	}
	lifecycle := s.captureLifecycle
	lifecycle.mu.Lock()
	defer lifecycle.mu.Unlock()
	state := lifecycle.checks[name]
	update(&state)
	lifecycle.checks[name] = state
	s.syncCaptureCapabilityLocked(name, state)
}

func (s *CheckSubmitter) setCaptureSubmitterRunning(running bool) {
	if s.captureLifecycle == nil || s.CaptureManager == nil {
		return
	}
	lifecycle := s.captureLifecycle
	lifecycle.mu.Lock()
	defer lifecycle.mu.Unlock()
	lifecycle.running = running
	for name, state := range lifecycle.checks {
		s.syncCaptureCapabilityLocked(name, state)
	}
}

func (s *CheckSubmitter) syncCaptureCapabilityLocked(name string, state captureCheckState) {
	stream := telemetrycapture.Processes
	owner := ""
	if name == checks.ConnectionsCheckName {
		stream, owner = telemetrycapture.Connections, "process"
	}
	lifecycle := s.captureLifecycle
	cadence := s.captureCadence(name)
	if lifecycle.running && state.running && state.healthy && cadence > 0 && !s.shouldDropPayload(name) {
		if s.CaptureManager.Register(telemetrycapture.Capability{Stream: stream, Cadence: cadence, ConnectionOwner: owner}) == nil {
			lifecycle.registered[stream] = true
		}
	} else if lifecycle.registered[stream] {
		s.CaptureManager.Unregister(stream)
		delete(lifecycle.registered, stream)
	}
}
