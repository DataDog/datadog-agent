// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

//go:build test

// Package mock provides a mock for the health platform forwarder component.
package mock

import (
	"context"
	"testing"

	healthplatformpayload "github.com/DataDog/agent-payload/v5/healthplatform"
)

// Mock delegates Send to a user-supplied function so tests can
// inline capture and error-injection logic without extra mock methods.
type Mock struct {
	t      testing.TB
	sendFn func(context.Context, *healthplatformpayload.HealthReport) error
}

// Option configures the mock forwarder returned by New.
type Option func(*Mock)

// WithSendFunc sets the function called by Send. Use it to capture payloads
// or inject errors inline in the test:
//
//	var count int32
//	fwd := forwardermock.New(t, forwardermock.WithSendFunc(
//	    func(_ context.Context, r *healthplatformpayload.HealthReport) error {
//	        atomic.AddInt32(&count, 1)
//	        return nil
//	    },
//	))
func WithSendFunc(fn func(context.Context, *healthplatformpayload.HealthReport) error) Option {
	return func(m *Mock) { m.sendFn = fn }
}

// New returns a mock forwarder for testing. Without options Send is a no-op.
func New(t testing.TB, opts ...Option) *Mock {
	m := &Mock{t: t}
	for _, o := range opts {
		o(m)
	}
	return m
}

// Send calls the configured sendFn, or returns nil if none was set.
func (m *Mock) Send(ctx context.Context, report *healthplatformpayload.HealthReport) error {
	m.t.Helper()
	if m.sendFn == nil {
		return nil
	}
	return m.sendFn(ctx, report)
}
