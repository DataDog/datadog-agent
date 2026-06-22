// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

//go:build test

// Package mock provides a no-op mock for the health platform scheduler.
package mock

import (
	"errors"
	"fmt"
	"sync"
	"time"

	runnerdef "github.com/DataDog/datadog-agent/comp/healthplatform/runner/def"
	schedulerdef "github.com/DataDog/datadog-agent/comp/healthplatform/scheduler/def"
)

// ScheduleCall records the arguments of a single Schedule() call.
type ScheduleCall struct {
	Source   string
	Fn       runnerdef.HealthCheckFunc
	Interval time.Duration
}

// Mock is a test implementation of scheduler.Component.
// It validates inputs the same way the real scheduler does and records calls
// for test inspection via ScheduledChecks().
type Mock struct {
	mu        sync.Mutex
	scheduled []ScheduleCall
}

// New returns a mock scheduler for testing.
func New() *Mock { return &Mock{} }

// Schedule validates inputs and records the call. It mirrors the real
// scheduler's error conditions (empty source, nil fn, duplicate source).
func (m *Mock) Schedule(source string, fn runnerdef.HealthCheckFunc, interval time.Duration, _ []string) error {
	if source == "" {
		return errors.New("source cannot be empty")
	}
	if fn == nil {
		return errors.New("health check function cannot be nil")
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	for _, s := range m.scheduled {
		if s.Source == source {
			return fmt.Errorf("health check for source %q is already registered", source)
		}
	}
	m.scheduled = append(m.scheduled, ScheduleCall{Source: source, Fn: fn, Interval: interval})
	return nil
}

// ScheduledChecks returns a snapshot of all registered Schedule() calls.
func (m *Mock) ScheduledChecks() []ScheduleCall {
	m.mu.Lock()
	defer m.mu.Unlock()
	out := make([]ScheduleCall, len(m.scheduled))
	copy(out, m.scheduled)
	return out
}

// ensure Mock satisfies the interface at compile time.
var _ schedulerdef.Component = (*Mock)(nil)
