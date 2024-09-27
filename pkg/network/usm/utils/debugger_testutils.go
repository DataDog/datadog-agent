// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux && test

package utils

import (
	"slices"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

// TraceMethod represents the method to trace a program.
type TraceMethod bool

const (
	// ManualTracingFallbackEnabled is used to enable manual tracing fallback
	ManualTracingFallbackEnabled TraceMethod = true
	// ManualTracingFallbackDisabled is used to disable manual tracing fallback
	ManualTracingFallbackDisabled TraceMethod = false
)

// GetTracedPrograms returns a list of traced programs by the specific program type
func GetTracedPrograms(programType string) []TracedProgram {
	res := debugger.GetTracedPrograms()
	i := 0 // output index
	for _, x := range res {
		if x.ProgramType == programType {
			// copy and increment index
			res[i] = x
			i++
		}
	}
	return res[:i]
}

// ResetDebugger resets the debugger instance. Since this is a global variable, creating multiple monitors
// in the same test will cause the debugger to contain multiple and old instances of the same program.
func ResetDebugger() {
	debugger = &tlsDebugger{
		attachers: make(map[string]Attacher),
	}
}

// IsProgramTraced checks if the process with the provided PID is
// traced.
func IsProgramTraced(programType string, pid int) bool {
	traced := GetTracedPrograms(programType)
	for _, prog := range traced {
		if slices.Contains[[]uint32](prog.PIDs, uint32(pid)) {
			return true
		}
	}
	return false
}

// WaitForProgramsToBeTraced waits for the program to be traced by the debugger
func WaitForProgramsToBeTraced(t *testing.T, programType string, pid int, traceManually TraceMethod) {
	// Wait for the program to be traced
	end := time.Now().Add(time.Second * 5)
	for time.Now().Before(end) {
		if IsProgramTraced(programType, pid) {
			return
		}
		time.Sleep(time.Millisecond * 100)
	}

	// Reaching here means the program is not traced
	if !traceManually {
		// We should not apply manual tracing, thus we should fail.
		t.Fatalf("process %v is not traced by %v", pid, programType)
	}
	t.Logf("process %v is not traced by %v, trying to attach manually", pid, programType)

	// Get attacher for the program type
	attacher, ok := debugger.attachers[programType]
	require.True(t, ok, "attacher for %v not found", programType)
	// Try to attach the PID. Any error other than ErrPathIsAlreadyRegistered is a failure.
	if err := attacher.AttachPID(uint32(pid)); err != ErrPathIsAlreadyRegistered {
		require.NoError(t, err)
	}
	require.Eventuallyf(t, func() bool {
		return IsProgramTraced(programType, pid)
	}, time.Second*5, time.Millisecond*100, "process %v is not traced by %v", pid, programType)
}

// WaitForPathToBeBlocked waits for the path to be blocked from tracing in the
// registry (due to failing activation).
func WaitForPathToBeBlocked(t *testing.T, programType string, path string) {
	pathID, err := NewPathIdentifier(path)
	require.NoError(t, err)
	require.Eventuallyf(t, func() bool {
		blocked := debugger.GetBlockedPathIDs(programType)
		for _, id := range blocked {
			if id == pathID {
				return true
			}
		}
		return false
	}, time.Second*5, time.Millisecond*100, "path %v is not blocked in %v", path, programType)
}
