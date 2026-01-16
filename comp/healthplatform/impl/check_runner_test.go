// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

package healthplatformimpl

import (
	"context"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	logmock "github.com/DataDog/datadog-agent/comp/core/log/mock"
	healthplatform "github.com/DataDog/datadog-agent/comp/healthplatform/def"
)

// mockReporter is a mock implementation of issueReporter for testing
type mockReporter struct {
	reportedIssues map[string]*healthplatform.IssueReport
	reportCount    int32
}

func newMockReporter() *mockReporter {
	return &mockReporter{
		reportedIssues: make(map[string]*healthplatform.IssueReport),
	}
}

func (m *mockReporter) ReportIssue(checkID string, _ string, report *healthplatform.IssueReport) error {
	atomic.AddInt32(&m.reportCount, 1)
	m.reportedIssues[checkID] = report
	return nil
}

// TestCheckRunnerRegisterCheck tests registering a health check
func TestCheckRunnerRegisterCheck(t *testing.T) {
	reporter := newMockReporter()
	runner := newCheckRunner(logmock.New(t), reporter)

	checkCalled := false
	checkFn := func() (*healthplatform.IssueReport, error) {
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
	reporter := newMockReporter()
	runner := newCheckRunner(logmock.New(t), reporter)

	// Empty check ID
	err := runner.RegisterCheck("", "Test Check", func() (*healthplatform.IssueReport, error) { return nil, nil }, 0)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "check ID cannot be empty")

	// Nil check function
	err = runner.RegisterCheck("test-check", "Test Check", nil, 0)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "check function cannot be nil")

	// Duplicate registration
	checkFn := func() (*healthplatform.IssueReport, error) { return nil, nil }
	err = runner.RegisterCheck("test-check", "Test Check", checkFn, 0)
	require.NoError(t, err)

	err = runner.RegisterCheck("test-check", "Test Check 2", checkFn, 0)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "already registered")
}

// TestCheckRunnerDefaultInterval tests that default interval is used when not specified
func TestCheckRunnerDefaultInterval(t *testing.T) {
	reporter := newMockReporter()
	runner := newCheckRunner(logmock.New(t), reporter)

	checkFn := func() (*healthplatform.IssueReport, error) { return nil, nil }

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
	reporter := newMockReporter()
	runner := newCheckRunner(logmock.New(t), reporter)

	callCount := int32(0)
	checkFn := func() (*healthplatform.IssueReport, error) {
		atomic.AddInt32(&callCount, 1)
		return &healthplatform.IssueReport{
			IssueID: "test-issue",
		}, nil
	}

	// Register with very short interval for testing
	err := runner.RegisterCheck("test-check", "Test Check", checkFn, 50*time.Millisecond)
	require.NoError(t, err)

	// Start the runner
	runner.Start()
	defer runner.Stop()

	// Wait for at least 2 executions
	time.Sleep(150 * time.Millisecond)

	assert.GreaterOrEqual(t, atomic.LoadInt32(&callCount), int32(2))
	assert.GreaterOrEqual(t, atomic.LoadInt32(&reporter.reportCount), int32(2))
}

// TestCheckRunnerReportsIssues tests that issues are reported correctly
func TestCheckRunnerReportsIssues(t *testing.T) {
	reporter := newMockReporter()
	runner := newCheckRunner(logmock.New(t), reporter)

	checkFn := func() (*healthplatform.IssueReport, error) {
		return &healthplatform.IssueReport{
			IssueID: "test-issue-id",
			Context: map[string]string{"key": "value"},
		}, nil
	}

	err := runner.RegisterCheck("test-check", "Test Check", checkFn, 50*time.Millisecond)
	require.NoError(t, err)

	runner.Start()
	defer runner.Stop()

	// Wait for check to run
	time.Sleep(100 * time.Millisecond)

	// Verify issue was reported
	report := reporter.reportedIssues["test-check"]
	require.NotNil(t, report)
	assert.Equal(t, "test-issue-id", report.IssueID)
	assert.Equal(t, "value", report.Context["key"])
}

// TestCheckRunnerClearsIssueWhenNil tests that nil report clears issues
func TestCheckRunnerClearsIssueWhenNil(t *testing.T) {
	reporter := newMockReporter()
	runner := newCheckRunner(logmock.New(t), reporter)

	// First return an issue, then nil
	returnIssue := true
	checkFn := func() (*healthplatform.IssueReport, error) {
		if returnIssue {
			returnIssue = false
			return &healthplatform.IssueReport{IssueID: "test-issue"}, nil
		}
		return nil, nil // No issue
	}

	err := runner.RegisterCheck("test-check", "Test Check", checkFn, 50*time.Millisecond)
	require.NoError(t, err)

	runner.Start()
	defer runner.Stop()

	// Wait for both runs
	time.Sleep(150 * time.Millisecond)

	// Verify nil was reported (clearing the issue)
	assert.Nil(t, reporter.reportedIssues["test-check"])
}

// TestCheckRunnerStartStop tests graceful start/stop
func TestCheckRunnerStartStop(t *testing.T) {
	reporter := newMockReporter()
	runner := newCheckRunner(logmock.New(t), reporter)

	checkFn := func() (*healthplatform.IssueReport, error) {
		return nil, nil
	}

	err := runner.RegisterCheck("test-check", "Test Check", checkFn, 10*time.Millisecond)
	require.NoError(t, err)

	// Start and stop should complete gracefully
	runner.Start()
	time.Sleep(50 * time.Millisecond)
	runner.Stop() // Stop blocks until all goroutines finish

	// Verify runner is no longer running
	runner.checkMux.RLock()
	assert.False(t, runner.started)
	runner.checkMux.RUnlock()
}

// TestCheckRunnerWithComponent tests check registration through the health platform component
func TestCheckRunnerWithComponent(t *testing.T) {
	lifecycle := newMockLifecycle()
	reqs := testRequires(t, lifecycle)
	reqs.Config.SetWithoutSource("health_platform.enabled", true)

	provides, err := NewComponent(reqs)
	require.NoError(t, err)

	comp := provides.Comp.(*healthPlatformImpl)
	require.NotNil(t, comp.checkRunner)

	callCount := int32(0)
	checkFn := func() (*healthplatform.IssueReport, error) {
		atomic.AddInt32(&callCount, 1)
		return nil, nil
	}

	// Register check before starting
	err = comp.RegisterCheck("test-check", "Test Check", checkFn, 50*time.Millisecond)
	require.NoError(t, err)

	// Start component
	err = lifecycle.Start(context.Background())
	require.NoError(t, err)

	// Wait for check to run
	time.Sleep(100 * time.Millisecond)

	assert.GreaterOrEqual(t, atomic.LoadInt32(&callCount), int32(1))

	// Stop component
	err = lifecycle.Stop(context.Background())
	require.NoError(t, err)
}
