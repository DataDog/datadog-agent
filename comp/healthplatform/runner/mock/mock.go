// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

//go:build test

// Package mock provides a mock implementation of the health platform runner component.
package mock

import (
	"fmt"
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
//
// If fn returns both reports and a non-nil error, the reports are still
// forwarded to the store before the error is returned — mirroring the real
// runner, which does not silently drop partial results on error.
//
// A panic inside fn is recovered, mirroring the real runner: it becomes a
// "health check panic: ..." error and ids is zeroed, so scheduler callers
// keep their previous issue state instead of crashing.
func (m *Mock) Run(source string, fn runnerdef.HealthCheckFunc) (ids []string, retErr error) {
	m.t.Helper()
	if m.runFn != nil {
		return m.runFn(source, fn)
	}
	if fn == nil {
		return nil, nil
	}
	defer func() {
		if rec := recover(); rec != nil {
			retErr = fmt.Errorf("health check panic: %v", rec)
			ids = nil
		}
	}()
	reports, err := fn()
	for _, r := range reports {
		if r.Source == "" {
			r.Source = source
		}
		issue := &healthplatformpayload.Issue{
			Id:        r.IssueID,
			IssueName: r.IssueName,
			Source:    r.Source,
			Tags:      r.Tags,
		}
		if reportErr := m.store.ReportIssue(issue); reportErr == nil {
			ids = append(ids, r.IssueID)
		}
	}
	retErr = err
	return ids, retErr
}
