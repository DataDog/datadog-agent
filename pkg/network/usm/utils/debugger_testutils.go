// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux

package utils

import (
	"slices"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
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
func WaitForProgramsToBeTraced(t *testing.T, programType string, pid int) {
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
