// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

package healthplatformimpl

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/fx"

	"github.com/DataDog/datadog-agent/comp/core"
	healthplatform "github.com/DataDog/datadog-agent/comp/core/health-platform/def"
	"github.com/DataDog/datadog-agent/comp/forwarder/defaultforwarder"
	"github.com/DataDog/datadog-agent/pkg/util/fxutil"
)

// newTestComponent creates a component using fxutil.Test with proper mock dependencies
func newTestComponent(t *testing.T) healthplatform.Component {
	return fxutil.Test[healthplatform.Component](t, fx.Options(
		core.MockBundle(),
		defaultforwarder.MockModule(),
		fxutil.ProvideComponentConstructor(NewComponent),
	))
}

func TestAddIssue(t *testing.T) {
	comp := newTestComponent(t)

	// Test adding valid issue
	issue := healthplatform.Issue{
		ID:    "test-issue-1",
		Name:  "Test Issue",
		Extra: "some extra info",
	}

	err := comp.AddIssue(issue)
	require.NoError(t, err)

	// Verify issue was added
	issues := comp.ListIssues()
	assert.Len(t, issues, 1)
	assert.Equal(t, issue, issues[0])

	// Test adding issue with empty ID
	invalidIssue := healthplatform.Issue{
		Name: "Invalid Issue",
	}
	err = comp.AddIssue(invalidIssue)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "issue ID cannot be empty")

	// Test adding issue with empty name
	invalidIssue2 := healthplatform.Issue{
		ID: "test-issue-2",
	}
	err = comp.AddIssue(invalidIssue2)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "issue name cannot be empty")
}

func TestRemoveIssue(t *testing.T) {
	comp := newTestComponent(t)

	// Add an issue first
	issue := healthplatform.Issue{
		ID:   "test-issue-1",
		Name: "Test Issue",
	}
	err := comp.AddIssue(issue)
	require.NoError(t, err)

	// Test removing existing issue
	err = comp.RemoveIssue("test-issue-1")
	require.NoError(t, err)

	// Verify issue was removed
	issues := comp.ListIssues()
	assert.Len(t, issues, 0)

	// Test removing non-existent issue
	err = comp.RemoveIssue("non-existent")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "not found")

	// Test removing with empty ID
	err = comp.RemoveIssue("")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "issue ID cannot be empty")
}

func TestListIssues(t *testing.T) {
	comp := newTestComponent(t)

	// Test empty list
	issues := comp.ListIssues()
	assert.Len(t, issues, 0)

	// Add multiple issues
	issue1 := healthplatform.Issue{ID: "1", Name: "Issue 1"}
	issue2 := healthplatform.Issue{ID: "2", Name: "Issue 2"}

	err := comp.AddIssue(issue1)
	require.NoError(t, err)
	err = comp.AddIssue(issue2)
	require.NoError(t, err)

	// Test list contains both issues
	issues = comp.ListIssues()
	assert.Len(t, issues, 2)

	// Verify we have both issues (order might vary)
	ids := make(map[string]bool)
	for _, issue := range issues {
		ids[issue.ID] = true
	}
	assert.True(t, ids["1"])
	assert.True(t, ids["2"])
}

func TestSubmitReportNoIssues(t *testing.T) {
	comp := newTestComponent(t)

	// Test submitting with no issues
	err := comp.SubmitReport(context.Background())
	require.NoError(t, err)
}

func TestStartStop(t *testing.T) {
	comp := newTestComponent(t)

	ctx := context.Background()

	// Test start
	err := comp.Start(ctx)
	require.NoError(t, err)

	// Test start when already running
	err = comp.Start(ctx)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "already running")

	// Test stop
	err = comp.Stop()
	require.NoError(t, err)

	// Test stop when not running
	err = comp.Stop()
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "not running")
}

func TestHealthReportPayload(t *testing.T) {
	issues := []healthplatform.Issue{
		{ID: "1", Name: "Issue 1", Extra: "extra1"},
		{ID: "2", Name: "Issue 2", Extra: "extra2"},
	}

	payload := &HealthReportPayload{
		Hostname:  "test-host",
		HostID:    "host-id-123",
		OrgID:     12345,
		Issues:    issues,
		Timestamp: 1234567890,
	}

	// Test DescribeItem
	description := payload.DescribeItem()
	assert.Contains(t, description, "HealthReport")
	assert.Contains(t, description, "test-host")
	assert.Contains(t, description, "2 issues")

	// Test SplitPayload
	splits, err := payload.SplitPayload(2)
	require.NoError(t, err)
	assert.Len(t, splits, 2)

	// Test SplitPayload with invalid times
	splits, err = payload.SplitPayload(0)
	assert.Error(t, err)
	assert.Nil(t, splits)

	// Test SplitPayload with single issue
	payload.Issues = []healthplatform.Issue{{ID: "1", Name: "Issue 1"}}
	splits, err = payload.SplitPayload(2)
	assert.Error(t, err)
	assert.Nil(t, splits)
}
