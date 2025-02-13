// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.
package agentmemprof

import (
	"os"
	"runtime"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/pkg/config/mock"
)

func TestRun(t *testing.T) {
	// Initialize mock configuration
	cfg := mock.New(t)

	// Set configuration value directly
	cfg.SetWithoutSource("memory_profile_threshold", 1024*1024) // 1 MB

	// Create a new check instance
	check := newCheck(cfg)

	// Mock memory usage to exceed threshold
	var memStats runtime.MemStats
	runtime.ReadMemStats(&memStats)
	memStats.HeapAlloc = 2 * 1024 * 1024 // 2 MB

	// Run the check
	err := check.Run()
	require.NoError(t, err)

	// Verify that the profile was captured
	assert.True(t, check.(*Check).profileCaptured)
}

func TestCaptureHeapProfile(t *testing.T) {
	// Create a temporary directory for profiles
	tempDir := t.TempDir()

	// Capture the heap profile
	err := captureHeapProfile(tempDir)
	require.NoError(t, err)

	// Verify that the profile file was created
	files, err := os.ReadDir(tempDir)
	require.NoError(t, err)
	assert.Len(t, files, 1)

	// Verify the file name format
	fileName := files[0].Name()
	assert.Regexp(t, `^heap-profile-\d+\.pprof$`, fileName)
}

func TestRunProfileAlreadyCaptured(t *testing.T) {
	// Initialize mock configuration
	cfg := mock.New(t)

	// Set configuration value directly
	cfg.SetWithoutSource("memory_profile_threshold", 1024*1024) // 1 MB

	// Create a new check instance
	check := newCheck(cfg).(*Check)
	check.profileCaptured = true

	// Run the check
	err := check.Run()
	require.NoError(t, err)

	// Verify that the profile was not captured again
	assert.True(t, check.profileCaptured)
}

func TestRunThresholdNotSet(t *testing.T) {
	// Initialize mock configuration
	cfg := mock.New(t)

	// Set configuration value directly
	cfg.SetWithoutSource("memory_profile_threshold", 0) // Threshold not set

	// Create a new check instance
	check := newCheck(cfg)

	// Run the check
	err := check.Run()
	require.NoError(t, err)

	// Verify that the profile was not captured
	assert.False(t, check.(*Check).profileCaptured)
}
