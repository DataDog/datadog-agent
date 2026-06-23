// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

//go:build test

// Package mock provides a mock for the health platform scheduler component.
package mock

import (
	"errors"
	"fmt"
	"sync"
	"testing"
	"time"

	runnerdef "github.com/DataDog/datadog-agent/comp/healthplatform/runner/def"
)

// Mock is a test implementation of scheduler.Component.
// It validates inputs the same way the real scheduler does (empty source,
// nil fn, duplicate source) and runs each check once synchronously via the
// injected runner when Schedule is called, mirroring the real scheduler's
// first-tick behaviour without background goroutines.
type Mock struct {
	t          testing.TB
	runner     runnerdef.Component
	mu         sync.Mutex
	registered map[string]struct{}
}

// New returns a mock scheduler for testing.
// runner is used to execute each registered check once synchronously.
// New returns a mock scheduler for testing.
// runner is used to execute each registered check once synchronously.
func New(t testing.TB, runner runnerdef.Component) *Mock {
	return &Mock{t: t, runner: runner, registered: make(map[string]struct{})}
}

// Schedule validates inputs, records the source, and runs fn once
// synchronously via the injected runner, mirroring the real scheduler's
// immediate first-tick on registration.
func (m *Mock) Schedule(source string, fn runnerdef.HealthCheckFunc, _ time.Duration, _ []string) error {
	m.t.Helper()
	if source == "" {
		return errors.New("source cannot be empty")
	}
	if fn == nil {
		return errors.New("health check function cannot be nil")
	}
	m.mu.Lock()
	if _, exists := m.registered[source]; exists {
		m.mu.Unlock()
		return fmt.Errorf("health check for source %q is already registered", source)
	}
	m.registered[source] = struct{}{}
	m.mu.Unlock()

	_, _ = m.runner.Run(source, fn)
	return nil
}
