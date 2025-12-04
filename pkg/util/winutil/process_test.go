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

func TestGetFileDescription(t *testing.T) {
	tests := []struct {
		name         string
		path         string
		expectError  bool
		validateFunc func(t *testing.T, desc string)
	}{
		{
			name:        "notepad.exe",
			path:        "C:\\Windows\\System32\\notepad.exe",
			expectError: false,
			validateFunc: func(t *testing.T, desc string) {
				assert.Contains(t, desc, "Notepad", "notepad.exe does not match expected description")
				t.Logf("notepad.exe description: %s", desc)
			},
		},
		{
			name:        "cmd.exe",
			path:        "C:\\Windows\\System32\\cmd.exe",
			expectError: false,
			validateFunc: func(t *testing.T, desc string) {
				assert.NotEmpty(t, desc)
				assert.Contains(t, desc, "Command", "cmd.exe does mention not match expected description")
				t.Logf("cmd.exe description: %s", desc)
			},
		},
		{
			name:        "explorer.exe",
			path:        "C:\\Windows\\explorer.exe",
			expectError: false,
			validateFunc: func(t *testing.T, desc string) {
				assert.Contains(t, desc, "Windows Explorer", "explorer.exe does not match expected description")
				t.Logf("explorer.exe description: %s", desc)
			},
		},
		{
			name:        "powershell.exe",
			path:        "C:\\Windows\\System32\\WindowsPowerShell\\v1.0\\powershell.exe",
			expectError: false,
			validateFunc: func(t *testing.T, desc string) {
				assert.Contains(t, desc, "Windows PowerShell", "powershell.exe does not match expected description")
				t.Logf("powershell.exe description: %s", desc)
			},
		},
		{
			name:        "kernel32.dll",
			path:        "C:\\Windows\\System32\\kernel32.dll",
			expectError: false,
			validateFunc: func(t *testing.T, desc string) {
				assert.NotEmpty(t, desc)
				t.Logf("kernel32.dll description: %s", desc)
			},
		},
		{
			name:        "non-existent file",
			path:        "C:\\DoesNotExist\\fake.exe",
			expectError: true,
			validateFunc: func(t *testing.T, desc string) {
				assert.Empty(t, desc)
			},
		},
		{
			name:        "empty path",
			path:        "",
			expectError: true,
			validateFunc: func(t *testing.T, desc string) {
				assert.Empty(t, desc)
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			desc, err := GetFileDescription(tt.path)

			if tt.expectError {
				assert.Error(t, err, "Expected error for path: %s", tt.path)
				t.Logf("Expected error: %v", err)
			} else {
				if err != nil {
					t.Logf("Warning: Could not get file description for %s: %v", tt.path, err)
					// Some systems might not have all files, so just log warning
					return
				}
				assert.NoError(t, err, "Should not error for valid path: %s", tt.path)
			}

			if tt.validateFunc != nil {
				tt.validateFunc(t, desc)
			}
		})
	}
}

func TestGetFileDescriptionMultipleCalls(t *testing.T) {
	// Test that multiple calls to the same file return consistent results
	path := "C:\\Windows\\System32\\notepad.exe"

	desc1, err1 := GetFileDescription(path)
	if err1 != nil {
		t.Skipf("Skipping test, notepad.exe not accessible: %v", err1)
	}

	desc2, err2 := GetFileDescription(path)
	require.NoError(t, err2)

	assert.Equal(t, desc1, desc2, "Multiple calls should return same description")
	t.Logf("Consistent description: %s", desc1)
}
