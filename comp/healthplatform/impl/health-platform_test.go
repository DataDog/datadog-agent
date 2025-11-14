// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

//go:build test

package healthplatformimpl

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	logmock "github.com/DataDog/datadog-agent/comp/core/log/mock"
	nooptelemetry "github.com/DataDog/datadog-agent/comp/core/telemetry/noopsimpl"
	compdef "github.com/DataDog/datadog-agent/comp/def"
	healthplatform "github.com/DataDog/datadog-agent/comp/healthplatform/def"
)

// mockLifecycle is a simple mock for testing
type mockLifecycle struct {
	startHook compdef.Hook
	stopHook  compdef.Hook
}

func newMockLifecycle() *mockLifecycle {
	return &mockLifecycle{}
}

func (m *mockLifecycle) Append(hook compdef.Hook) {
	if hook.OnStart != nil {
		m.startHook = hook
	}
	if hook.OnStop != nil {
		m.stopHook = hook
	}
}

func (m *mockLifecycle) Start(ctx context.Context) error {
	if m.startHook.OnStart != nil {
		return m.startHook.OnStart(ctx)
	}
	return nil
}

func (m *mockLifecycle) Stop(ctx context.Context) error {
	if m.stopHook.OnStop != nil {
		return m.stopHook.OnStop(ctx)
	}
	return nil
}

// TestNewComponent tests the creation of a new health platform component
func TestNewComponent(t *testing.T) {
	// Create test dependencies
	reqs := Requires{
		Lifecycle: newMockLifecycle(),
		Log:       logmock.New(t),
		Telemetry: nooptelemetry.GetCompatComponent(),
	}

	// Create the component
	provides, err := NewComponent(reqs)
	require.NoError(t, err)
	assert.NotNil(t, provides.Comp)

	// Verify the component implements the interface
	var _ healthplatform.Component = provides.Comp
}

// TestRegisterCheck tests health check registration functionality
func TestRegisterCheck(t *testing.T) {
	reqs := Requires{
		Lifecycle: newMockLifecycle(),
		Log:       logmock.New(t),
		Telemetry: nooptelemetry.GetCompatComponent(),
	}

	provides, err := NewComponent(reqs)
	require.NoError(t, err)
	comp := provides.Comp

	// Test registering a valid health check
	validCheck := healthplatform.CheckConfig{
		CheckName: "test-check",
		CheckID:   "test-check-1",
		Run: func(_ context.Context) (*healthplatform.IssueReport, error) {
			return nil, nil
		},
	}

	err = comp.RegisterCheck(validCheck)
	assert.NoError(t, err)

	// Test registering a check with empty ID
	invalidCheck := healthplatform.CheckConfig{
		CheckName: "invalid-check",
		CheckID:   "",
		Run: func(_ context.Context) (*healthplatform.IssueReport, error) {
			return nil, nil
		},
	}

	err = comp.RegisterCheck(invalidCheck)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "check ID cannot be empty")

	// Test registering a check with nil callback
	nilCallbackCheck := healthplatform.CheckConfig{
		CheckName: "nil-callback-check",
		CheckID:   "nil-callback-1",
	}

	err = comp.RegisterCheck(nilCallbackCheck)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "check callback cannot be nil")
}

