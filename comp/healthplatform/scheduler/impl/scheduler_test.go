// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

//go:build test

package schedulerimpl

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	healthplatformpayload "github.com/DataDog/agent-payload/v5/healthplatform"

	logmock "github.com/DataDog/datadog-agent/comp/core/log/mock"
	runnerdef "github.com/DataDog/datadog-agent/comp/healthplatform/runner/def"
	storedef "github.com/DataDog/datadog-agent/comp/healthplatform/store/def"
)

// mockRunner records Run calls and returns the configured response.
type mockRunner struct {
	mu       sync.Mutex
	calls    []string
	response func(source string) ([]string, error)
}

func (m *mockRunner) Run(source string, _ runnerdef.HealthCheckFunc) ([]string, error) {
	m.mu.Lock()
	m.calls = append(m.calls, source)
	resp := m.response
	m.mu.Unlock()
	if resp != nil {
		return resp(source)
	}
	return nil, nil
}

// mockStore records ResolveIssue calls.
type mockStore struct {
	mu          sync.Mutex
	resolvedIDs []string
}

func (m *mockStore) ResolveIssue(id string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.resolvedIDs = append(m.resolvedIDs, id)
}
func (m *mockStore) ReportIssue(_ storedef.IssueReport) error                     { return nil }
func (m *mockStore) ResolveAllIssues()                                            {}
func (m *mockStore) GetIssue(_ string) *healthplatformpayload.Issue               { return nil }
func (m *mockStore) GetAllIssues() (int, map[string]*healthplatformpayload.Issue) { return 0, nil }
func (m *mockStore) GetActiveIssueIDsByIssueType(_ string) []string               { return nil }

func newTestScheduler(t *testing.T, runner runnerdef.Component, store storedef.Component) *scheduler {
	t.Helper()
	return &scheduler{
		log:    logmock.New(t),
		runner: runner,
		store:  store,
		checks: make(map[string]*registeredHealthCheck),
	}
}

func TestScheduleRegisters(t *testing.T) {
	s := newTestScheduler(t, &mockRunner{}, &mockStore{})
	fn := func() ([]storedef.IssueReport, error) { return nil, nil }

	require.NoError(t, s.Schedule("mycomp", fn, time.Minute, nil))

	s.checkMux.RLock()
	_, exists := s.checks["mycomp"]
	s.checkMux.RUnlock()
	assert.True(t, exists)
}

func TestScheduleValidation(t *testing.T) {
	s := newTestScheduler(t, &mockRunner{}, &mockStore{})
	fn := func() ([]storedef.IssueReport, error) { return nil, nil }

	assert.Error(t, s.Schedule("", fn, time.Minute, nil))
	assert.Error(t, s.Schedule("mycomp", nil, time.Minute, nil))
}

func TestScheduleDuplicateSource(t *testing.T) {
	s := newTestScheduler(t, &mockRunner{}, &mockStore{})
	fn := func() ([]storedef.IssueReport, error) { return nil, nil }

	require.NoError(t, s.Schedule("mycomp", fn, time.Minute, nil))
	assert.Error(t, s.Schedule("mycomp", fn, time.Minute, nil))
}

func TestScheduleDefaultInterval(t *testing.T) {
	s := newTestScheduler(t, &mockRunner{}, &mockStore{})
	fn := func() ([]storedef.IssueReport, error) { return nil, nil }

	require.NoError(t, s.Schedule("mycomp", fn, 0, nil))

	s.checkMux.RLock()
	check := s.checks["mycomp"]
	s.checkMux.RUnlock()
	assert.Equal(t, defaultCheckInterval, check.interval)
}

