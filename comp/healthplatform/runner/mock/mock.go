// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

//go:build test

// Package mock provides a mock implementation of the health platform runner component.
package mock

import (
	"testing"

	healthplatformpayload "github.com/DataDog/agent-payload/v5/healthplatform"

	runnerdef "github.com/DataDog/datadog-agent/comp/healthplatform/runner/def"
	storedef "github.com/DataDog/datadog-agent/comp/healthplatform/store/def"
)

// mockRunner is a test implementation of runner.Component.
// It calls fn, forwards each emitted report to store.ReportIssue (using a
// minimal proto — no registry template lookup), and returns the reported IDs,
// mirroring the real runner's contract.
type mockRunner struct {
	t     testing.TB
	store storedef.Component
}

// New returns a mock runner for testing.
// store is used to forward issues, matching the real runner's behaviour.
func New(t testing.TB, store storedef.Component) *mockRunner {
	return &mockRunner{t: t, store: store}
}

// Run calls fn, reports each IssueReport to the store, and returns the IDs
// that were successfully reported. Returns nil ids on error.
func (m *mockRunner) Run(source string, fn runnerdef.HealthCheckFunc) ([]string, error) {
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
		if r.Source == "" {
			r.Source = source
		}
		issue := &healthplatformpayload.Issue{
			Id:        r.IssueID,
			IssueName: r.IssueName,
			Source:    r.Source,
		}
		if reportErr := m.store.ReportIssue(issue); reportErr == nil {
			ids = append(ids, r.IssueID)
		}
	}
	return ids, nil
}
