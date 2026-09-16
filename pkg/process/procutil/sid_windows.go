// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build windows

package procutil

import (
	"fmt"
	"sync"
	"syscall"
	"unsafe"

	"golang.org/x/sys/windows"
)

var procAdjustTokenPrivileges = windows.NewLazySystemDLL("advapi32.dll").NewProc("AdjustTokenPrivileges")

var (
	enableDebugPrivilegeMu      sync.Mutex
	enableDebugPrivilegeEnabled bool
)

// enableDebugPrivilege enables SeDebugPrivilege on this process's own token. LocalSystem (the account process-agent runs as) holds this privilege by default, but a held privilege isn't applied to access checks until explicitly turned on here; without this, OpenProcess/OpenProcessToken below can fail with ERROR_ACCESS_DENIED against processes with a hardened or non-default DACL (e.g. elevated/admin-launched processes). Only a successful outcome is cached: a transient failure (e.g. the token not being ready yet) must not permanently disable this for the life of the process.
func enableDebugPrivilege() error {
	enableDebugPrivilegeMu.Lock()
	defer enableDebugPrivilegeMu.Unlock()
	if enableDebugPrivilegeEnabled {
		return nil
	}

	var token windows.Token
	if err := windows.OpenProcessToken(windows.CurrentProcess(), windows.TOKEN_ADJUST_PRIVILEGES|windows.TOKEN_QUERY, &token); err != nil {
		return fmt.Errorf("failed to open process token: %w", err)
	}
	defer token.Close()

	privName, err := windows.UTF16PtrFromString("SeDebugPrivilege")
	if err != nil {
		return fmt.Errorf("failed to encode SeDebugPrivilege name: %w", err)
	}

	var tp windows.Tokenprivileges
	if err := windows.LookupPrivilegeValue(nil, privName, &tp.Privileges[0].Luid); err != nil {
		return fmt.Errorf("failed to look up SeDebugPrivilege LUID: %w", err)
	}
	tp.PrivilegeCount = 1
	tp.Privileges[0].Attributes = windows.SE_PRIVILEGE_ENABLED

	if err := adjustTokenPrivileges(token, &tp); err != nil {
		return fmt.Errorf("failed to adjust token privileges: %w", err)
	}

	enableDebugPrivilegeEnabled = true
	return nil
}

// adjustTokenPrivileges calls the raw Win32 AdjustTokenPrivileges API directly instead of using
// windows.AdjustTokenPrivileges, because that BOOL-returning API reports success even when it
// silently fails to enable a privilege the token doesn't actually hold — that partial-failure case
// is only signaled via GetLastError() == ERROR_NOT_ALL_ASSIGNED, which the x/sys/windows wrapper
// never surfaces as a Go error. LazyProc.Call captures GetLastError() atomically as part of the
// same syscall trampoline, so checking it here (unlike a separate windows.GetLastError() call
// afterwards) isn't racy with respect to goroutine/OS-thread migration.
func adjustTokenPrivileges(token windows.Token, tp *windows.Tokenprivileges) error {
	r1, _, callErr := procAdjustTokenPrivileges.Call(
		uintptr(token),
		0,
		uintptr(unsafe.Pointer(tp)),
		0,
		0,
		0,
	)
	if r1 == 0 {
		return callErr
	}
	if errno, ok := callErr.(syscall.Errno); ok && errno == windows.ERROR_NOT_ALL_ASSIGNED {
		return fmt.Errorf("token does not hold SeDebugPrivilege: %w", callErr)
	}
	return nil
}

// GetSIDForPID returns the Windows SID string of the process owning pid (e.g. "S-1-5-21-...-1001"), resolved using this process's own token privileges. It is meant to be called from process-agent, which runs as LocalSystem, so OpenProcessToken below succeeds even when pid belongs to a different, lower-privileged user — letting other, lower-privileged agent processes ask process-agent for a PID's owner instead of needing that broad privilege granted to them directly.
func GetSIDForPID(pid int32) (string, error) {
	if err := enableDebugPrivilege(); err != nil {
		return "", fmt.Errorf("failed to enable SeDebugPrivilege: %w", err)
	}

	// OpenProcessHandle can return a valid handle alongside a non-nil error: its own internal
	// second OpenProcess call (an unrelated PROCESS_VM_READ upgrade attempt) may fail even though
	// the first, already-successful PROCESS_QUERY_LIMITED_INFORMATION handle is all this function
	// needs for the OpenProcessToken(..., TOKEN_QUERY, ...) call below. Only a zero handle means
	// no usable handle was obtained at all.
	h, _, err := OpenProcessHandle(pid)
	if h != 0 {
		defer windows.Close(h)
	} else {
		return "", fmt.Errorf("failed to open process %d: %w", pid, err)
	}

	var token windows.Token
	if err := windows.OpenProcessToken(h, windows.TOKEN_QUERY, &token); err != nil {
		return "", fmt.Errorf("failed to open process token for pid %d: %w", pid, err)
	}
	defer token.Close()

	tokenUser, err := token.GetTokenUser()
	if err != nil {
		return "", fmt.Errorf("failed to get token user for pid %d: %w", pid, err)
	}

	return tokenUser.User.Sid.String(), nil
}
