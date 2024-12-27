// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package ports

import (
	"strings"
	"syscall"
	"testing"
)

// TestRetrieveProcessName_ValidPID tests that RetrieveProcessName returns a non-empty name
// for the current process (should be the Agent).
func TestRetrieveProcessName_ValidPID(t *testing.T) {
	// Grab current process PID
	pid := syscall.Getpid()

	name, err := RetrieveProcessName(pid, "")
	if err != nil {
		t.Fatalf("RetrieveProcessName failed for PID %d: %v", pid, err)
	}

	if name == "" {
		t.Errorf("Expected a non-empty process name for PID %d, but got an empty string", pid)
	} else {
		t.Logf("RetrieveProcessName succeeded. PID %d => '%s'", pid, name)
	}
}

// TestRetrieveProcessName_InvalidPID tests that RetrieveProcessName returns an error
// if the PID is invalid.
func TestRetrieveProcessName_InvalidPID(t *testing.T) {
	_, err := RetrieveProcessName(-1, "")
	if err == nil {
		t.Error("Expected an error when calling RetrieveProcessName with an invalid PID (-1), but got nil")
	} else if !strings.Contains(strings.ToLower(err.Error()), "ntquer") {
		// We only do a loose check here, to see if it's the NT query error
		t.Logf("Received expected error for invalid PID: %v", err)
	}
}
