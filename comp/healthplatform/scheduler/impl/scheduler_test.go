// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

//go:build test

package schedulerimpl

import (
	"context"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	healthplatformpayload "github.com/DataDog/agent-payload/v5/healthplatform"

	logmock "github.com/DataDog/datadog-agent/comp/core/log/mock"
)

// mockReporter is a simple mock for testing check runner registration/validation
type mockReporter struct {
	reportCount int32
}

func newMockReporter() *mockReporter {
	return &mockReporter{}
}

func (m *mockReporter) ReportIssue(_ string, _ string, _ *healthplatformpayload.IssueReport) error {
	atomic.AddInt32(&m.reportCount, 1)
	return nil
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

// TestCheckRunnerRegisterCheck tests registering a health check
func TestCheckRunnerRegisterCheck(t *testing.T) {
	runner, _ := newTestRunner(t)

	checkCalled := false
	checkFn := func() (*healthplatformpayload.IssueReport, error) {
		checkCalled = true
		return nil, nil
	}

	err := runner.RegisterCheck("test-check", "Test Check", checkFn, 1*time.Minute)
	require.NoError(t, err)

	// Verify check is registered
	runner.checkMux.RLock()
	_, exists := runner.checks["test-check"]
	runner.checkMux.RUnlock()
	assert.True(t, exists)

	// Verify check hasn't been called yet (runner not started)
	assert.False(t, checkCalled)
}

// TestCheckRunnerRegisterCheckValidation tests validation of RegisterCheck
func TestCheckRunnerRegisterCheckValidation(t *testing.T) {
	runner, _ := newTestRunner(t)

	// Empty check ID
	err := runner.RegisterCheck("", "Test Check", func() (*healthplatformpayload.IssueReport, error) { return nil, nil }, 0)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "check ID cannot be empty")

	// Nil check function
	err = runner.RegisterCheck("test-check", "Test Check", nil, 0)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "check function cannot be nil")

	// Duplicate registration
	checkFn := func() (*healthplatformpayload.IssueReport, error) { return nil, nil }
	err = runner.RegisterCheck("test-check", "Test Check", checkFn, 0)
	require.NoError(t, err)

	err = runner.RegisterCheck("test-check", "Test Check 2", checkFn, 0)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "already registered")
}

// TestCheckRunnerDefaultInterval tests that default interval is used when not specified
func TestCheckRunnerDefaultInterval(t *testing.T) {
	runner, _ := newTestRunner(t)

	checkFn := func() (*healthplatformpayload.IssueReport, error) { return nil, nil }

	// Zero interval should use default
	err := runner.RegisterCheck("test-check", "Test Check", checkFn, 0)
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
	err := runner.RegisterCheck("test-check", "Test Check", checkFn, 50*time.Millisecond)
	require.NoError(t, err)

	// Start the runner
	runner.start(context.Background())      //nolint:errcheck
	defer runner.stop(context.Background()) //nolint:errcheck

	// Wait for at least 2 executions
	assert.Eventually(t, func() bool {
		return atomic.LoadInt32(&callCount) >= 2 && atomic.LoadInt32(&reporter.reportCount) >= 2
	}, 500*time.Millisecond, 10*time.Millisecond)
}

// TestCheckRunnerStartStop tests graceful start/stop
func TestCheckRunnerStartStop(t *testing.T) {
	runner, reporter := newTestRunner(t)

	checkFn := func() (*healthplatformpayload.IssueReport, error) {
		return nil, nil
	}

	err := runner.RegisterCheck("test-check", "Test Check", checkFn, 10*time.Millisecond)
	require.NoError(t, err)

	// Start and stop should complete gracefully
	runner.start(context.Background()) //nolint:errcheck

	// Wait for at least one execution
	assert.Eventually(t, func() bool {
		return atomic.LoadInt32(&reporter.reportCount) >= 1
	}, 100*time.Millisecond, 5*time.Millisecond)

	runner.stop(context.Background()) //nolint:errcheck // Stop blocks until all goroutines finish

	// Verify runner is no longer running
	runner.checkMux.RLock()
	assert.False(t, runner.started)
	runner.checkMux.RUnlock()
}
