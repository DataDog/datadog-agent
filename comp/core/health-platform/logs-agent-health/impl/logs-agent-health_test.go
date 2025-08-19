// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

package logsagenthealthimpl

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	logsagenthealth "github.com/DataDog/datadog-agent/comp/core/health-platform/logs-agent-health/def"
)

// newTestComponent creates a simple test component with mock dependencies
func newTestComponent(t *testing.T) logsagenthealth.Component {
	// Create a simple component with nil dependencies for testing
	return &component{
		ctx:      context.Background(),
		done:     make(chan struct{}),
		interval: 30 * time.Second, // Default interval
	}
}

func TestCheckHealth(t *testing.T) {
	comp := newTestComponent(t)

	ctx := context.Background()

	// Test health check (this will depend on the actual system state)
	issues, err := comp.CheckHealth(ctx)
	require.NoError(t, err)

	// We can't predict the exact issues, but we can verify the structure
	for _, issue := range issues {
		assert.NotEmpty(t, issue.ID)
		assert.NotEmpty(t, issue.Name)
		assert.NotEmpty(t, issue.Severity)
		// Extra field can be empty for some issues
	}
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

func TestSeverityConstants(t *testing.T) {
	// Test that severity constants are properly defined
	assert.Equal(t, "low", string(logsagenthealth.SeverityLow))
	assert.Equal(t, "medium", string(logsagenthealth.SeverityMedium))
	assert.Equal(t, "high", string(logsagenthealth.SeverityHigh))
	assert.Equal(t, "critical", string(logsagenthealth.SeverityCritical))
}

func TestIssueStructure(t *testing.T) {
	// Test creating an issue with all fields
	issue := logsagenthealth.Issue{
		ID:       "test-issue",
		Name:     "Test Issue",
		Extra:    "Test extra information",
		Severity: logsagenthealth.SeverityMedium,
	}

	assert.Equal(t, "test-issue", issue.ID)
	assert.Equal(t, "Test Issue", issue.Name)
	assert.Equal(t, "Test extra information", issue.Extra)
	assert.Equal(t, logsagenthealth.SeverityMedium, issue.Severity)
}
