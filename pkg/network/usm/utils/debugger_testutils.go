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

// WaitForProgramsToBeTraced waits for the program to be traced by the debugger
func WaitForProgramsToBeTraced(t *testing.T, programType string, pid int) {
	require.Eventuallyf(t, func() bool {
		traced := GetTracedPrograms(programType)
		for _, prog := range traced {
			if slices.Contains[[]uint32](prog.PIDs, uint32(pid)) {
				return true
			}
		}
		return false
	}, time.Second*5, time.Millisecond*100, "process %v is not traced by %v", pid, programType)
}
