// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

package healthplatformimpl

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/comp/core/config"
	"github.com/DataDog/datadog-agent/comp/core/hostname/hostnameinterface"
	logmock "github.com/DataDog/datadog-agent/comp/core/log/mock"
	nooptelemetry "github.com/DataDog/datadog-agent/comp/core/telemetry/noopsimpl"
	compdef "github.com/DataDog/datadog-agent/comp/def"
	healthplatform "github.com/DataDog/datadog-agent/comp/healthplatform/def"
)

// mockLifecycle is a minimal implementation of lifecycle for testing
type mockLifecycle struct {
	hooks []compdef.Hook
}

func newMockLifecycle() *mockLifecycle {
	return &mockLifecycle{
		hooks: make([]compdef.Hook, 0),
	}
}

func (m *mockLifecycle) Append(hook compdef.Hook) {
	m.hooks = append(m.hooks, hook)
}

func (m *mockLifecycle) Start(ctx context.Context) error {
	for _, hook := range m.hooks {
		if hook.OnStart != nil {
			if err := hook.OnStart(ctx); err != nil {
				return err
			}
		}
	}
	return nil
}

func (m *mockLifecycle) Stop(ctx context.Context) error {
	for _, hook := range m.hooks {
		if hook.OnStop != nil {
			if err := hook.OnStop(ctx); err != nil {
				return err
			}
		}
	}
	return nil
}

// mockHostname is a simple mock for hostname component
type mockHostname struct {
	hostname string
}

func (m *mockHostname) Get(_ context.Context) (string, error) {
	return m.hostname, nil
}

func (m *mockHostname) GetWithProvider(_ context.Context) (hostnameinterface.Data, error) {
	return hostnameinterface.Data{Hostname: m.hostname, Provider: "mock"}, nil
}

func (m *mockHostname) GetSafe(_ context.Context) string {
	return m.hostname
}

// testRequires creates a Requires struct for testing with health platform enabled
func testRequires(t *testing.T, lifecycle *mockLifecycle) Requires {
	cfg := config.NewMock(t)
	cfg.SetWithoutSource("health_platform.enabled", true)

	if lifecycle == nil {
		lifecycle = newMockLifecycle()
	}

	return Requires{
		Lifecycle: lifecycle,
		Config:    cfg,
		Log:       logmock.New(t),
		Telemetry: nooptelemetry.GetCompatComponent(),
		Hostname:  &mockHostname{hostname: "test-hostname"},
	}
}

// TestNewComponent tests component initialization
func TestNewComponent(t *testing.T) {
	reqs := testRequires(t, nil)

	provides, err := NewComponent(reqs)
	require.NoError(t, err)
	require.NotNil(t, provides.Comp)

	// Test that the component implements the interface
	var _ healthplatform.Component = provides.Comp
}

// TestReportIssue tests direct issue reporting functionality
func TestReportIssue(t *testing.T) {
	lifecycle := newMockLifecycle()
	reqs := testRequires(t, lifecycle)

	provides, err := NewComponent(reqs)
	require.NoError(t, err)
	comp := provides.Comp

	// Start the component
	err = lifecycle.Start(context.Background())
	require.NoError(t, err)

	// Report an issue
	err = comp.ReportIssue(
		"logs-docker-file-permissions",
		"Docker File Tailing Permissions",
		&healthplatform.IssueReport{
			IssueID: "docker-file-tailing-disabled",
			Context: map[string]string{
				"dockerDir": "/var/lib/docker",
				"os":        "linux",
			},
		},
	)
	require.NoError(t, err)

	// Test GetAllIssues
	count, allIssues := comp.GetAllIssues()
	assert.Equal(t, 1, count)
	assert.NotNil(t, allIssues)
	assert.Contains(t, allIssues, "logs-docker-file-permissions")

	// Test GetIssueForCheck
	issueForCheck := comp.GetIssueForCheck("logs-docker-file-permissions")
	assert.NotNil(t, issueForCheck)
	assert.Equal(t, "docker-file-tailing-disabled", issueForCheck.ID)

	// Test GetIssueForCheck with non-existent check
	nonExistentIssue := comp.GetIssueForCheck("non-existent")
	assert.Nil(t, nonExistentIssue)

	// Stop the component
	err = lifecycle.Stop(context.Background())
	require.NoError(t, err)
}

