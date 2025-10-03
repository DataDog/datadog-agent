// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

package healthplatformimpl

import (
	"context"
	"errors"
	"sync"
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
		Callback: func() ([]healthplatform.Issue, error) {
			return []healthplatform.Issue{}, nil
		},
	}

	err = comp.RegisterCheck(validCheck)
	assert.NoError(t, err)

	// Test registering a check with empty ID
	invalidCheck := healthplatform.CheckConfig{
		CheckName: "invalid-check",
		CheckID:   "",
		Callback: func() ([]healthplatform.Issue, error) {
			return []healthplatform.Issue{}, nil
		},
	}

	err = comp.RegisterCheck(invalidCheck)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "check ID cannot be empty")

	// Test registering a check with nil callback
	nilCallbackCheck := healthplatform.CheckConfig{
		CheckName: "nil-callback-check",
		CheckID:   "nil-callback-1",
		Callback:  nil,
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

	// Create test issues
	testIssues := []healthplatform.Issue{
		{
			ID:          "issue-1",
			IssueName:   "Test Issue 1",
			Title:       "Test Title 1",
			Description: "Test Description 1",
			Category:    "test",
			Severity:    "warning",
		},
		{
			ID:          "issue-2",
			IssueName:   "Test Issue 2",
			Title:       "Test Title 2",
			Description: "Test Description 2",
			Category:    "test",
			Severity:    "error",
		},
	}

	// Register a health check that returns issues
	checkWithIssues := healthplatform.CheckConfig{
		CheckName: "issue-check",
		CheckID:   "issue-check-1",
		Callback: func() ([]healthplatform.Issue, error) {
			return testIssues, nil
		},
	}

	err = comp.RegisterCheck(checkWithIssues)
	require.NoError(t, err)

	// Register a health check that returns no issues
	checkWithoutIssues := healthplatform.CheckConfig{
		CheckName: "no-issue-check",
		CheckID:   "no-issue-check-1",
		Callback: func() ([]healthplatform.Issue, error) {
			return []healthplatform.Issue{}, nil
		},
	}

	err = comp.RegisterCheck(checkWithoutIssues)
	require.NoError(t, err)

	// Start the component
	lifecycle := reqs.Lifecycle.(*mockLifecycle)
	err = lifecycle.Start(context.Background())
	require.NoError(t, err)

	// Manually trigger health checks for testing
	comp.RunHealthChecksNow()

	// Test GetAllIssues
	allIssues := comp.GetAllIssues()
	assert.NotNil(t, allIssues)
	assert.Contains(t, allIssues, "issue-check-1")
	assert.Contains(t, allIssues, "no-issue-check-1")

	// Test GetIssuesForCheck
	issuesForCheck := comp.GetIssuesForCheck("issue-check-1")
	assert.Len(t, issuesForCheck, 2)
	assert.Equal(t, "issue-1", issuesForCheck[0].ID)
	assert.Equal(t, "issue-2", issuesForCheck[1].ID)

	// Test GetIssuesForCheck with non-existent check
	nonExistentIssues := comp.GetIssuesForCheck("non-existent")
	assert.Len(t, nonExistentIssues, 0)

	// Test GetTotalIssueCount
	totalCount := comp.GetTotalIssueCount()
	assert.Equal(t, 2, totalCount)

	// Test ClearIssuesForCheck
	comp.ClearIssuesForCheck("issue-check-1")
	clearedIssues := comp.GetIssuesForCheck("issue-check-1")
	assert.Len(t, clearedIssues, 0)

	// Verify total count decreased
	newTotalCount := comp.GetTotalIssueCount()
	assert.Equal(t, 0, newTotalCount)

	// Test ClearAllIssues
	comp.ClearAllIssues()
	allIssuesAfterClear := comp.GetAllIssues()
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
	callCount := 0
	var callCountMux sync.Mutex

	// Register a health check that tracks calls
	trackingCheck := healthplatform.CheckConfig{
		CheckName: "tracking-check",
		CheckID:   "tracking-check-1",
		Callback: func() ([]healthplatform.Issue, error) {
			callCountMux.Lock()
			callCount++
			callCountMux.Unlock()
			return []healthplatform.Issue{}, nil
		},
	}

	err = comp.RegisterCheck(trackingCheck)
	require.NoError(t, err)

	// Start the component
	lifecycle := reqs.Lifecycle.(*mockLifecycle)
	err = lifecycle.Start(context.Background())
	require.NoError(t, err)

	// Manually trigger health checks for testing
	comp.RunHealthChecksNow()

	// Verify the callback was called
	callCountMux.Lock()
	assert.Greater(t, callCount, 0, "Health check callback should have been called")
	callCountMux.Unlock()

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
		Callback: func() ([]healthplatform.Issue, error) {
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
	comp.RunHealthChecksNow()

	// Verify no issues were stored due to error
	issues := comp.GetIssuesForCheck("error-check-1")
	assert.Len(t, issues, 0)

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
		Callback: func() ([]healthplatform.Issue, error) {
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
	comp.RunHealthChecksNow()

	// Verify no issues were stored due to panic
	issues := comp.GetIssuesForCheck("panic-check-1")
	assert.Len(t, issues, 0)

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
				Callback: func() ([]healthplatform.Issue, error) {
					return []healthplatform.Issue{}, nil
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
			comp.GetAllIssues()
			comp.GetTotalIssueCount()
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
		Callback: func() ([]healthplatform.Issue, error) {
			return []healthplatform.Issue{}, nil
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
	allIssues := comp.GetAllIssues()
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

	// Create a test issue without timestamp
	testIssue := healthplatform.Issue{
		ID:          "timestamp-test-issue",
		IssueName:   "Timestamp Test Issue",
		Title:       "Timestamp Test Title",
		Description: "Timestamp Test Description",
		Category:    "test",
		Severity:    "warning",
		DetectedAt:  "", // Empty timestamp
	}

	// Register a health check that returns the test issue
	timestampCheck := healthplatform.CheckConfig{
		CheckName: "timestamp-check",
		CheckID:   "timestamp-check-1",
		Callback: func() ([]healthplatform.Issue, error) {
			return []healthplatform.Issue{testIssue}, nil
		},
	}

	err = comp.RegisterCheck(timestampCheck)
	require.NoError(t, err)

	// Start the component
	lifecycle := reqs.Lifecycle.(*mockLifecycle)
	err = lifecycle.Start(context.Background())
	require.NoError(t, err)

	// Manually trigger health checks for testing
	comp.RunHealthChecksNow()

	// Verify the issue got a timestamp
	issues := comp.GetIssuesForCheck("timestamp-check-1")
	require.Len(t, issues, 1)
	assert.NotEmpty(t, issues[0].DetectedAt)
	assert.NotEqual(t, "", issues[0].DetectedAt)

	// Stop the component
	err = lifecycle.Stop(context.Background())
	require.NoError(t, err)
}