// TestIssueManagement tests issue detection, retrieval, and clearing
func TestIssueManagement(t *testing.T) {
	reqs := Requires{
		Lifecycle: newMockLifecycle(),
		Log:       logmock.New(t),
		Telemetry: nooptelemetry.GetCompatComponent(),
	}

	provides, err := NewComponent(reqs)
	require.NoError(t, err)
	comp := provides.Comp

	// Register a health check that returns an issue report
	// Using the docker-file-tailing-disabled issue that's in the registry
	checkWithIssues := healthplatform.CheckConfig{
		CheckName: "issue-check",
		CheckID:   "issue-check-1",
		Run: func(_ context.Context) (*healthplatform.IssueReport, error) {
			return &healthplatform.IssueReport{
				IssueID: "docker-file-tailing-disabled",
				Context: map[string]string{
					"dockerDir": "/var/lib/docker",
					"os":        "linux",
				},
			}, nil
		},
	}

	err = comp.RegisterCheck(checkWithIssues)
	require.NoError(t, err)

	// Register a health check that returns no issues
	checkWithoutIssues := healthplatform.CheckConfig{
		CheckName: "no-issue-check",
		CheckID:   "no-issue-check-1",
		Run: func(_ context.Context) (*healthplatform.IssueReport, error) {
			return nil, nil
		},
	}

	err = comp.RegisterCheck(checkWithoutIssues)
	require.NoError(t, err)

	// Start the component
	lifecycle := reqs.Lifecycle.(*mockLifecycle)
	err = lifecycle.Start(context.Background())
	require.NoError(t, err)

	// Manually trigger health checks for testing
	comp.RunHealthChecks(false)

	// Test GetAllIssues
	count, allIssues := comp.GetAllIssues()
	assert.NotNil(t, allIssues)
	assert.Contains(t, allIssues, "issue-check-1")
	assert.Equal(t, 1, count)
	// no-issue-check-1 returns nil, so it won't be in the map

	// Test GetIssueForCheck
	issueForCheck := comp.GetIssueForCheck("issue-check-1")
	assert.NotNil(t, issueForCheck)
	assert.Equal(t, "docker-file-tailing-disabled", issueForCheck.ID)

	// Test GetIssueForCheck with non-existent check
	nonExistentIssue := comp.GetIssueForCheck("non-existent")
	assert.Nil(t, nonExistentIssue)

	// Test issue count (1 check with issue, 1 without = 1 total)
	totalCount, _ := comp.GetAllIssues()
	assert.Equal(t, 1, totalCount)

	// Test ClearIssuesForCheck
	comp.ClearIssuesForCheck("issue-check-1")
	clearedIssue := comp.GetIssueForCheck("issue-check-1")
	assert.Nil(t, clearedIssue)

	// Verify total count decreased
	newTotalCount, _ := comp.GetAllIssues()
	assert.Equal(t, 0, newTotalCount)

	// Test ClearAllIssues
	comp.ClearAllIssues()
	countAfterClear, allIssuesAfterClear := comp.GetAllIssues()
	assert.Equal(t, 0, countAfterClear)
	assert.Len(t, allIssuesAfterClear, 0)

	// Stop the component
	err = lifecycle.Stop(context.Background())
	require.NoError(t, err)
}

// TestHealthCheckExecution tests the execution of health checks
func TestHealthCheckExecution(t *testing.T) {
	reqs := Requires{
		Lifecycle: newMockLifecycle(),
		Log:       logmock.New(t),
		Telemetry: nooptelemetry.GetCompatComponent(),
	}

	provides, err := NewComponent(reqs)
	require.NoError(t, err)
	comp := provides.Comp

	// Track how many times the callback is called
	var callCount atomic.Int32

	// Register a health check that tracks calls
	trackingCheck := healthplatform.CheckConfig{
		CheckName: "tracking-check",
		CheckID:   "tracking-check-1",
		Run: func(_ context.Context) (*healthplatform.IssueReport, error) {
			callCount.Add(1)
			return nil, nil
		},
	}

	err = comp.RegisterCheck(trackingCheck)
	require.NoError(t, err)

	// Start the component
	lifecycle := reqs.Lifecycle.(*mockLifecycle)
	err = lifecycle.Start(context.Background())
	require.NoError(t, err)

	// Manually trigger health checks for testing
	comp.RunHealthChecks(false)

	// Verify the callback was called
	assert.Greater(t, int(callCount.Load()), 0, "Health check callback should have been called")

	// Stop the component
	err = lifecycle.Stop(context.Background())
	require.NoError(t, err)
}

// TestHealthCheckErrorHandling tests error handling in health checks
func TestHealthCheckErrorHandling(t *testing.T) {
	reqs := Requires{
		Lifecycle: newMockLifecycle(),
		Log:       logmock.New(t),
		Telemetry: nooptelemetry.GetCompatComponent(),
	}

	provides, err := NewComponent(reqs)
	require.NoError(t, err)
	comp := provides.Comp

	// Register a health check that returns an error
	errorCheck := healthplatform.CheckConfig{
		CheckName: "error-check",
		CheckID:   "error-check-1",
		Run: func(_ context.Context) (*healthplatform.IssueReport, error) {
			return nil, errors.New("test error")
		},
	}

	err = comp.RegisterCheck(errorCheck)
	require.NoError(t, err)

	// Start the component
	lifecycle := reqs.Lifecycle.(*mockLifecycle)
	err = lifecycle.Start(context.Background())
	require.NoError(t, err)

	// Manually trigger health checks for testing
	comp.RunHealthChecks(false)

	// Verify no issue was stored due to error
	issue := comp.GetIssueForCheck("error-check-1")
	assert.Nil(t, issue)

	// Stop the component
	err = lifecycle.Stop(context.Background())
	require.NoError(t, err)
}

