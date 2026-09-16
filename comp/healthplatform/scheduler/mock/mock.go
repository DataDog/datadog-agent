// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

//go:build test

// Package mock provides a mock for the health platform scheduler component.
package mock

import (
	"errors"
	"fmt"
	"sync"
	"testing"
	"time"

	runnerdef "github.com/DataDog/datadog-agent/comp/healthplatform/runner/def"
	storedef "github.com/DataDog/datadog-agent/comp/healthplatform/store/def"
)

// Mock is a test implementation of scheduler.Component. It validates inputs
// the same way the real scheduler does (empty source, nil fn, duplicate
// source) and runs each check once synchronously via the injected runner
// when Schedule is called, mirroring the real scheduler's immediate
// first-tick behaviour without background goroutines. initialIssueIDs is
// diffed against that first run's result and resolved through the store,
// exactly like the real scheduler seeding lastIssueIDs from it.
type Mock struct {
	t          testing.TB
	runner     runnerdef.Component
	store      storedef.Component
	mu         sync.Mutex
	registered map[string]struct{}
}

// New returns a mock scheduler for testing. runner executes each registered
// check; store is where initialIssueIDs the first run no longer reports get
// resolved, mirroring the real scheduler's dependency graph.
func New(t testing.TB, runner runnerdef.Component, store storedef.Component) *Mock {
	return &Mock{t: t, runner: runner, store: store, registered: make(map[string]struct{})}
}

// Schedule validates inputs, records the source, and runs fn once
// synchronously via the injected runner, mirroring the real scheduler's
// immediate first-tick on registration. initialIssueIDs seeds the diff for
// that first run exactly like the real scheduler: any ID in initialIssueIDs
// that the run no longer reports is resolved through the store.
func (m *Mock) Schedule(source string, fn runnerdef.HealthCheckFunc, _ time.Duration, initialIssueIDs []string) error {
	m.t.Helper()
	if source == "" {
		return errors.New("source cannot be empty")
	}
	if fn == nil {
		return errors.New("health check function cannot be nil")
	}
	m.mu.Lock()
	if _, exists := m.registered[source]; exists {
		m.mu.Unlock()
		return fmt.Errorf("health check for source %q is already registered", source)
	}
	m.registered[source] = struct{}{}
	m.mu.Unlock()

	newIDs, err := m.runner.Run(source, fn)
	if err != nil {
		return nil
	}

	newSet := make(map[string]struct{}, len(newIDs))
	for _, id := range newIDs {
		newSet[id] = struct{}{}
	}
	for _, id := range initialIssueIDs {
		if _, still := newSet[id]; !still {
			m.store.ResolveIssue(id)
		}
	}
	return nil
}
