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

	forwarder "github.com/DataDog/datadog-agent/comp/healthplatform/forwarder/def"
)

// Mock is a test implementation of forwarder.Component that records every
// report passed to Send() so tests can assert on what would be forwarded.
type Mock struct {
	mu      sync.Mutex
	reports []*healthplatformpayload.HealthReport
}

// New returns a mock forwarder for testing.
func New() *Mock { return &Mock{} }

// Send records a deep clone of the report. It never returns an error.
func (m *Mock) Send(_ context.Context, report *healthplatformpayload.HealthReport) error {
	clone := proto.Clone(report).(*healthplatformpayload.HealthReport)
	m.mu.Lock()
	defer m.mu.Unlock()
	m.reports = append(m.reports, clone)
	return nil
}

// SentReports returns a snapshot of all reports passed to Send().
func (m *Mock) SentReports() []*healthplatformpayload.HealthReport {
	m.mu.Lock()
	defer m.mu.Unlock()
	out := make([]*healthplatformpayload.HealthReport, len(m.reports))
	copy(out, m.reports)
	return out
}

// ensure Mock satisfies the interface at compile time.
var _ forwarder.Component = (*Mock)(nil)
