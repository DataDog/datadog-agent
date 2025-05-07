// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build windows

package winutil

import (
	"errors"
	"fmt"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"golang.org/x/sys/windows"
	"testing"
	"unsafe"
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
			description: "protected process 1",
			processName: "csrss.exe",
			expected:    true,
		},
		{
			description: "protected process 2",
			processName: "services.exe",
			expected:    true,
		},
		{
			description: "protected process 3",
			processName: "smss.exe",
			expected:    true,
		},
		{
			description: "protected process 4",
			processName: "wininit.exe",
			expected:    true,
		},
		{
			description: "unprotected process",
			processName: "<CURRENT_PROCESS>",
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
				fmt.Printf("pid: %d\n", pid)
				require.NoError(t, err)
			}

			procHandle, err := windows.OpenProcess(windows.PROCESS_QUERY_LIMITED_INFORMATION, false, pid)
			require.NoError(t, err)

			if tc.expected {
				var procmem uintptr
				var peb peb32
				toRead := uint32(unsafe.Sizeof(peb))
				bytesRead, err := ReadProcessMemory(procHandle, procmem, uintptr(unsafe.Pointer(&peb)), toRead)
				require.Error(t, err)
				assert.Equal(t, uint64(0), bytesRead)
			}

			isProcessProtected, err := IsProcessProtected(procHandle)
			assert.Equal(t, tc.expected, isProcessProtected)
		})
	}
}
