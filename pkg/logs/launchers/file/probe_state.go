// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package file

import (
	"time"

	"github.com/DataDog/datadog-agent/pkg/util/log"
)

const (
	estaleWarnInterval = 60 * time.Second
	estaleStalledAfter = 120 * time.Second
)

type probeStatus string

// probeStatus values are observability labels for sequential handoff degradation.
// Control flow uses booleans (BufferedProbeRejected, verify gates, replacementRequested).
const (
	probeStatusOK                    probeStatus = "ok"
	probeStatusEstaleDegraded        probeStatus = "estale_degraded"         // pathname probe returned ESTALE
	probeStatusStalled               probeStatus = "stalled"                 // ESTALE persisted beyond estaleStalledAfter
	probeStatusBufferedProbeRejected probeStatus = "buffered_probe_rejected" // direct configured but pathname probe used buffered I/O
	probeStatusVerifying             probeStatus = "verifying_descriptor"    // pass-2 fd fingerprint in progress
	probeStatusDescriptorMismatch    probeStatus = "descriptor_mismatch"     // buffered fd fingerprint != stored candidate
)

type activeProbeState struct {
	consecutiveEstale int
	firstEstaleAt     time.Time
	lastEstaleAt      time.Time
	lastWarnAt        time.Time
	probeStatus       probeStatus
}

func (s *Launcher) noteActiveProbeEstale(scanKey string) {
	now := time.Now()
	s.activeProbeMu.Lock()
	defer s.activeProbeMu.Unlock()
	if s.activeProbeState == nil {
		s.activeProbeState = make(map[string]*activeProbeState)
	}
	state := s.activeProbeState[scanKey]
	if state == nil {
		state = &activeProbeState{firstEstaleAt: now}
		s.activeProbeState[scanKey] = state
	}
	state.consecutiveEstale++
	state.lastEstaleAt = now
	state.probeStatus = probeStatusEstaleDegraded
	if !state.firstEstaleAt.IsZero() && now.Sub(state.firstEstaleAt) >= estaleStalledAfter {
		state.probeStatus = probeStatusStalled
	}
	s.maybeWarnProbeState(scanKey, state, now)
}

func (s *Launcher) noteActiveProbeBufferedProbeRejected(scanKey string) {
	s.activeProbeMu.Lock()
	defer s.activeProbeMu.Unlock()
	if s.activeProbeState == nil {
		s.activeProbeState = make(map[string]*activeProbeState)
	}
	state := s.activeProbeState[scanKey]
	if state == nil {
		state = &activeProbeState{}
		s.activeProbeState[scanKey] = state
	}
	state.probeStatus = probeStatusBufferedProbeRejected
	now := time.Now()
	s.maybeWarnProbeState(scanKey, state, now)
}

func (s *Launcher) clearActiveProbeState(scanKey string) {
	s.activeProbeMu.Lock()
	defer s.activeProbeMu.Unlock()
	delete(s.activeProbeState, scanKey)
}

func (s *Launcher) transferActiveProbeState(scanKey string, intent *replacementIntent) {
	if intent == nil {
		return
	}
	s.activeProbeMu.Lock()
	defer s.activeProbeMu.Unlock()
	state := s.activeProbeState[scanKey]
	if state == nil {
		return
	}
	intent.consecutiveEstale = state.consecutiveEstale
	intent.firstEstaleAt = state.firstEstaleAt
	intent.lastEstaleAt = state.lastEstaleAt
	intent.lastWarnAt = state.lastWarnAt
	if state.probeStatus != "" {
		intent.probeStatus = state.probeStatus
	}
	delete(s.activeProbeState, scanKey)
}

func (s *Launcher) noteIntentEstale(scanKey, path string) {
	intent := s.getReplacementIntent(scanKey, path)
	if intent == nil {
		return
	}
	now := time.Now()
	s.pathHandoffsMu.Lock()
	defer s.pathHandoffsMu.Unlock()
	handoff := s.pathHandoffs[normalizeHandoffPath(path)]
	if handoff == nil {
		return
	}
	intent = handoff.replacements[scanKey]
	if intent == nil {
		return
	}
	if intent.consecutiveEstale == 0 {
		intent.firstEstaleAt = now
	}
	intent.consecutiveEstale++
	intent.lastEstaleAt = now
	intent.probeStatus = probeStatusEstaleDegraded
	if !intent.firstEstaleAt.IsZero() && now.Sub(intent.firstEstaleAt) >= estaleStalledAfter {
		intent.probeStatus = probeStatusStalled
	}
	s.maybeWarnIntent(scanKey, intent, now)
	handoff.replacements[scanKey] = intent
}

func (s *Launcher) setIntentProbeStatus(scanKey, path string, status probeStatus) {
	s.pathHandoffsMu.Lock()
	defer s.pathHandoffsMu.Unlock()
	handoff := s.pathHandoffs[normalizeHandoffPath(path)]
	if handoff == nil {
		return
	}
	intent := handoff.replacements[scanKey]
	if intent == nil {
		return
	}
	intent.probeStatus = status
	handoff.replacements[scanKey] = intent
}

func (s *Launcher) maybeWarnProbeState(scanKey string, state *activeProbeState, now time.Time) {
	if state == nil {
		return
	}
	if !state.lastWarnAt.IsZero() && now.Sub(state.lastWarnAt) < estaleWarnInterval {
		return
	}
	state.lastWarnAt = now
	log.Warnf("log file probe degraded for scan key %q: status=%s consecutive_estale=%d", scanKey, state.probeStatus, state.consecutiveEstale)
}

func (s *Launcher) maybeWarnIntent(scanKey string, intent *replacementIntent, now time.Time) {
	if intent == nil {
		return
	}
	if !intent.lastWarnAt.IsZero() && now.Sub(intent.lastWarnAt) < estaleWarnInterval {
		return
	}
	intent.lastWarnAt = now
	log.Warnf("sequential replacement probe degraded for scan key %q: status=%s consecutive_estale=%d", scanKey, intent.probeStatus, intent.consecutiveEstale)
}

// activeProbeStateForTest exposes active probe state for unit tests.
func (s *Launcher) activeProbeStateForTest(scanKey string) (probeStatus, int) {
	s.activeProbeMu.Lock()
	defer s.activeProbeMu.Unlock()
	state := s.activeProbeState[scanKey]
	if state == nil {
		return "", 0
	}
	return state.probeStatus, state.consecutiveEstale
}

// replacementProbeStatusForTest exposes intent probe status for unit tests.
func (s *Launcher) replacementProbeStatusForTest(scanKey, path string) probeStatus {
	s.pathHandoffsMu.Lock()
	defer s.pathHandoffsMu.Unlock()
	handoff := s.pathHandoffs[normalizeHandoffPath(path)]
	if handoff == nil {
		return ""
	}
	intent := handoff.replacements[scanKey]
	if intent == nil {
		return ""
	}
	return intent.probeStatus
}
