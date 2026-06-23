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

// mockForwarder is a no-op implementation of forwarder.Component.
type mockForwarder struct {
	t testing.TB
}

// New returns a mock forwarder for testing.
func New(t testing.TB) *mockForwarder { return &mockForwarder{t: t} }

// Send is a no-op that satisfies forwarder.Component.
func (m *mockForwarder) Send(_ context.Context, _ *healthplatformpayload.HealthReport) error {
	m.t.Helper()
	return nil
}
