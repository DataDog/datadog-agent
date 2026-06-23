// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

//go:build test

// Package mock provides a mock for the health platform forwarder component.
package mock

import (
	"context"
	"sync"
	"sync/atomic"
	"testing"

	healthplatformpayload "github.com/DataDog/agent-payload/v5/healthplatform"
)

// mockForwarder records Send calls and supports configurable error injection.
type mockForwarder struct {
	t         testing.TB
	mu        sync.Mutex
	reports   []*healthplatformpayload.HealthReport
	sendErr   error
	sendCount atomic.Int32
}

// New returns a mock forwarder for testing.
func New(t testing.TB) *mockForwarder { return &mockForwarder{t: t} }

// SetSendError configures the error returned by Send.
func (m *mockForwarder) SetSendError(err error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.sendErr = err
}

// Send records the report and returns any configured error.
func (m *mockForwarder) Send(_ context.Context, report *healthplatformpayload.HealthReport) error {
	m.t.Helper()
	m.sendCount.Add(1)
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.sendErr != nil {
		return m.sendErr
	}
	m.reports = append(m.reports, report)
	return nil
}

// SendCallCount returns the total number of Send calls (including failed ones).
func (m *mockForwarder) SendCallCount() int32 {
	return m.sendCount.Load()
}

// SendCalls returns the reports captured by successful Send calls.
func (m *mockForwarder) SendCalls() []*healthplatformpayload.HealthReport {
	m.mu.Lock()
	defer m.mu.Unlock()
	out := make([]*healthplatformpayload.HealthReport, len(m.reports))
	copy(out, m.reports)
	return out
}