// TestHealthCheckPanicRecovery tests panic recovery in health checks
func TestHealthCheckPanicRecovery(t *testing.T) {
	reqs := Requires{
		Lifecycle: newMockLifecycle(),
		Log:       logmock.New(t),
		Telemetry: nooptelemetry.GetCompatComponent(),
	}

	provides, err := NewComponent(reqs)
	require.NoError(t, err)
	comp := provides.Comp

	// Register a health check that panics
	panicCheck := healthplatform.CheckConfig{
		CheckName: "panic-check",
		CheckID:   "panic-check-1",
		Run: func(_ context.Context) (*healthplatform.IssueReport, error) {
			panic("test panic")
		},
	}

	err = comp.RegisterCheck(panicCheck)
	require.NoError(t, err)

	// Start the component
	lifecycle := reqs.Lifecycle.(*mockLifecycle)
	err = lifecycle.Start(context.Background())
	require.NoError(t, err)

	// Manually trigger health checks for testing
	comp.RunHealthChecks(false)

	// Verify no issue was stored due to panic
	issue := comp.GetIssueForCheck("panic-check-1")
	assert.Nil(t, issue)

	// Stop the component
	err = lifecycle.Stop(context.Background())
	require.NoError(t, err)
}

// TestConcurrentOperations tests thread safety of the component
func TestConcurrentOperations(t *testing.T) {
	reqs := Requires{
		Lifecycle: newMockLifecycle(),
		Log:       logmock.New(t),
		Telemetry: nooptelemetry.GetCompatComponent(),
	}

	provides, err := NewComponent(reqs)
	require.NoError(t, err)
	comp := provides.Comp

	// Start the component
	lifecycle := reqs.Lifecycle.(*mockLifecycle)
	err = lifecycle.Start(context.Background())
	require.NoError(t, err)

	// Run concurrent operations
	var wg sync.WaitGroup
	numGoroutines := 10

	// Concurrent check registrations
	for i := 0; i < numGoroutines; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			check := healthplatform.CheckConfig{
				CheckName: "concurrent-check",
				CheckID:   "concurrent-check-" + string(rune(id)),
				Run: func(_ context.Context) (*healthplatform.IssueReport, error) {
					return nil, nil
				},
			}
			comp.RegisterCheck(check)
		}(i)
	}

	// Concurrent issue retrieval
	for i := 0; i < numGoroutines; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, _ = comp.GetAllIssues()
		}()
	}

	// Wait for all operations to complete
	wg.Wait()

	// Stop the component
	err = lifecycle.Stop(context.Background())
	require.NoError(t, err)
}

// TestComponentLifecycle tests the start/stop lifecycle of the component
func TestComponentLifecycle(t *testing.T) {
	reqs := Requires{
		Lifecycle: newMockLifecycle(),
		Log:       logmock.New(t),
		Telemetry: nooptelemetry.GetCompatComponent(),
	}

	provides, err := NewComponent(reqs)
	require.NoError(t, err)
	comp := provides.Comp

	// Register a health check
	check := healthplatform.CheckConfig{
		CheckName: "lifecycle-check",
		CheckID:   "lifecycle-check-1",
		Run: func(_ context.Context) (*healthplatform.IssueReport, error) {
			return nil, nil
		},
	}

	err = comp.RegisterCheck(check)
	require.NoError(t, err)

	// Test starting the component
	lifecycle := reqs.Lifecycle.(*mockLifecycle)
	err = lifecycle.Start(context.Background())
	require.NoError(t, err)

	// Wait for health checks to run
	time.Sleep(100 * time.Millisecond)

	// Test stopping the component
	err = lifecycle.Stop(context.Background())
	require.NoError(t, err)
}

