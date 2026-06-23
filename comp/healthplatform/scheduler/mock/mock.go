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

// mockScheduler is a test implementation of scheduler.Component.
// Schedule validates inputs the same way the real scheduler does
// (empty source, nil fn, duplicate source) but does not run any goroutines.
type mockScheduler struct {
	t          testing.TB
	mu         sync.Mutex
	registered map[string]struct{}
}

// New returns a mock scheduler for testing.
func New(t testing.TB) *mockScheduler {
	return &mockScheduler{t: t, registered: make(map[string]struct{})}
}

// Schedule validates inputs and records the source. It mirrors the real
// scheduler's error conditions (empty source, nil fn, duplicate source).
func (m *mockScheduler) Schedule(source string, fn runnerdef.HealthCheckFunc, _ time.Duration, _ []string) error {
	m.t.Helper()
	if source == "" {
		return errors.New("source cannot be empty")
	}
	if fn == nil {
		return errors.New("health check function cannot be nil")
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	if _, exists := m.registered[source]; exists {
		return fmt.Errorf("health check for source %q is already registered", source)
	}
	m.registered[source] = struct{}{}
	return nil
}
