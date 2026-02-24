// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

//go:build test

package healthplatformimpl

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	healthplatformpayload "github.com/DataDog/agent-payload/v5/healthplatform"

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

// testRequires creates a Requires struct for testing with health platform enabled
func testRequires(t *testing.T, lifecycle *mockLifecycle) Requires {
	cfg := config.NewMock(t)
	cfg.SetWithoutSource("health_platform.enabled", true)
	// Use temp directory to avoid test interference
	cfg.SetWithoutSource("run_path", t.TempDir())

	if lifecycle == nil {
		lifecycle = newMockLifecycle()
	}

	hostnameMock, _ := hostnameinterface.NewMock("test-hostname")

	return Requires{
		Lifecycle: lifecycle,
		Config:    cfg,
		Log:       logmock.New(t),
		Telemetry: nooptelemetry.GetCompatComponent(),
		Hostname:  hostnameMock,
	}
}

// testRequiresWithRunPath creates a Requires struct with a custom run_path for persistence testing
func testRequiresWithRunPath(t *testing.T, lifecycle *mockLifecycle, runPath string) Requires {
	cfg := config.NewMock(t)
	cfg.SetWithoutSource("health_platform.enabled", true)
	cfg.SetWithoutSource("run_path", runPath)

	if lifecycle == nil {
		lifecycle = newMockLifecycle()
	}

	hostnameMock, _ := hostnameinterface.NewMock("test-hostname")

	return Requires{
		Lifecycle: lifecycle,
		Config:    cfg,
		Log:       logmock.New(t),
		Telemetry: nooptelemetry.GetCompatComponent(),
		Hostname:  hostnameMock,
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
		&healthplatformpayload.IssueReport{
			IssueId: "docker-file-tailing-disabled",
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
	assert.Equal(t, "docker-file-tailing-disabled", issueForCheck.Id)

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
		&healthplatformpayload.IssueReport{
			IssueId: "docker-file-tailing-disabled",
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
		&healthplatformpayload.IssueReport{
			IssueId: "docker-file-tailing-disabled",
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
		&healthplatformpayload.IssueReport{
			IssueId: "docker-file-tailing-disabled",
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
	err = comp.ReportIssue("", "Test", &healthplatformpayload.IssueReport{
		IssueId: "docker-file-tailing-disabled",
	})
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "check ID cannot be empty")

	// Test empty issue ID
	err = comp.ReportIssue("check-1", "Test", &healthplatformpayload.IssueReport{
		IssueId: "",
	})
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "issue ID cannot be empty")

	// Test unknown issue ID
	err = comp.ReportIssue("check-1", "Test", &healthplatformpayload.IssueReport{
		IssueId: "unknown-issue",
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
				&healthplatformpayload.IssueReport{
					IssueId: "docker-file-tailing-disabled",
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
		&healthplatformpayload.IssueReport{
			IssueId: "docker-file-tailing-disabled",
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
		&healthplatformpayload.IssueReport{
			IssueId: "docker-file-tailing-disabled",
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

	hostnameMock, _ := hostnameinterface.NewMock("test-hostname")

	reqs := Requires{
		Lifecycle: newMockLifecycle(),
		Config:    cfg,
		Log:       logmock.New(t),
		Telemetry: nooptelemetry.GetCompatComponent(),
		Hostname:  hostnameMock,
	}

	provides, err := NewComponent(reqs)
	require.NoError(t, err)
	require.NotNil(t, provides.Comp)

	// Verify it's the noop implementation
	_, ok := provides.Comp.(*noopHealthPlatform)
	assert.True(t, ok, "Expected noopHealthPlatform when disabled")

	// Verify all methods work but do nothing
	err = provides.Comp.ReportIssue("test-check", "Test Check", &healthplatformpayload.IssueReport{
		IssueId: "docker-file-tailing-disabled",
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

// TestGetIssuesHandlerEmpty tests the HTTP handler returns empty list when no issues
func TestGetIssuesHandlerEmpty(t *testing.T) {
	lifecycle := newMockLifecycle()
	reqs := testRequires(t, lifecycle)

	provides, err := NewComponent(reqs)
	require.NoError(t, err)

	// Get the implementation to access the handler
	impl, ok := provides.Comp.(*healthPlatformImpl)
	require.True(t, ok, "Expected healthPlatformImpl")

	// Create a test request
	req := httptest.NewRequest(http.MethodGet, "/health-platform/issues", nil)
	w := httptest.NewRecorder()

	// Call the handler
	impl.getIssuesHandler(w, req)

	// Check the response
	resp := w.Result()
	defer resp.Body.Close()

	assert.Equal(t, http.StatusOK, resp.StatusCode)
	assert.Equal(t, "application/json", resp.Header.Get("Content-Type"))

	// Parse the response
	var response struct {
		Count  int                                     `json:"count"`
		Issues map[string]*healthplatformpayload.Issue `json:"issues"`
	}
	err = json.NewDecoder(resp.Body).Decode(&response)
	require.NoError(t, err)

	assert.Equal(t, 0, response.Count)
	assert.Empty(t, response.Issues)
}

// TestGetIssuesHandlerWithIssues tests the HTTP handler returns issues correctly
func TestGetIssuesHandlerWithIssues(t *testing.T) {
	lifecycle := newMockLifecycle()
	reqs := testRequires(t, lifecycle)

	provides, err := NewComponent(reqs)
	require.NoError(t, err)

	// Start the component
	err = lifecycle.Start(context.Background())
	require.NoError(t, err)

	// Get the implementation to access the handler
	impl, ok := provides.Comp.(*healthPlatformImpl)
	require.True(t, ok, "Expected healthPlatformImpl")

	// Report some issues
	err = provides.Comp.ReportIssue(
		"check-1",
		"Check 1",
		&healthplatformpayload.IssueReport{
			IssueId: "docker-file-tailing-disabled",
			Context: map[string]string{
				"dockerDir": "/var/lib/docker",
				"os":        "linux",
			},
		},
	)
	require.NoError(t, err)

	err = provides.Comp.ReportIssue(
		"check-2",
		"Check 2",
		&healthplatformpayload.IssueReport{
			IssueId: "docker-file-tailing-disabled",
			Context: map[string]string{
				"dockerDir": "/var/lib/docker",
				"os":        "windows",
			},
		},
	)
	require.NoError(t, err)

	// Create a test request
	req := httptest.NewRequest(http.MethodGet, "/health-platform/issues", nil)
	w := httptest.NewRecorder()

	// Call the handler
	impl.getIssuesHandler(w, req)

	// Check the response
	resp := w.Result()
	defer resp.Body.Close()

	assert.Equal(t, http.StatusOK, resp.StatusCode)
	assert.Equal(t, "application/json", resp.Header.Get("Content-Type"))

	// Parse the response
	var response struct {
		Count  int                                     `json:"count"`
		Issues map[string]*healthplatformpayload.Issue `json:"issues"`
	}
	err = json.NewDecoder(resp.Body).Decode(&response)
	require.NoError(t, err)

	assert.Equal(t, 2, response.Count)
	assert.Len(t, response.Issues, 2)
	assert.Contains(t, response.Issues, "check-1")
	assert.Contains(t, response.Issues, "check-2")

	// Verify issue details
	issue1 := response.Issues["check-1"]
	assert.Equal(t, "docker-file-tailing-disabled", issue1.Id)
	assert.NotEmpty(t, issue1.Title)
	assert.NotEmpty(t, issue1.DetectedAt)

	// Stop the component
	err = lifecycle.Stop(context.Background())
	require.NoError(t, err)
}

// ============================================================================
// Persistence Tests
// ============================================================================

// TestPersistenceStateTransitions tests all state transitions in one test:
// - new -> ongoing -> resolved
// - resolved -> new (reoccurrence)
// - different issue ID -> new
func TestPersistenceStateTransitions(t *testing.T) {
	tmpDir := t.TempDir()
	lifecycle := newMockLifecycle()
	reqs := testRequiresWithRunPath(t, lifecycle, tmpDir)

	provides, err := NewComponent(reqs)
	require.NoError(t, err)

	impl, ok := provides.Comp.(*healthPlatformImpl)
	require.True(t, ok)

	err = lifecycle.Start(context.Background())
	require.NoError(t, err)

	// 1. Report a new issue -> state should be "new"
	err = provides.Comp.ReportIssue("check-1", "Check 1", &healthplatformpayload.IssueReport{
		IssueId: "docker-file-tailing-disabled",
		Context: map[string]string{"dockerDir": "/var/lib/docker", "os": "linux"},
	})
	require.NoError(t, err)

	persisted := impl.persistedIssues["check-1"]
	require.NotNil(t, persisted)
	assert.Equal(t, IssueStateNew, persisted.State)
	assert.Equal(t, "docker-file-tailing-disabled", persisted.IssueID)
	firstSeen := persisted.FirstSeen

	// 2. Report same issue again -> state should be "ongoing"
	err = provides.Comp.ReportIssue("check-1", "Check 1", &healthplatformpayload.IssueReport{
		IssueId: "docker-file-tailing-disabled",
		Context: map[string]string{"dockerDir": "/var/lib/docker", "os": "linux"},
	})
	require.NoError(t, err)

	persisted = impl.persistedIssues["check-1"]
	assert.Equal(t, IssueStateOngoing, persisted.State)
	assert.Equal(t, firstSeen, persisted.FirstSeen) // first_seen should not change

	// 3. Resolve the issue -> state should be "resolved"
	err = provides.Comp.ReportIssue("check-1", "Check 1", nil)
	require.NoError(t, err)

	persisted = impl.persistedIssues["check-1"]
	assert.Equal(t, IssueStateResolved, persisted.State)
	assert.NotEmpty(t, persisted.ResolvedAt)

	// 4. Issue reoccurs -> state should be "new" again (not ongoing)
	err = provides.Comp.ReportIssue("check-1", "Check 1", &healthplatformpayload.IssueReport{
		IssueId: "docker-file-tailing-disabled",
		Context: map[string]string{"dockerDir": "/var/lib/docker", "os": "linux"},
	})
	require.NoError(t, err)

	persisted = impl.persistedIssues["check-1"]
	assert.Equal(t, IssueStateNew, persisted.State)
	assert.Empty(t, persisted.ResolvedAt) // resolved_at should be cleared

	// 5. Different issue ID for same check -> state should be "new"
	err = provides.Comp.ReportIssue("check-1", "Check 1", &healthplatformpayload.IssueReport{
		IssueId: "docker-file-tailing-disabled",
		Context: map[string]string{"dockerDir": "/var/lib/docker", "os": "linux"},
	})
	require.NoError(t, err)
	assert.Equal(t, IssueStateOngoing, impl.persistedIssues["check-1"].State) // now ongoing

	err = provides.Comp.ReportIssue("check-1", "Check 1", &healthplatformpayload.IssueReport{
		IssueId: "check-execution-failure", // different issue ID
		Context: map[string]string{"dockerDir": "/var/lib/docker", "os": "linux"},
	})
	require.NoError(t, err)

	persisted = impl.persistedIssues["check-1"]
	assert.Equal(t, IssueStateNew, persisted.State)
	assert.Equal(t, "check-execution-failure", persisted.IssueID)

	// Verify file was created on disk
	persistencePath := filepath.Join(tmpDir, "health-platform", "issues.json")
	assert.FileExists(t, persistencePath)

	err = lifecycle.Stop(context.Background())
	require.NoError(t, err)
}

// TestPersistenceAcrossRestart simulates the full restart scenario
func TestPersistenceAcrossRestart(t *testing.T) {
	tmpDir := t.TempDir()

	// === First run ===
	lifecycle1 := newMockLifecycle()
	reqs1 := testRequiresWithRunPath(t, lifecycle1, tmpDir)

	provides1, err := NewComponent(reqs1)
	require.NoError(t, err)

	err = lifecycle1.Start(context.Background())
	require.NoError(t, err)

	// Report issue 1 and issue 2
	err = provides1.Comp.ReportIssue("check-1", "Check 1", &healthplatformpayload.IssueReport{
		IssueId: "docker-file-tailing-disabled",
		Context: map[string]string{"dockerDir": "/var/lib/docker", "os": "linux"},
	})
	require.NoError(t, err)

	err = provides1.Comp.ReportIssue("check-2", "Check 2", &healthplatformpayload.IssueReport{
		IssueId: "check-execution-failure",
		Context: map[string]string{"dockerDir": "/var/lib/docker", "os": "linux"},
	})
	require.NoError(t, err)

	// Report issue 3
	err = provides1.Comp.ReportIssue("check-3", "Check 3", &healthplatformpayload.IssueReport{
		IssueId: "docker-file-tailing-disabled",
		Context: map[string]string{"dockerDir": "/var/lib/docker", "os": "linux"},
	})
	require.NoError(t, err)

	// Resolve issue 2
	err = provides1.Comp.ReportIssue("check-2", "Check 2", nil)
	require.NoError(t, err)

	// Stop first instance
	err = lifecycle1.Stop(context.Background())
	require.NoError(t, err)

	// === Second run (simulate restart) ===
	lifecycle2 := newMockLifecycle()
	reqs2 := testRequiresWithRunPath(t, lifecycle2, tmpDir)

	provides2, err := NewComponent(reqs2)
	require.NoError(t, err)

	impl2, ok := provides2.Comp.(*healthPlatformImpl)
	require.True(t, ok)

	err = lifecycle2.Start(context.Background())
	require.NoError(t, err)

	// Verify issues were loaded (check-1 and check-3 should be active, check-2 resolved)
	count, _ := provides2.Comp.GetAllIssues()
	assert.Equal(t, 2, count)

	// Simulate: issue 1 is now resolved (user fixed their environment)
	err = provides2.Comp.ReportIssue("check-1", "Check 1", nil)
	require.NoError(t, err)

	// Simulate: issue 3 is still present
	err = provides2.Comp.ReportIssue("check-3", "Check 3", &healthplatformpayload.IssueReport{
		IssueId: "docker-file-tailing-disabled",
		Context: map[string]string{"dockerDir": "/var/lib/docker", "os": "linux"},
	})
	require.NoError(t, err)

	// Verify final state: issue 1 and 2 resolved, issue 3 ongoing
	assert.Equal(t, IssueStateResolved, impl2.persistedIssues["check-1"].State)
	assert.Equal(t, IssueStateResolved, impl2.persistedIssues["check-2"].State)
	assert.Equal(t, IssueStateOngoing, impl2.persistedIssues["check-3"].State)

	// Verify only issue 3 is in active issues
	count, issues := provides2.Comp.GetAllIssues()
	assert.Equal(t, 1, count)
	assert.Contains(t, issues, "check-3")

	// Stop second instance
	err = lifecycle2.Stop(context.Background())
	require.NoError(t, err)
}