func TestTickDiffResolveDisappeared(t *testing.T) {
	mr := &mockRunner{
		response: func(_ string) ([]string, error) { return []string{"A", "B"}, nil },
	}
	ms := &mockStore{}
	s := newTestScheduler(t, mr, ms)

	check := &registeredHealthCheck{
		source:       "mycomp",
		fn:           func() ([]storedef.IssueReport, error) { return nil, nil },
		lastIssueIDs: make(map[string]struct{}),
		stopCh:       make(chan struct{}),
	}
	s.checks["mycomp"] = check

	// Tick 1: emits A, B — no resolves yet.
	s.tick(check)
	ms.mu.Lock()
	assert.Empty(t, ms.resolvedIDs)
	ms.mu.Unlock()

	// Tick 2: emits only A — B should be resolved.
	mr.mu.Lock()
	mr.response = func(_ string) ([]string, error) { return []string{"A"}, nil }
	mr.mu.Unlock()

	s.tick(check)
	ms.mu.Lock()
	assert.Equal(t, []string{"B"}, ms.resolvedIDs)
	ms.mu.Unlock()
}

func TestTickEmptyResultResolvesAll(t *testing.T) {
	mr := &mockRunner{
		response: func(_ string) ([]string, error) { return []string{"A", "B"}, nil },
	}
	ms := &mockStore{}
	s := newTestScheduler(t, mr, ms)

	check := &registeredHealthCheck{
		source:       "mycomp",
		fn:           func() ([]storedef.IssueReport, error) { return nil, nil },
		lastIssueIDs: make(map[string]struct{}),
		stopCh:       make(chan struct{}),
	}
	s.checks["mycomp"] = check

	s.tick(check)

	mr.mu.Lock()
	mr.response = func(_ string) ([]string, error) { return nil, nil }
	mr.mu.Unlock()

	s.tick(check)

	ms.mu.Lock()
	assert.ElementsMatch(t, []string{"A", "B"}, ms.resolvedIDs)
	ms.mu.Unlock()
}

func TestTickErrorDoesNotResolveActiveIssues(t *testing.T) {
	// First tick succeeds and records A, B as active.
	mr := &mockRunner{
		response: func(_ string) ([]string, error) { return []string{"A", "B"}, nil },
	}
	ms := &mockStore{}
	s := newTestScheduler(t, mr, ms)

	check := &registeredHealthCheck{
		source:       "mycomp",
		fn:           func() ([]storedef.IssueReport, error) { return nil, nil },
		lastIssueIDs: make(map[string]struct{}),
		stopCh:       make(chan struct{}),
	}
	s.checks["mycomp"] = check

	s.tick(check)
	ms.mu.Lock()
	assert.Empty(t, ms.resolvedIDs)
	ms.mu.Unlock()

	// Second tick errors — lastIssueIDs must not be updated and no resolves fired.
	mr.mu.Lock()
	mr.response = func(_ string) ([]string, error) {
		return nil, errors.New("transient probe failure")
	}
	mr.mu.Unlock()

	s.tick(check)

	ms.mu.Lock()
	assert.Empty(t, ms.resolvedIDs, "a transient error must not resolve active issues")
	ms.mu.Unlock()

	// Confirm lastIssueIDs is still {A, B} (not cleared by the error tick).
	s.checkMux.RLock()
	assert.Equal(t, map[string]struct{}{"A": {}, "B": {}}, check.lastIssueIDs)
	s.checkMux.RUnlock()
}

func TestSchedulerLifecycle(t *testing.T) {
	var callCount int32
	mr := &mockRunner{
		response: func(_ string) ([]string, error) {
			atomic.AddInt32(&callCount, 1)
			return nil, nil
		},
	}
	s := newTestScheduler(t, mr, &mockStore{})

	fn := func() ([]storedef.IssueReport, error) { return nil, nil }
	require.NoError(t, s.Schedule("mycomp", fn, 20*time.Millisecond, nil))

	require.NoError(t, s.start(context.Background()))

	assert.Eventually(t, func() bool { return atomic.LoadInt32(&callCount) >= 2 },
		500*time.Millisecond, 10*time.Millisecond)

	require.NoError(t, s.stop(context.Background()))
	// stop() calls wg.Wait() so all goroutines have exited.
	// Any tick in-flight when stopCh closed has now completed; count is frozen.
	assert.GreaterOrEqual(t, atomic.LoadInt32(&callCount), int32(2))
}