// TestIssueResolution tests issue resolution (reporting nil)
func TestIssueResolution(t *testing.T) {
	lifecycle := newMockLifecycle()
	reqs := testRequires(t, lifecycle)

	provides, err := NewComponent(reqs)
	require.NoError(t, err)
	comp := provides.Comp

	// Start the component
	err = lifecycle.Start(context.Background())
	require.NoError(t, err)

	// Report an issue
	err = comp.ReportIssue(
		"test-check-1",
		"Test Check",
		&healthplatform.IssueReport{
			IssueID: "docker-file-tailing-disabled",
			Context: map[string]string{
				"dockerDir": "/var/lib/docker",
				"os":        "linux",
			},
		},
	)
	require.NoError(t, err)

	// Verify issue exists
	count, _ := comp.GetAllIssues()
	assert.Equal(t, 1, count)

	// Report resolution (nil report)
	err = comp.ReportIssue("test-check-1", "Test Check", nil)
	require.NoError(t, err)

	// Verify issue was removed
	newCount, _ := comp.GetAllIssues()
	assert.Equal(t, 0, newCount)

	clearedIssue := comp.GetIssueForCheck("test-check-1")
	assert.Nil(t, clearedIssue)

	// Stop the component
	err = lifecycle.Stop(context.Background())
	require.NoError(t, err)
}

// TestClearMethods tests clearing functionality
func TestClearMethods(t *testing.T) {
	lifecycle := newMockLifecycle()
	reqs := testRequires(t, lifecycle)

	provides, err := NewComponent(reqs)
	require.NoError(t, err)
	comp := provides.Comp

	// Report a couple of issues
	err = comp.ReportIssue(
		"check-1",
		"Check 1",
		&healthplatform.IssueReport{
			IssueID: "docker-file-tailing-disabled",
			Context: map[string]string{
				"dockerDir": "/var/lib/docker",
				"os":        "linux",
			},
		},
	)
	require.NoError(t, err)

	err = comp.ReportIssue(
		"check-2",
		"Check 2",
		&healthplatform.IssueReport{
			IssueID: "docker-file-tailing-disabled",
			Context: map[string]string{
				"dockerDir": "/var/lib/docker",
				"os":        "linux",
			},
		},
	)
	require.NoError(t, err)

	// Verify both issues exist
	count, _ := comp.GetAllIssues()
	assert.Equal(t, 2, count)

	// Test ClearIssuesForCheck
	comp.ClearIssuesForCheck("check-1")
	count, _ = comp.GetAllIssues()
	assert.Equal(t, 1, count)

	// Test ClearAllIssues
	comp.ClearAllIssues()
	countAfterClear, allIssuesAfterClear := comp.GetAllIssues()
	assert.Equal(t, 0, countAfterClear)
	assert.Len(t, allIssuesAfterClear, 0)
}

// TestReportIssueErrors tests error handling
func TestReportIssueErrors(t *testing.T) {
	reqs := testRequires(t, nil)

	provides, err := NewComponent(reqs)
	require.NoError(t, err)
	comp := provides.Comp

	// Test empty check ID
	err = comp.ReportIssue("", "Test", &healthplatform.IssueReport{
		IssueID: "docker-file-tailing-disabled",
	})
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "check ID cannot be empty")

	// Test empty issue ID
	err = comp.ReportIssue("check-1", "Test", &healthplatform.IssueReport{
		IssueID: "",
	})
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "issue ID cannot be empty")

	// Test unknown issue ID
	err = comp.ReportIssue("check-1", "Test", &healthplatform.IssueReport{
		IssueID: "unknown-issue",
	})
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "failed to build issue")
}

