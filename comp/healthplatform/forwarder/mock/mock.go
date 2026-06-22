// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

//go:build test

// Package mock provides a no-op mock for the health platform forwarder.
package mock

import (
	"context"
	"sync"

	healthplatformpayload "github.com/DataDog/agent-payload/v5/healthplatform"
	"google.golang.org/protobuf/proto"
)

// Mock is a test implementation of forwarder.Component that records every
// report passed to Send() so tests can assert on what would be forwarded.
type mockForwarder struct {
	mu      sync.Mutex
	reports []*healthplatformpayload.HealthReport
}

// New returns a mock forwarder for testing.
func New() *mockForwarder { return &mockForwarder{} }

// Send records a deep clone of the report. It never returns an error.
func (m *mockForwarder) Send(_ context.Context, report *healthplatformpayload.HealthReport) error {
	clone := proto.Clone(report).(*healthplatformpayload.HealthReport)
	m.mu.Lock()
	defer m.mu.Unlock()
	m.reports = append(m.reports, clone)
	return nil
}

// SentReports returns a snapshot of all reports passed to Send().
func (m *mockForwarder) SentReports() []*healthplatformpayload.HealthReport {
	m.mu.Lock()
	defer m.mu.Unlock()
	out := make([]*healthplatformpayload.HealthReport, len(m.reports))
	copy(out, m.reports)
	return out
}
