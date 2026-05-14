// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package testcommon

import (
	"fmt"
	"os"

	"golang.org/x/sys/windows"
)

// tryLoadSequence replicates what loadAndCreate() in api.cpp does and reports each step,
// so we can see exactly where the failure is before the C layer obscures the error code.
func tryLoadSequence(dllPath, pythonHomeStr string) {
	// Step 1: full-path load without SetDllDirectory (baseline — expected to fail if python313.dll
	// is not in the standard search path)
	handle, err := windows.LoadDLL(dllPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "[rtloader debug] LoadDLL full-path (no SetDllDir): FAILED: %v\n", err)
	} else {
		handle.Release()
		fmt.Fprintf(os.Stderr, "[rtloader debug] LoadDLL full-path (no SetDllDir): OK\n")
	}

	// Step 2: call SetDllDirectory the same way api.cpp does, then retry
	if err := windows.SetDllDirectory(pythonHomeStr); err != nil {
		fmt.Fprintf(os.Stderr, "[rtloader debug] SetDllDirectory(%s): FAILED: %v\n", pythonHomeStr, err)
	} else {
		fmt.Fprintf(os.Stderr, "[rtloader debug] SetDllDirectory(%s): OK\n", pythonHomeStr)
	}

	handle, err = windows.LoadDLL(dllPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "[rtloader debug] LoadDLL full-path (after SetDllDir): FAILED: %v\n", err)
	} else {
		handle.Release()
		fmt.Fprintf(os.Stderr, "[rtloader debug] LoadDLL full-path (after SetDllDir): OK\n")
	}

	// Reset SetDllDirectory so we don't affect the subsequent make3 call
	// (passing empty string restores default behaviour)
	_ = windows.SetDllDirectory("")
}
