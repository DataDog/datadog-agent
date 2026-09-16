// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build windows

package procutil

import (
	"fmt"
	"sync"

	"golang.org/x/sys/windows"
)

var (
	enableDebugPrivilegeOnce sync.Once
	enableDebugPrivilegeErr  error
)

// enableDebugPrivilege enables SeDebugPrivilege on this process's own token. LocalSystem (the account process-agent runs as) holds this privilege by default, but a held privilege isn't applied to access checks until explicitly turned on here; without this, OpenProcess/OpenProcessToken below can fail with ERROR_ACCESS_DENIED against processes with a hardened or non-default DACL (e.g. elevated/admin-launched processes). Runs once per process, since a token's available privileges are fixed at logon and re-enabling is a wasted syscall.
func enableDebugPrivilege() error {
	enableDebugPrivilegeOnce.Do(func() {
		var token windows.Token
		if err := windows.OpenProcessToken(windows.CurrentProcess(), windows.TOKEN_ADJUST_PRIVILEGES|windows.TOKEN_QUERY, &token); err != nil {
			enableDebugPrivilegeErr = fmt.Errorf("failed to open process token: %w", err)
			return
		}
		defer token.Close()

		privName, err := windows.UTF16PtrFromString("SeDebugPrivilege")
		if err != nil {
			enableDebugPrivilegeErr = fmt.Errorf("failed to encode SeDebugPrivilege name: %w", err)
			return
		}

		var tp windows.Tokenprivileges
		if err := windows.LookupPrivilegeValue(nil, privName, &tp.Privileges[0].Luid); err != nil {
			enableDebugPrivilegeErr = fmt.Errorf("failed to look up SeDebugPrivilege LUID: %w", err)
			return
		}
		tp.PrivilegeCount = 1
		tp.Privileges[0].Attributes = windows.SE_PRIVILEGE_ENABLED

		if err := windows.AdjustTokenPrivileges(token, false, &tp, 0, nil, nil); err != nil {
			enableDebugPrivilegeErr = fmt.Errorf("failed to adjust token privileges: %w", err)
		}
	})
	return enableDebugPrivilegeErr
}

// GetSIDForPID returns the Windows SID string of the process owning pid (e.g. "S-1-5-21-...-1001"), resolved using this process's own token privileges. It is meant to be called from process-agent, which runs as LocalSystem, so OpenProcessToken below succeeds even when pid belongs to a different, lower-privileged user — letting other, lower-privileged agent processes ask process-agent for a PID's owner instead of needing that broad privilege granted to them directly.
func GetSIDForPID(pid int32) (string, error) {
	if err := enableDebugPrivilege(); err != nil {
		return "", fmt.Errorf("failed to enable SeDebugPrivilege: %w", err)
	}

	h, _, err := OpenProcessHandle(pid)
	if h != 0 {
		defer windows.Close(h)
	}
	if err != nil {
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
