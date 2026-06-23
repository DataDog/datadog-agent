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
// When a WithSendFunc option is provided it takes full control; otherwise the
// default behaviour captures reports and respects SetSendError.
type mockForwarder struct {
	t         testing.TB
	mu        sync.Mutex
	reports   []*healthplatformpayload.HealthReport
	sendErr   error
	sendCount atomic.Int32
	sendFn    func(context.Context, *healthplatformpayload.HealthReport) error
}

// Option configures the mock forwarder returned by New.
type Option func(*mockForwarder)

// WithSendFunc sets a custom function called by Send, overriding the default
// capture behaviour. Use it to inline complex send logic in the test.
func WithSendFunc(fn func(context.Context, *healthplatformpayload.HealthReport) error) Option {
	return func(m *mockForwarder) { m.sendFn = fn }
}

// New returns a mock forwarder for testing.
func New(t testing.TB, opts ...Option) *mockForwarder {
	m := &mockForwarder{t: t}
	for _, o := range opts {
		o(m)
	}
	return m
}

// SetSendError configures the error returned by Send (when no WithSendFunc is set).
func (m *mockForwarder) SetSendError(err error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.sendErr = err
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

// Send records the call. If a sendFn was configured it delegates there;
// otherwise it increments the counter, returns any configured error, and on
// success appends the report.
func (m *mockForwarder) Send(ctx context.Context, report *healthplatformpayload.HealthReport) error {
	m.t.Helper()
	m.sendCount.Add(1)
	if m.sendFn != nil {
		return m.sendFn(ctx, report)
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.sendErr != nil {
		return m.sendErr
	}
	m.reports = append(m.reports, report)
	return nil
}
