// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

//go:build test

package schedulerimpl

import (
	"context"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	healthplatformpayload "github.com/DataDog/agent-payload/v5/healthplatform"

	logmock "github.com/DataDog/datadog-agent/comp/core/log/mock"
	storedef "github.com/DataDog/datadog-agent/comp/healthplatform/store/def"
)

// mockReporter is a simple mock for testing health check scheduling/validation.
type mockReporter struct {
	reportCount  int32
	resolveCount int32
	mu           sync.Mutex
	lastReport   storedef.IssueReport
}

func newMockReporter() *mockReporter {
	return &mockReporter{}
}

func (m *mockReporter) ReportIssue(r storedef.IssueReport) error {
	m.mu.Lock()
	m.lastReport = r
	m.mu.Unlock()
	atomic.AddInt32(&m.reportCount, 1)
	return nil
}

func (m *mockReporter) ResolveIssue(_ string) {
	atomic.AddInt32(&m.resolveCount, 1)
}

// newTestRunner creates a checkRunner with reporter wired for testing
func newTestRunner(t *testing.T) (*checkRunner, *mockReporter) {
	t.Helper()
	reporter := newMockReporter()
	r := &checkRunner{
		log:    logmock.New(t),
		checks: make(map[string]*registeredCheck),
	}
	r.SetReporter(reporter)
	return r, reporter
}

// TestCheckRunnerScheduleHealthCheck tests registering a health check
func TestCheckRunnerScheduleHealthCheck(t *testing.T) {
	runner, _ := newTestRunner(t)

	checkCalled := false
	checkFn := func() (*healthplatformpayload.IssueReport, error) {
		checkCalled = true
		return nil, nil
	}

	err := runner.ScheduleHealthCheck("test-check", "Test Check", checkFn, 1*time.Minute)
	require.NoError(t, err)

	// Verify check is registered
	runner.checkMux.RLock()
	_, exists := runner.checks["test-check"]
	runner.checkMux.RUnlock()
	assert.True(t, exists)

	// Verify check hasn't been called yet (runner not started)
	assert.False(t, checkCalled)
}

// TestCheckRunnerScheduleHealthCheckValidation tests validation of ScheduleHealthCheck
func TestCheckRunnerScheduleHealthCheckValidation(t *testing.T) {
	runner, _ := newTestRunner(t)

	// Empty check ID
	err := runner.ScheduleHealthCheck("", "Test Check", func() (*healthplatformpayload.IssueReport, error) { return nil, nil }, 0)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "check ID cannot be empty")

	// Nil check function
	err = runner.ScheduleHealthCheck("test-check", "Test Check", nil, 0)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "check function cannot be nil")

	// Duplicate registration
	checkFn := func() (*healthplatformpayload.IssueReport, error) { return nil, nil }
	err = runner.ScheduleHealthCheck("test-check", "Test Check", checkFn, 0)
	require.NoError(t, err)

	err = runner.ScheduleHealthCheck("test-check", "Test Check 2", checkFn, 0)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "already registered")
}

// TestCheckRunnerDefaultInterval tests that default interval is used when not specified
func TestCheckRunnerDefaultInterval(t *testing.T) {
	runner, _ := newTestRunner(t)

	checkFn := func() (*healthplatformpayload.IssueReport, error) { return nil, nil }

	// Zero interval should use default
	err := runner.ScheduleHealthCheck("test-check", "Test Check", checkFn, 0)
	require.NoError(t, err)

	runner.checkMux.RLock()
	check := runner.checks["test-check"]
	runner.checkMux.RUnlock()

	assert.Equal(t, defaultCheckInterval, check.interval)
}

// TestCheckRunnerRunsChecks tests that registered checks are executed
func TestCheckRunnerRunsChecks(t *testing.T) {
	runner, reporter := newTestRunner(t)

	callCount := int32(0)
	checkFn := func() (*healthplatformpayload.IssueReport, error) {
		atomic.AddInt32(&callCount, 1)
		return nil, nil
	}

	// Register with very short interval for testing
	err := runner.ScheduleHealthCheck("test-check", "Test Check", checkFn, 50*time.Millisecond)
	require.NoError(t, err)

	// Start the runner
	runner.start(context.Background())      //nolint:errcheck
	defer runner.stop(context.Background()) //nolint:errcheck

	// The check returns nil so ResolveIssue (not ReportIssue) is called each tick.
	assert.Eventually(t, func() bool {
		return atomic.LoadInt32(&callCount) >= 2 && atomic.LoadInt32(&reporter.resolveCount) >= 2
	}, 500*time.Millisecond, 10*time.Millisecond)
}

// TestCheckRunnerStartStop tests graceful start/stop
func TestCheckRunnerStartStop(t *testing.T) {
	runner, reporter := newTestRunner(t)

	checkFn := func() (*healthplatformpayload.IssueReport, error) {
		return nil, nil
	}

	err := runner.ScheduleHealthCheck("test-check", "Test Check", checkFn, 10*time.Millisecond)
	require.NoError(t, err)

	// Start and stop should complete gracefully
	runner.start(context.Background()) //nolint:errcheck

	// Check returns nil, so ResolveIssue is called each tick (not ReportIssue).
	assert.Eventually(t, func() bool {
		return atomic.LoadInt32(&reporter.resolveCount) >= 1
	}, 100*time.Millisecond, 5*time.Millisecond)

	runner.stop(context.Background()) //nolint:errcheck // Stop blocks until all goroutines finish

	// Verify runner is no longer running
	runner.checkMux.RLock()
	assert.False(t, runner.started)
	runner.checkMux.RUnlock()
}

// TestExecuteCheckDoesNotOverrideSource verifies that the scheduler never sets Source
// in the IssueReport forwarded to the reporter. Issue templates own the Source field;
// setting it from checkName would shadow the template's value (e.g. "logs" becomes
// "Docker Socket Permissions").
func TestExecuteCheckDoesNotOverrideSource(t *testing.T) {
	runner, reporter := newTestRunner(t)

	checkFn := func() (*healthplatformpayload.IssueReport, error) {
		return &healthplatformpayload.IssueReport{
			IssueId: "some-issue-type",
		}, nil
	}

	runner.executeCheck(&registeredCheck{
		checkID:   "my-check-id",
		checkName: "My Check Display Name",
		checkFn:   checkFn,
		stopCh:    make(chan struct{}),
	})

	require.Equal(t, int32(1), atomic.LoadInt32(&reporter.reportCount))
	reporter.mu.Lock()
	got := reporter.lastReport
	reporter.mu.Unlock()

	assert.Empty(t, got.Source, "scheduler must not set Source; issue template owns that field")
	assert.Equal(t, "my-check-id", got.IssueID)
	assert.Equal(t, "some-issue-type", got.IssueType)
}