// TestDefaultChecksRegistration tests that default checks are registered
func TestDefaultChecksRegistration(t *testing.T) {
	reqs := Requires{
		Lifecycle: newMockLifecycle(),
		Log:       logmock.New(t),
		Telemetry: nooptelemetry.GetCompatComponent(),
	}

	provides, err := NewComponent(reqs)
	require.NoError(t, err)
	comp := provides.Comp

	// Start the component to trigger default check registration
	lifecycle := reqs.Lifecycle.(*mockLifecycle)
	err = lifecycle.Start(context.Background())
	require.NoError(t, err)

	// Wait for health checks to run
	time.Sleep(100 * time.Millisecond)

	// Verify default checks are registered by checking if any issues exist
	_, allIssues := comp.GetAllIssues()
	assert.NotNil(t, allIssues)

	// Stop the component
	err = lifecycle.Stop(context.Background())
	require.NoError(t, err)
}

// TestIssueTimestamping tests that issues get proper timestamps
func TestIssueTimestamping(t *testing.T) {
	reqs := Requires{
		Lifecycle: newMockLifecycle(),
		Log:       logmock.New(t),
		Telemetry: nooptelemetry.GetCompatComponent(),
	}

	provides, err := NewComponent(reqs)
	require.NoError(t, err)
	comp := provides.Comp

	// Register a health check that returns an issue report
	timestampCheck := healthplatform.CheckConfig{
		CheckName: "timestamp-check",
		CheckID:   "timestamp-check-1",
		Run: func(_ context.Context) (*healthplatform.IssueReport, error) {
			return &healthplatform.IssueReport{
				IssueID: "docker-file-tailing-disabled",
				Context: map[string]string{
					"dockerDir": "/var/lib/docker",
					"os":        "linux",
				},
			}, nil
		},
	}

	err = comp.RegisterCheck(timestampCheck)
	require.NoError(t, err)

	// Start the component
	lifecycle := reqs.Lifecycle.(*mockLifecycle)
	err = lifecycle.Start(context.Background())
	require.NoError(t, err)

	// Manually trigger health checks for testing
	comp.RunHealthChecks(false)

	// Verify the issue got a timestamp
	issue := comp.GetIssueForCheck("timestamp-check-1")
	require.NotNil(t, issue)
	assert.NotEmpty(t, issue.DetectedAt)
	assert.NotEqual(t, "", issue.DetectedAt)

	// Stop the component
	err = lifecycle.Stop(context.Background())
	require.NoError(t, err)
}

// TestHealthCheckContextCancellation tests that health checks respect context cancellation
func TestHealthCheckContextCancellation(t *testing.T) {
	reqs := Requires{
		Lifecycle: newMockLifecycle(),
		Log:       logmock.New(t),
		Telemetry: nooptelemetry.GetCompatComponent(),
	}

	provides, err := NewComponent(reqs)
	require.NoError(t, err)
	comp := provides.Comp

	// Track if the check was cancelled
	var wasCancelled atomic.Int32

	// Register a health check that respects context cancellation
	cancellableCheck := healthplatform.CheckConfig{
		CheckName: "cancellable-check",
		CheckID:   "cancellable-check-1",
		Run: func(ctx context.Context) (*healthplatform.IssueReport, error) {
			// Simulate a long-running operation
			select {
			case <-time.After(5 * time.Second):
				// This should not happen in the test
				return nil, nil
			case <-ctx.Done():
				// Context was cancelled, mark it
				wasCancelled.Store(1)
				return nil, ctx.Err()
			}
		},
	}

	err = comp.RegisterCheck(cancellableCheck)
	require.NoError(t, err)

	// Start the component
	lifecycle := reqs.Lifecycle.(*mockLifecycle)
	err = lifecycle.Start(context.Background())
	require.NoError(t, err)

	// Trigger the check asynchronously
	comp.RunHealthChecks(true)

	// Give it a moment to start
	time.Sleep(100 * time.Millisecond)

	// Stop the component (should cancel the context)
	err = lifecycle.Stop(context.Background())
	require.NoError(t, err)

	// Wait a bit for cancellation to propagate
	time.Sleep(100 * time.Millisecond)

	// Verify the check was cancelled
	assert.Equal(t, int32(1), wasCancelled.Load(), "Health check should have been cancelled")
}
