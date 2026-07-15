// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

//go:build test

package schedulerimpl

import (
	"context"
	"errors"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	logmock "github.com/DataDog/datadog-agent/comp/core/log/mock"
	runnerdef "github.com/DataDog/datadog-agent/comp/healthplatform/runner/def"
	runnermock "github.com/DataDog/datadog-agent/comp/healthplatform/runner/mock"
	storedef "github.com/DataDog/datadog-agent/comp/healthplatform/store/def"
	storemock "github.com/DataDog/datadog-agent/comp/healthplatform/store/mock"
)

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
	store := storemock.New(t)
	runner := runnermock.New(t, store)
	s := newTestScheduler(t, runner, store)
	fn := func() ([]runnerdef.IssueReport, error) { return nil, nil }

	require.NoError(t, s.Schedule("mycomp", fn, time.Minute, nil))

	s.checkMux.RLock()
	_, exists := s.checks["mycomp"]
	s.checkMux.RUnlock()
	assert.True(t, exists)
}

func TestScheduleValidation(t *testing.T) {
	store := storemock.New(t)
	runner := runnermock.New(t, store)
	s := newTestScheduler(t, runner, store)
	fn := func() ([]runnerdef.IssueReport, error) { return nil, nil }

	assert.Error(t, s.Schedule("", fn, time.Minute, nil))
	assert.Error(t, s.Schedule("mycomp", nil, time.Minute, nil))
}

func TestScheduleDuplicateSource(t *testing.T) {
	store := storemock.New(t)
	runner := runnermock.New(t, store)
	s := newTestScheduler(t, runner, store)
	fn := func() ([]runnerdef.IssueReport, error) { return nil, nil }

	require.NoError(t, s.Schedule("mycomp", fn, time.Minute, nil))
	assert.Error(t, s.Schedule("mycomp", fn, time.Minute, nil))
}

func TestScheduleDefaultInterval(t *testing.T) {
	store := storemock.New(t)
	runner := runnermock.New(t, store)
	s := newTestScheduler(t, runner, store)
	fn := func() ([]runnerdef.IssueReport, error) { return nil, nil }

	require.NoError(t, s.Schedule("mycomp", fn, 0, nil))

	s.checkMux.RLock()
	check := s.checks["mycomp"]
	s.checkMux.RUnlock()
	assert.Equal(t, defaultCheckInterval, check.interval)
}

func TestTickDiffResolveDisappeared(t *testing.T) {
	store := storemock.New(t)
	runner := runnermock.New(t, store, runnermock.WithRunFunc(
		func(_ string, _ runnerdef.HealthCheckFunc) ([]string, error) {
			return []string{"A", "B"}, nil
		},
	))
	s := newTestScheduler(t, runner, store)

	check := &registeredHealthCheck{
		source:       "mycomp",
		fn:           func() ([]runnerdef.IssueReport, error) { return nil, nil },
		lastIssueIDs: make(map[string]struct{}),
		stopCh:       make(chan struct{}),
	}
	s.checks["mycomp"] = check

	// Tick 1: emits A, B — no resolves yet.
	s.tick(check)
	assert.Empty(t, store.ResolvedIDs())

	// Tick 2: emits only A — B should be resolved.
	runner2 := runnermock.New(t, store, runnermock.WithRunFunc(
		func(_ string, _ runnerdef.HealthCheckFunc) ([]string, error) {
			return []string{"A"}, nil
		},
	))
	s.runner = runner2

	s.tick(check)
	assert.Equal(t, []string{"B"}, store.ResolvedIDs())
}

func TestTickEmptyResultResolvesAll(t *testing.T) {
	store := storemock.New(t)
	runner := runnermock.New(t, store, runnermock.WithRunFunc(
		func(_ string, _ runnerdef.HealthCheckFunc) ([]string, error) {
			return []string{"A", "B"}, nil
		},
	))
	s := newTestScheduler(t, runner, store)

	check := &registeredHealthCheck{
		source:       "mycomp",
		fn:           func() ([]runnerdef.IssueReport, error) { return nil, nil },
		lastIssueIDs: make(map[string]struct{}),
		stopCh:       make(chan struct{}),
	}
	s.checks["mycomp"] = check

	s.tick(check)

	runner2 := runnermock.New(t, store, runnermock.WithRunFunc(
		func(_ string, _ runnerdef.HealthCheckFunc) ([]string, error) {
			return nil, nil
		},
	))
	s.runner = runner2

	s.tick(check)
	assert.ElementsMatch(t, []string{"A", "B"}, store.ResolvedIDs())
}

func TestTickErrorDoesNotResolveActiveIssues(t *testing.T) {
	store := storemock.New(t)
	runner := runnermock.New(t, store, runnermock.WithRunFunc(
		func(_ string, _ runnerdef.HealthCheckFunc) ([]string, error) {
			return []string{"A", "B"}, nil
		},
	))
	s := newTestScheduler(t, runner, store)

	check := &registeredHealthCheck{
		source:       "mycomp",
		fn:           func() ([]runnerdef.IssueReport, error) { return nil, nil },
		lastIssueIDs: make(map[string]struct{}),
		stopCh:       make(chan struct{}),
	}
	s.checks["mycomp"] = check

	s.tick(check)
	assert.Empty(t, store.ResolvedIDs())

	// Second tick errors — lastIssueIDs must not be updated and no resolves fired.
	runner2 := runnermock.New(t, store, runnermock.WithRunFunc(
		func(_ string, _ runnerdef.HealthCheckFunc) ([]string, error) {
			return nil, errors.New("transient probe failure")
		},
	))
	s.runner = runner2

	s.tick(check)
	assert.Empty(t, store.ResolvedIDs(), "a transient error must not resolve active issues")

	// Confirm lastIssueIDs is still {A, B} (not cleared by the error tick).
	s.checkMux.RLock()
	assert.Equal(t, map[string]struct{}{"A": {}, "B": {}}, check.lastIssueIDs)
	s.checkMux.RUnlock()
}

func TestSchedulerLifecycle(t *testing.T) {
	var callCount int32
	store := storemock.New(t)
	runner := runnermock.New(t, store, runnermock.WithRunFunc(
		func(_ string, _ runnerdef.HealthCheckFunc) ([]string, error) {
			atomic.AddInt32(&callCount, 1)
			return nil, nil
		},
	))
	s := newTestScheduler(t, runner, store)

	fn := func() ([]runnerdef.IssueReport, error) { return nil, nil }
	require.NoError(t, s.Schedule("mycomp", fn, 20*time.Millisecond, nil))

	require.NoError(t, s.start(context.Background()))

	assert.Eventually(t, func() bool { return atomic.LoadInt32(&callCount) >= 2 },
		500*time.Millisecond, 10*time.Millisecond)

	require.NoError(t, s.stop(context.Background()))
	assert.GreaterOrEqual(t, atomic.LoadInt32(&callCount), int32(2))
}
