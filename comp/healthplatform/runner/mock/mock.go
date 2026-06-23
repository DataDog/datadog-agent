// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

//go:build test

// Package mock provides a mock implementation of the health platform runner component.
package mock

import (
	"testing"

	runnerdef "github.com/DataDog/datadog-agent/comp/healthplatform/runner/def"
)

// mockRunner is a test implementation of runner.Component.
// Run calls fn and returns the IssueID of each emitted report, mirroring the
// real runner without the registry lookup or store interaction.
type mockRunner struct {
	t testing.TB
}

// New returns a mock runner for testing.
func New(t testing.TB) *mockRunner { return &mockRunner{t: t} }

// Run calls fn and collects the IssueID from each emitted IssueReport.
// Returns nil ids on error, matching the real runner's partial-result contract.
func (m *mockRunner) Run(_ string, fn runnerdef.HealthCheckFunc) ([]string, error) {
	m.t.Helper()
	if fn == nil {
		return nil, nil
	}
	reports, err := fn()
	if err != nil {
		return nil, err
	}
	ids := make([]string, 0, len(reports))
	for _, r := range reports {
		ids = append(ids, r.IssueID)
	}
	return ids, nil
}
