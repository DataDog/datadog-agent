// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

//go:build integration

package discovery

import (
	"errors"
	"fmt"
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/comp/core/autodiscovery/integration"
	"github.com/DataDog/datadog-agent/pkg/aggregator/mocksender"
)

func TestDiscoveryCheckIntegrationWithProcessLogProvider(t *testing.T) {
	// Clear any existing warnings
	ClearAllWarnings()

	// Create a non-existent file to trigger a permission warning
	nonExistentFile := "/non/existent/path/test.log"

	// Simulate what the process_log provider does when it encounters an unreadable file
	// This would normally happen in the isFileReadable function
	AddWarning(
		nonExistentFile,
		fmt.Errorf("open %s: %w", nonExistentFile, os.ErrPermission),
		"Discovered log file /non/existent/path/test.log could not be opened due to lack of permissions",
	)

	// Now test that the discovery check picks up this warning
	check := newCheck()
	senderManager := mocksender.CreateDefaultDemultiplexer()

	err := check.Configure(senderManager, 0, integration.Data{}, integration.Data{}, "test")
	require.NoError(t, err)

	// Run the check
	err = check.Run()
	require.NoError(t, err)

	// Verify the check captured the warning
	warnings := check.GetWarnings()
	require.Len(t, warnings, 1)
	assert.Contains(t, warnings[0].Error(), "could not be opened due to lack of permissions")

	// Verify it's a structured Warning
	plWarning, ok := warnings[0].(*Warning)
	require.True(t, ok)
	assert.Equal(t, nonExistentFile, plWarning.Resource)
	assert.Equal(t, ErrorCodePermissionDenied, plWarning.ErrorCode)
	assert.Contains(t, plWarning.ErrorString, nonExistentFile)
	assert.Contains(t, plWarning.ErrorString, "permission denied")

	// Verify that subsequent calls return the same warnings (they persist)
	warnings2 := check.GetWarnings()
	require.Len(t, warnings2, 1)
	assert.Contains(t, warnings2[0].Error(), "could not be opened due to lack of permissions")

	// Explicitly remove the warning
	RemoveWarning(nonExistentFile)

	// Run check again - warning should be gone
	err = check.Run()
	require.NoError(t, err)

	warnings3 := check.GetWarnings()
	require.Len(t, warnings3, 0)
}

func TestDiscoveryCheckWithMultipleWarnings(t *testing.T) {
	// Clear any existing warnings
	ClearAllWarnings()

	// Add multiple warnings using the direct interface
	AddWarning("file1", os.ErrPermission, "Permission denied for file1")
	AddWarning("file2", os.ErrNotExist, "File not found: file2")
	AddWarning("file3", errors.New("binary file detected"), "Binary file detected: file3")

	// Test discovery check
	check := newCheck()
	senderManager := mocksender.CreateDefaultDemultiplexer()

	err := check.Configure(senderManager, 0, integration.Data{}, integration.Data{}, "test")
	require.NoError(t, err)

	err = check.Run()
	require.NoError(t, err)

	// Verify all warnings are captured
	warnings := check.GetWarnings()
	require.Len(t, warnings, 3)

	// Verify warning content and structure
	warningMap := make(map[string]*Warning)
	for _, warning := range warnings {
		plWarning, ok := warning.(*Warning)
		require.True(t, ok)
		warningMap[plWarning.Resource] = plWarning
	}

	assert.Equal(t, ErrorCodePermissionDenied, warningMap["file1"].ErrorCode)
	assert.Equal(t, ErrorCodeFileNotFound, warningMap["file2"].ErrorCode)
	assert.Equal(t, ErrorCodeGeneric, warningMap["file3"].ErrorCode)
	assert.Equal(t, os.ErrPermission.Error(), warningMap["file1"].ErrorString)
	assert.Equal(t, os.ErrNotExist.Error(), warningMap["file2"].ErrorString)
	assert.Equal(t, "binary file detected", warningMap["file3"].ErrorString)
	assert.Equal(t, "Permission denied for file1", warningMap["file1"].Message)
	assert.Equal(t, "File not found: file2", warningMap["file2"].Message)
	assert.Equal(t, "Binary file detected: file3", warningMap["file3"].Message)
}

func TestDirectWarningInterface(t *testing.T) {
	// Clear any existing warnings
	ClearAllWarnings()

	// Test the direct interface between process_log provider and discovery check
	AddWarning(
		"testfile.log",
		os.ErrPermission,
		"Test warning from process_log provider",
	)

	// Verify warnings are captured by the discovery check
	check := newCheck()
	senderManager := mocksender.CreateDefaultDemultiplexer()

	err := check.Configure(senderManager, 0, integration.Data{}, integration.Data{}, "test")
	require.NoError(t, err)

	err = check.Run()
	require.NoError(t, err)

	warnings := check.GetWarnings()
	require.Len(t, warnings, 1)
	assert.Equal(t, "Test warning from process_log provider", warnings[0].Error())
}
