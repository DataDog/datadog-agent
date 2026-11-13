// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux && test

package utils

import (
	"errors"
	"path/filepath"
	"slices"
	"strings"
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
func GetTracedPrograms(moduleName, programType string) []TracedProgram {
	res := debugger.GetTracedPrograms(moduleName)
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
		registries: make(map[string][]*FileRegistry),
		attachers:  make(map[string]map[string]Attacher),
	}
}

// IsProgramTraced checks if the process with the provided PID is
// traced.
func IsProgramTraced(moduleName, programType string, pid int) bool {
	traced := GetTracedPrograms(moduleName, programType)
	for _, prog := range traced {
		if slices.Contains[[]uint32](prog.PIDs, uint32(pid)) {
			return true
		}
	}
	return false
}

// WaitForProgramsToBeTraced waits for the program to be traced by the given programs type of the given module.
func WaitForProgramsToBeTraced(t *testing.T, moduleName, programType string, pid int, traceManually TraceMethod) {
	t.Helper()
	// Wait for the program to be traced
	end := time.Now().Add(time.Second * 5)
	for time.Now().Before(end) {
		if IsProgramTraced(moduleName, programType, pid) {
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

	require.NoError(t, attachManually(t, moduleName, programType, pid))
	require.Eventuallyf(t, func() bool {
		return IsProgramTraced(moduleName, programType, pid)
	}, time.Second*5, time.Millisecond*100, "process %v is not traced by %v", pid, programType)
}

// attachManually attaches pid through the attacher registered for programType. It returns
// AttachPID's error, except ErrPathIsAlreadyRegistered, which means the pid was already attached.
func attachManually(t *testing.T, moduleName, programType string, pid int) error {
	t.Helper()
	attacher, ok := debugger.attachers[moduleName][programType]
	require.True(t, ok, "attacher for %v not found", programType)
	if err := attacher.AttachPID(uint32(pid)); err != nil && !errors.Is(err, ErrPathIsAlreadyRegistered) {
		return err
	}
	return nil
}

// librariesTracedTimeout is how long WaitForLibrariesToBeTraced waits before, and again after,
// the manual attach. Unit tests shorten it.
var librariesTracedTimeout = 5 * time.Second

// missingTracedLibraries returns the entries of libPrefixes for which pid is not yet traced
// through a library whose file name starts with that prefix.
func missingTracedLibraries(moduleName, programType string, pid int, libPrefixes []string) []string {
	traced := GetTracedPrograms(moduleName, programType)
	var missing []string
	for _, prefix := range libPrefixes {
		found := slices.ContainsFunc(traced, func(p TracedProgram) bool {
			return slices.Contains(p.PIDs, uint32(pid)) && strings.HasPrefix(filepath.Base(p.FilePath), prefix)
		})
		if !found {
			missing = append(missing, prefix)
		}
	}
	return missing
}

// WaitForLibrariesToBeTraced waits until pid is traced through a library matching each of
// libPrefixes (e.g. "libssl.so", "libcrypto.so"). Use it instead of WaitForProgramsToBeTraced
// when the probes span several libraries: the attacher registers each library separately, as
// its open event arrives, so a pid can be traced through one library before the others.
//
// If the libraries are not all traced within 5 seconds and traceManually is set, it attaches
// the pid manually. AttachPID attaches every matching library in the pid's maps and skips the
// ones that are already registered, so it fills in whichever library was missed. An AttachPID
// error is only logged: it can come from an unrelated matching library, so the wait that
// follows decides whether the listed libraries are traced.
func WaitForLibrariesToBeTraced(t *testing.T, moduleName, programType string, pid int, libPrefixes []string, traceManually TraceMethod) {
	t.Helper()
	waitForAll := func(timeout time.Duration) []string {
		end := time.Now().Add(timeout)
		for {
			missing := missingTracedLibraries(moduleName, programType, pid, libPrefixes)
			if len(missing) == 0 || !time.Now().Before(end) {
				return missing
			}
			time.Sleep(50 * time.Millisecond)
		}
	}

	missing := waitForAll(librariesTracedTimeout)
	if len(missing) == 0 {
		return
	}
	if !traceManually {
		t.Fatalf("process %v is not traced by %v through %v", pid, programType, missing)
	}
	t.Logf("process %v is not traced by %v through %v, trying to attach manually", pid, programType, missing)

	if err := attachManually(t, moduleName, programType, pid); err != nil {
		t.Logf("manual attach of process %v returned an error: %v", pid, err)
	}
	if missing = waitForAll(librariesTracedTimeout); len(missing) != 0 {
		t.Fatalf("process %v is not traced by %v through %v after manual attach", pid, programType, missing)
	}
}

// WaitForPathToBeBlocked waits for the path to be blocked from tracing in the
// registry (due to failing activation).
func WaitForPathToBeBlocked(t *testing.T, moduleName, programType string, path string) {
	pathID, err := NewPathIdentifier(path)
	require.NoError(t, err)
	require.Eventuallyf(t, func() bool {
		blocked := debugger.GetBlockedPathIDs(moduleName, programType)
		return slices.Contains(blocked, pathID)
	}, time.Second*5, time.Millisecond*100, "path %v is not blocked in %v", path, programType)
}
