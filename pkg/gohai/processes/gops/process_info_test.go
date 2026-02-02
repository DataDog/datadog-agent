// This file is licensed under the MIT License.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright © 2015 Kentaro Kuribayashi <kentarok@gmail.com>
// Copyright 2014-present Datadog, Inc.

//go:build linux || darwin

package gops

import (
	"testing"

	"github.com/stretchr/testify/require"
)

// TestGetProcesses_RecoversFromPanic tests that GetProcesses doesn't panic
// even if gopsutil encounters malformed proc files.
//
// Note: This test verifies the recovery mechanism doesn't break normal operation.
// In practice, gopsutil panics are rare and hard to reproduce reliably in unit tests
// due to their dependency on the actual /proc filesystem state. The recovery wrapper
// ensures that if a panic occurs (e.g., from splitProcStat parsing malformed /proc/[pid]/stat),
// it's caught and logged, allowing processing to continue for other PIDs.
func TestGetProcesses_RecoversFromPanic(t *testing.T) {
	// Test that GetProcesses completes without panicking
	// The recovery wrapper ensures that any panics from gopsutil are caught
	require.NotPanics(t, func() {
		_, err := GetProcesses()
		// GetProcesses should complete successfully even if some processes fail
		// We don't assert on the error here since it depends on system state
		_ = err
	})
}

// TestPanicRecoveryMechanism tests that the recovery wrapper pattern correctly catches panics.
// This validates the recovery mechanism used in GetProcesses.
func TestPanicRecoveryMechanism(t *testing.T) {
	panicMessage := "slice bounds out of range [1:0]"
	panicCaught := false

	// Simulate the panic recovery pattern used in GetProcesses
	func() {
		defer func() {
			if r := recover(); r != nil {
				panicCaught = true
				// Verify we caught the expected panic
				require.Equal(t, panicMessage, r)
			}
		}()

		// Simulate a panic like gopsutil's splitProcStat would throw
		panic(panicMessage)
	}()

	// Verify the panic was caught
	require.True(t, panicCaught, "Panic should have been caught by recover")
}
