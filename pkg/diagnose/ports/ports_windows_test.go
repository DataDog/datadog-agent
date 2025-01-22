// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package ports

import (
	"syscall"
	"testing"

	"github.com/stretchr/testify/require"
)

// TestRetrieveProcessName_ValidPID tests that RetrieveProcessName returns a non-empty name
// for the current process (should be the Agent).
func TestRetrieveProcessName_ValidPID(t *testing.T) {
	// Grab current process PID
	pid := syscall.Getpid()

	name, err := RetrieveProcessName(pid, "")
	require.NoError(t, err, "RetrieveProcessName failed with error: %v", err)
	require.NotEmpty(t, name, "Expected a non-empty process name for PID %d, but got an empty string", pid)
}

// TestRetrieveProcessName_InvalidPID tests that RetrieveProcessName returns an error
// if the PID is invalid.
func TestRetrieveProcessName_InvalidPID(t *testing.T) {
	_, err := RetrieveProcessName(-1, "")
	require.Error(t, err, "Expected an error when calling RetrieveProcessName with an invalid PID (-1), but got nil")
}
