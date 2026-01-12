// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

package discovery

import (
	"encoding/json"
	"fmt"
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/comp/core/autodiscovery/integration"
	"github.com/DataDog/datadog-agent/pkg/aggregator/mocksender"
)

func TestDiscoveryCheckRun(t *testing.T) {
	// Clear any existing warnings
	clearAllWarnings()

	// Add a warning directly to the global collector
	AddWarning(
		"test-file.log",
		os.ErrPermission,
		"Test warning message",
	)

	check := newCheck()
	senderManager := mocksender.CreateDefaultDemultiplexer()

	err := check.Configure(senderManager, 0, integration.Data{}, integration.Data{}, "test")
	require.NoError(t, err)

	err = check.Run()
	require.NoError(t, err)

	warnings := check.GetWarnings()
	require.Len(t, warnings, 1)
	assert.Contains(t, warnings[0].Error(), "Test warning message")
}

func TestDiscoveryCheckWarningsPersist(t *testing.T) {
	// Clear any existing warnings
	clearAllWarnings()

	// Add a warning directly to the global collector
	AddWarning(
		"test-file.log",
		os.ErrPermission,
		"Test warning message",
	)

	check := newCheck()
	senderManager := mocksender.CreateDefaultDemultiplexer()

	err := check.Configure(senderManager, 0, integration.Data{}, integration.Data{}, "test")
	require.NoError(t, err)

	err = check.Run()
	require.NoError(t, err)

	warnings := check.GetWarnings()
	require.Len(t, warnings, 1)

	// Run check again - warnings should persist
	err = check.Run()
	require.NoError(t, err)

	warnings2 := check.GetWarnings()
	require.Len(t, warnings2, 1)
	assert.Contains(t, warnings2[0].Error(), "Test warning message")

	// Explicitly remove the warning
	RemoveWarning("test-file.log")

	// Now warnings should be gone
	err = check.Run()
	require.NoError(t, err)

	warnings3 := check.GetWarnings()
	require.Len(t, warnings3, 0)
}

func TestProcessLogWarningStructure(t *testing.T) {
	// Clear any existing warnings
	clearAllWarnings()

	// Test structured warning
	AddWarning(
		"/var/log/app.log",
		fmt.Errorf("open /var/log/app.log: %w", os.ErrPermission),
		"Cannot access log file /var/log/app.log due to permissions",
	)

	check := newCheck()
	senderManager := mocksender.CreateDefaultDemultiplexer()

	err := check.Configure(senderManager, 0, integration.Data{}, integration.Data{}, "test")
	require.NoError(t, err)

	err = check.Run()
	require.NoError(t, err)

	warnings := check.GetWarnings()
	require.Len(t, warnings, 1)

	// Should be a Warning with structured data
	plWarning, ok := warnings[0].(*Warning)
	require.True(t, ok)
	assert.Equal(t, warningTypeLogFile, plWarning.Type)
	assert.Equal(t, 1, plWarning.Version)
	assert.Equal(t, "/var/log/app.log", plWarning.Resource)
	assert.Equal(t, errorCodePermissionDenied, plWarning.ErrorCode)
	assert.Equal(t, "open /var/log/app.log: permission denied", plWarning.ErrorString)
	assert.Equal(t, "Cannot access log file /var/log/app.log due to permissions", plWarning.Message)

	// Check that Error() returns the serialized json
	jsonWarning, err := json.Marshal(warnings[0])
	require.NoError(t, err)
	assert.Equal(t, string(jsonWarning), warnings[0].Error())
}

func TestProcessLogWarningRemoval(t *testing.T) {
	// Clear any existing warnings
	clearAllWarnings()

	// Add multiple warnings
	AddWarning("file1.log", os.ErrPermission, "File 1 warning")
	AddWarning("file2.log", os.ErrNotExist, "File 2 warning")

	check := newCheck()
	senderManager := mocksender.CreateDefaultDemultiplexer()

	err := check.Configure(senderManager, 0, integration.Data{}, integration.Data{}, "test")
	require.NoError(t, err)

	err = check.Run()
	require.NoError(t, err)

	warnings := check.GetWarnings()
	require.Len(t, warnings, 2)

	// Remove one warning
	RemoveWarning("file1.log")

	err = check.Run()
	require.NoError(t, err)

	warnings = check.GetWarnings()
	require.Len(t, warnings, 1)

	plWarning, ok := warnings[0].(*Warning)
	require.True(t, ok)
	assert.Equal(t, "file2.log", plWarning.Resource)

	// Remove remaining warning
	RemoveWarning("file2.log")

	err = check.Run()
	require.NoError(t, err)

	warnings = check.GetWarnings()
	require.Len(t, warnings, 0)
}

// TestPermissionDeniedErrorCodeConsistency ensures that permission denied errors
// generate consistent error codes that the backend can rely on.
func TestPermissionDeniedErrorCodeConsistency(t *testing.T) {
	// Clear any existing warnings
	clearAllWarnings()

	testFile := "/path/to/test/file.log"

	// Test direct os.ErrPermission - this is what os.Open returns for permission issues
	t.Run("direct os.ErrPermission", func(t *testing.T) {
		AddWarning(testFile, os.ErrPermission, "Test permission denied message")

		// use getWarningsAsErrors instead
		warnings := globalWarningCollector.getWarningsAsErrors()
		require.Len(t, warnings, 1)

		plWarning, ok := warnings[0].(*Warning)
		require.True(t, ok)

		// The backend expects this exact error code for permission denied errors
		assert.Equal(t, errorCodePermissionDenied, plWarning.ErrorCode,
			"Permission denied error code must remain consistent for backend compatibility")
		assert.Equal(t, os.ErrPermission.Error(), plWarning.ErrorString,
			"Error string should contain the original error message")

		clearAllWarnings()
	})

	// Test wrapped permission error - this is what might come from os.Open with file path
	t.Run("wrapped permission error", func(t *testing.T) {
		wrappedErr := fmt.Errorf("open %s: %w", testFile, os.ErrPermission)
		AddWarning(testFile, wrappedErr, "Test wrapped permission denied message")

		warnings := globalWarningCollector.getWarningsAsErrors()
		require.Len(t, warnings, 1)

		plWarning, ok := warnings[0].(*Warning)
		require.True(t, ok)

		// Should still get the permission denied error code
		assert.Equal(t, errorCodePermissionDenied, plWarning.ErrorCode,
			"Wrapped permission errors should still use permission-denied error code")
		assert.Equal(t, wrappedErr.Error(), plWarning.ErrorString,
			"Error string should contain the full wrapped error message")

		clearAllWarnings()
	})
}