// TestConcurrentReporting tests concurrent issue reporting
func TestConcurrentReporting(t *testing.T) {
	reqs := testRequires(t, nil)

	provides, err := NewComponent(reqs)
	require.NoError(t, err)
	comp := provides.Comp

	numGoroutines := 100
	var wg sync.WaitGroup

	// Concurrent issue reporting
	for i := 0; i < numGoroutines; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			checkID := "concurrent-check-" + string(rune(id))
			_ = comp.ReportIssue(
				checkID,
				"Concurrent Check",
				&healthplatform.IssueReport{
					IssueID: "docker-file-tailing-disabled",
					Context: map[string]string{
						"dockerDir": "/var/lib/docker",
						"os":        "linux",
					},
				},
			)
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
}

// TestLifecycle tests component lifecycle
func TestLifecycle(t *testing.T) {
	lifecycle := newMockLifecycle()
	reqs := testRequires(t, lifecycle)

	provides, err := NewComponent(reqs)
	require.NoError(t, err)
	comp := provides.Comp

	// Test starting the component
	err = lifecycle.Start(context.Background())
	require.NoError(t, err)

	// Report an issue
	err = comp.ReportIssue(
		"lifecycle-check-1",
		"Lifecycle Check",
		&healthplatform.IssueReport{
			IssueID: "docker-file-tailing-disabled",
			Context: map[string]string{
				"dockerDir": "/var/lib/docker",
				"os":        "linux",
			},
		},
	)
	require.NoError(t, err)

	// Test stopping the component
	err = lifecycle.Stop(context.Background())
	require.NoError(t, err)
}

// TestIssueTimestamp tests that issues get timestamps
func TestIssueTimestamp(t *testing.T) {
	lifecycle := newMockLifecycle()
	reqs := testRequires(t, lifecycle)

	provides, err := NewComponent(reqs)
	require.NoError(t, err)
	comp := provides.Comp

	// Start the component
	err = lifecycle.Start(context.Background())
	require.NoError(t, err)

	// Report an issue
	err = comp.ReportIssue(
		"timestamp-check-1",
		"Timestamp Check",
		&healthplatform.IssueReport{
			IssueID: "docker-file-tailing-disabled",
			Context: map[string]string{
				"dockerDir": "/var/lib/docker",
				"os":        "linux",
			},
		},
	)
	require.NoError(t, err)

	// Verify the issue got a timestamp
	issue := comp.GetIssueForCheck("timestamp-check-1")
	require.NotNil(t, issue)
	assert.NotEmpty(t, issue.DetectedAt)

	// Parse the timestamp to verify it's valid RFC3339
	_, err = time.Parse(time.RFC3339, issue.DetectedAt)
	assert.NoError(t, err)

	// Stop the component
	err = lifecycle.Stop(context.Background())
	require.NoError(t, err)
}

// TestComponentDisabled tests that component is disabled when config flag is false
func TestComponentDisabled(t *testing.T) {
	cfg := config.NewMock(t)
	cfg.SetWithoutSource("health_platform.enabled", false)

	reqs := Requires{
		Lifecycle: newMockLifecycle(),
		Config:    cfg,
		Log:       logmock.New(t),
		Telemetry: nooptelemetry.GetCompatComponent(),
	}

	provides, err := NewComponent(reqs)
	require.NoError(t, err)
	require.NotNil(t, provides.Comp)

	// Verify it's the noop implementation
	_, ok := provides.Comp.(*noopHealthPlatform)
	assert.True(t, ok, "Expected noopHealthPlatform when disabled")

	// Verify all methods work but do nothing
	err = provides.Comp.ReportIssue("test-check", "Test Check", &healthplatform.IssueReport{
		IssueID: "docker-file-tailing-disabled",
		Context: map[string]string{"dockerDir": "/var/lib/docker"},
	})
	assert.NoError(t, err)

	// Verify no issues are tracked
	count, issues := provides.Comp.GetAllIssues()
	assert.Equal(t, 0, count)
	assert.Empty(t, issues)

	// Verify GetIssueForCheck returns nil
	issue := provides.Comp.GetIssueForCheck("test-check")
	assert.Nil(t, issue)

	// Verify clear methods work without error
	provides.Comp.ClearIssuesForCheck("test-check")
	provides.Comp.ClearAllIssues()
}
