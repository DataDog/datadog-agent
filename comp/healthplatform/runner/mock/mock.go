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

// Mock is a test implementation of runner.Component.
type Mock struct {
	t     testing.TB
	store storedef.Component
	runFn func(source string, fn runnerdef.HealthCheckFunc) ([]string, error)
}

// Option configures the mock runner returned by New.
type Option func(*Mock)

// WithRunFunc overrides Run entirely. Use it when you need to control the
// returned IDs without executing a real health check function:
//
//	runnermock.New(t, store, runnermock.WithRunFunc(
//	    func(source string, _ runnerdef.HealthCheckFunc) ([]string, error) {
//	        return []string{"issue-1"}, nil
//	    },
//	))
func WithRunFunc(fn func(source string, fn runnerdef.HealthCheckFunc) ([]string, error)) Option {
	return func(m *Mock) { m.runFn = fn }
}

// New returns a mock runner for testing.
// store is used to forward issues when no WithRunFunc is set.
// New returns a mock runner for testing.
// store is used to forward issues when no WithRunFunc is set.
func New(t testing.TB, store storedef.Component, opts ...Option) *Mock {
	m := &Mock{t: t, store: store}
	for _, o := range opts {
		o(m)
	}
	return m
}

// Run delegates to the configured runFn if set, otherwise calls fn and
// reports each IssueReport to the store (mirroring the real runner without
// the registry template lookup).
func (m *Mock) Run(source string, fn runnerdef.HealthCheckFunc) ([]string, error) {
	m.t.Helper()
	if m.runFn != nil {
		return m.runFn(source, fn)
	}
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
