// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

//go:build test

package healthplatformimpl

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

// TestCheckRunnerRegisterCheck tests registering a health check
func TestCheckRunnerRegisterCheck(t *testing.T) {
	reporter := newMockReporter()
	runner := newCheckRunner(logmock.New(t), reporter)

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
	reporter := newMockReporter()
	runner := newCheckRunner(logmock.New(t), reporter)

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
	reporter := newMockReporter()
	runner := newCheckRunner(logmock.New(t), reporter)

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
	reporter := newMockReporter()
	runner := newCheckRunner(logmock.New(t), reporter)

	callCount := int32(0)
	checkFn := func() (*healthplatformpayload.IssueReport, error) {
		atomic.AddInt32(&callCount, 1)
		return nil, nil
	}

	// Register with very short interval for testing
	err := runner.RegisterCheck("test-check", "Test Check", checkFn, 50*time.Millisecond)
	require.NoError(t, err)

	// Start the runner
	runner.Start()
	defer runner.Stop()

	// Wait for at least 2 executions
	assert.Eventually(t, func() bool {
		return atomic.LoadInt32(&callCount) >= 2 && atomic.LoadInt32(&reporter.reportCount) >= 2
	}, 500*time.Millisecond, 10*time.Millisecond)
}

// TestCheckRunnerStartStop tests graceful start/stop
func TestCheckRunnerStartStop(t *testing.T) {
	reporter := newMockReporter()
	runner := newCheckRunner(logmock.New(t), reporter)

	checkFn := func() (*healthplatformpayload.IssueReport, error) {
		return nil, nil
	}

	err := runner.RegisterCheck("test-check", "Test Check", checkFn, 10*time.Millisecond)
	require.NoError(t, err)

	// Start and stop should complete gracefully
	runner.Start()

	// Wait for at least one execution
	assert.Eventually(t, func() bool {
		return atomic.LoadInt32(&reporter.reportCount) >= 1
	}, 100*time.Millisecond, 5*time.Millisecond)

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
	checkFn := func() (*healthplatformpayload.IssueReport, error) {
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
	assert.Eventually(t, func() bool {
		return atomic.LoadInt32(&callCount) >= 1
	}, 500*time.Millisecond, 10*time.Millisecond)

	// Stop component
	err = lifecycle.Stop(context.Background())
	require.NoError(t, err)
}

// TestCheckRunnerReportsIssues tests that issues are reported correctly using the component
func TestCheckRunnerReportsIssues(t *testing.T) {
	lifecycle := newMockLifecycle()
	reqs := testRequires(t, lifecycle)
	reqs.Config.SetWithoutSource("health_platform.enabled", true)

	provides, err := NewComponent(reqs)
	require.NoError(t, err)

	comp := provides.Comp.(*healthPlatformImpl)

	checkFn := func() (*healthplatformpayload.IssueReport, error) {
		return &healthplatformpayload.IssueReport{
			IssueId: "check-execution-failure", // Use real issue ID from registry
			Context: map[string]string{"checkName": "test-check", "error": "test error"},
		}, nil
	}

	err = comp.RegisterCheck("test-check", "Test Check", checkFn, 50*time.Millisecond)
	require.NoError(t, err)

	err = lifecycle.Start(context.Background())
	require.NoError(t, err)
	defer func() { _ = lifecycle.Stop(context.Background()) }()

	// Wait for issue to be reported
	assert.Eventually(t, func() bool {
		issue := comp.GetIssueForCheck("test-check")
		return issue != nil && issue.Id == "check-execution-failure"
	}, 500*time.Millisecond, 10*time.Millisecond)
}

// TestCheckRunnerClearsIssueWhenNil tests that nil report clears issues using the component
func TestCheckRunnerClearsIssueWhenNil(t *testing.T) {
	lifecycle := newMockLifecycle()
	reqs := testRequires(t, lifecycle)
	reqs.Config.SetWithoutSource("health_platform.enabled", true)

	provides, err := NewComponent(reqs)
	require.NoError(t, err)

	comp := provides.Comp.(*healthPlatformImpl)

	// First return an issue, then nil
	callCount := int32(0)
	checkFn := func() (*healthplatformpayload.IssueReport, error) {
		count := atomic.AddInt32(&callCount, 1)
		if count == 1 {
			return &healthplatformpayload.IssueReport{
				IssueId: "check-execution-failure", // Use real issue ID from registry
				Context: map[string]string{"checkName": "test-check", "error": "test error"},
			}, nil
		}
		return nil, nil // No issue - should clear
	}

	err = comp.RegisterCheck("test-check", "Test Check", checkFn, 50*time.Millisecond)
	require.NoError(t, err)

	err = lifecycle.Start(context.Background())
	require.NoError(t, err)
	defer func() { _ = lifecycle.Stop(context.Background()) }()

	// Wait for issue to be cleared (after second run)
	assert.Eventually(t, func() bool {
		return atomic.LoadInt32(&callCount) >= 2 && comp.GetIssueForCheck("test-check") == nil
	}, 500*time.Millisecond, 10*time.Millisecond)
}
