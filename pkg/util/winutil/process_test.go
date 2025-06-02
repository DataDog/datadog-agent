// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build windows

package winutil

import (
	"errors"
	"fmt"
	"testing"
	"unsafe"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"golang.org/x/sys/windows"
)

// similar to ProcessesByPID but needed a different implementation to find
// specific protected processes by name
func getPIDbyName(name string) (uint32, error) {
	// create snapshot of processes
	// 0x00000002 is the flag for a process snapshot called TH32CS_SNAPPROCESS
	hSnap, err := windows.CreateToolhelp32Snapshot(0x00000002, 0)
	if err != nil {
		return 0, fmt.Errorf("CreateToolhelp32Snapshot: %w", err)
	}

	defer windows.CloseHandle(hSnap)

	var pe windows.ProcessEntry32
	pe.Size = uint32(unsafe.Sizeof(pe))

	// get the first process
	if err := windows.Process32First(hSnap, &pe); err != nil {
		return 0, fmt.Errorf("Process32First: %w", err)
	}

	// iterate through list of processes
	for {
		processName := windows.UTF16ToString(pe.ExeFile[:])
		if processName == name {
			return pe.ProcessID, nil
		}
		if err := windows.Process32Next(hSnap, &pe); err != nil {
			// no more entries
			if errors.Is(err, windows.ERROR_NO_MORE_FILES) {
				break
			}
			return 0, fmt.Errorf("Process32Next: %w", err)
		}
	}

	return 0, fmt.Errorf("%s not found", name)
}

func TestIsProcessProtected(t *testing.T) {
	for _, tc := range []struct {
		description string
		processName string
		expected    bool
	}{
		{
			description: "protected process light. session manager subsytem (first user mode process)",
			processName: "csrss.exe",
			expected:    true,
		},
		{
			description: "protected process light. client server runtime subsystem (needed for Win32 calls)",
			processName: "smss.exe",
			expected:    true,
		},
		{
			description: "unprotected process. current process",
			processName: "<CURRENT_PROCESS>",
			expected:    false,
		},
		{
			description: "unprotected process. generic service host process",
			processName: "svchost.exe",
			expected:    false,
		},
	} {
		t.Run(tc.description, func(t *testing.T) {
			var pid uint32
			var err error
			if tc.processName == "<CURRENT_PROCESS>" {
				pid = windows.GetCurrentProcessId()
			} else {
				pid, err = getPIDbyName(tc.processName)
				require.NoError(t, err)
			}

			procHandle, err := windows.OpenProcess(windows.PROCESS_QUERY_LIMITED_INFORMATION, false, pid)
			require.NoError(t, err)
			defer windows.CloseHandle(procHandle)

			isProcessProtected, err := IsProcessProtected(procHandle)
			assert.NoError(t, err)
			assert.Equal(t, tc.expected, isProcessProtected)

			// CHECKING MEMORY ACCESS PRIVILEGES
			if tc.expected {
				_, err := GetCommandParamsForProcess(procHandle, true)
				assert.Error(t, err)
			} else {
				// unprotected process's should allow reading memory with a higher privilege handle
				privilegedProcHandle, err := windows.OpenProcess(windows.PROCESS_QUERY_LIMITED_INFORMATION|windows.PROCESS_VM_READ, false, pid)
				assert.NoError(t, err)
				defer windows.CloseHandle(privilegedProcHandle)
				_, err = GetCommandParamsForProcess(privilegedProcHandle, true)
				assert.NoError(t, err)
			}
		})
	}
}
