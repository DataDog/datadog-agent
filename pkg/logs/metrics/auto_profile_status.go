// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package metrics

import (
	"fmt"
	"sort"
	"sync"
	"time"
)

// AutoProfileSnapshot is a point-in-time status snapshot for the logs auto profile watchdog.
type AutoProfileSnapshot struct {
	Enabled             bool
	LastDecisionAction  string
	LastDecisionReason  string
	LastDecisionAt      time.Time
	LastApplyStatus     string
	LastApplyReason     string
	LastApplyAt         time.Time
	LastAppliedSettings map[string]string
	CooldownRemaining   time.Duration
	AppliesLastHour     int
}

// autoProfileStatus tracks status fields exposed in the logs status page.
type autoProfileStatus struct {
	mu sync.Mutex

	enabled bool

	lastDecisionAction string
	lastDecisionReason string
	lastDecisionAt     time.Time

	lastApplyStatus     string
	lastApplyReason     string
	lastApplyAt         time.Time
	lastAppliedSettings map[string]string

	cooldownUntil   time.Time
	applyTimestamps []time.Time
}

// GlobalAutoProfileStatus is the package-level watchdog status state.
var GlobalAutoProfileStatus = newAutoProfileStatus()

func newAutoProfileStatus() *autoProfileStatus {
	return &autoProfileStatus{
		lastAppliedSettings: make(map[string]string),
		applyTimestamps:     make([]time.Time, 0, 8),
	}
}

// SetEnabled records whether watchdog mode is currently active.
func (s *autoProfileStatus) SetEnabled(enabled bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.enabled = enabled
}

// SetCooldownUntil records the end of the current cooldown window.
func (s *autoProfileStatus) SetCooldownUntil(until time.Time) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.cooldownUntil = until
}

// RecordDecision stores metadata for the most recent watchdog decision.
func (s *autoProfileStatus) RecordDecision(action, reason string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.lastDecisionAction = action
	s.lastDecisionReason = reason
	s.lastDecisionAt = time.Now()
}

// RecordApply stores metadata for the most recent watchdog apply attempt.
func (s *autoProfileStatus) RecordApply(status, reason string, settings map[string]interface{}) {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.lastApplyStatus = status
	s.lastApplyReason = reason
	s.lastApplyAt = time.Now()
	s.lastAppliedSettings = stringifySettings(settings)

	s.applyTimestamps = append(s.applyTimestamps, s.lastApplyAt)
	s.pruneApplyHistoryLocked(s.lastApplyAt)
}

func (s *autoProfileStatus) pruneApplyHistoryLocked(now time.Time) {
	cutoff := now.Add(-1 * time.Hour)
	n := 0
	for _, ts := range s.applyTimestamps {
		if ts.After(cutoff) {
			s.applyTimestamps[n] = ts
			n++
		}
	}
	s.applyTimestamps = s.applyTimestamps[:n]
}

func stringifySettings(settings map[string]interface{}) map[string]string {
	out := make(map[string]string, len(settings))
	for k, v := range settings {
		out[k] = fmt.Sprint(v)
	}
	return out
}

// Snapshot returns the current status snapshot.
func (s *autoProfileStatus) Snapshot(now time.Time) AutoProfileSnapshot {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.pruneApplyHistoryLocked(now)

	remaining := time.Duration(0)
	if now.Before(s.cooldownUntil) {
		remaining = s.cooldownUntil.Sub(now)
	}

	return AutoProfileSnapshot{
		Enabled:             s.enabled,
		LastDecisionAction:  s.lastDecisionAction,
		LastDecisionReason:  s.lastDecisionReason,
		LastDecisionAt:      s.lastDecisionAt,
		LastApplyStatus:     s.lastApplyStatus,
		LastApplyReason:     s.lastApplyReason,
		LastApplyAt:         s.lastApplyAt,
		LastAppliedSettings: copyStringMap(s.lastAppliedSettings),
		CooldownRemaining:   remaining,
		AppliesLastHour:     len(s.applyTimestamps),
	}
}

func copyStringMap(in map[string]string) map[string]string {
	out := make(map[string]string, len(in))
	for k, v := range in {
		out[k] = v
	}
	return out
}

// AutoProfileInfoProvider formats watchdog status for logs status output.
type AutoProfileInfoProvider struct{}

// Info returns pre-formatted lines for status rendering.
func (AutoProfileInfoProvider) Info() []string {
	s := GlobalAutoProfileStatus.Snapshot(time.Now())
	lines := []string{
		fmt.Sprintf("Enabled: %t", s.Enabled),
	}

	if !s.LastDecisionAt.IsZero() {
		lines = append(lines,
			fmt.Sprintf("Last Decision: %s (reason: %s) at %s",
				s.LastDecisionAction, s.LastDecisionReason, s.LastDecisionAt.Local().Format(time.RFC3339)))
	}

	if !s.LastApplyAt.IsZero() {
		lines = append(lines,
			fmt.Sprintf("Last Apply: %s (reason: %s) at %s",
				s.LastApplyStatus, s.LastApplyReason, s.LastApplyAt.Local().Format(time.RFC3339)))
		if len(s.LastAppliedSettings) > 0 {
			lines = append(lines, "Last Applied Settings:")
			keys := make([]string, 0, len(s.LastAppliedSettings))
			for k := range s.LastAppliedSettings {
				keys = append(keys, k)
			}
			sort.Strings(keys)
			for _, k := range keys {
				lines = append(lines, fmt.Sprintf("  %s=%s", k, s.LastAppliedSettings[k]))
			}
		}
	}

	lines = append(lines, fmt.Sprintf("Cooldown Remaining: %s", s.CooldownRemaining.Round(time.Second)))
	lines = append(lines, fmt.Sprintf("Applies Last Hour: %d", s.AppliesLastHour))
	return lines
}
