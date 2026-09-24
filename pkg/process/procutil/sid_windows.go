// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build windows

package procutil

import (
	"errors"
	"fmt"
	"net"
	"sync"
	"syscall"
	"unsafe"

	"golang.org/x/sys/windows"

	"github.com/DataDog/datadog-agent/pkg/util/log"
)

// ErrConnectionOwnerNotFound means no process owns the queried TCP connection; treat as "not found", not an internal error.
var ErrConnectionOwnerNotFound = errors.New("no process owns the connection")

// ErrConnectionOwnerChanged means the owning PID changed during resolution (possible PID reuse); treat as "not found".
var ErrConnectionOwnerChanged = errors.New("connection owner changed during resolution (possible PID reuse)")

var procAdjustTokenPrivileges = windows.NewLazySystemDLL("advapi32.dll").NewProc("AdjustTokenPrivileges")

var (
	enableDebugPrivilegeMu      sync.Mutex
	enableDebugPrivilegeEnabled bool
)

// enableDebugPrivilege turns on SeDebugPrivilege so token reads reach hardened processes; caches only success so transient failures don't disable it permanently.
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

// adjustTokenPrivileges calls raw Win32 AdjustTokenPrivileges so it can detect ERROR_NOT_ALL_ASSIGNED (partial failure the x/sys wrapper hides), atomically via LazyProc.Call to avoid a racy GetLastError.
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

// ensureDebugPrivilege enables SeDebugPrivilege best-effort; failure is logged and tolerated since it's not needed for same-user processes and the later token read fails on its own if it was.
func ensureDebugPrivilege() {
	if err := enableDebugPrivilege(); err != nil {
		log.Debugf("proceeding without SeDebugPrivilege: %v", err)
	}
}

// sidFromProcessHandle reads the owning user's SID string from an open handle granting at least PROCESS_QUERY_LIMITED_INFORMATION.
func sidFromProcessHandle(h windows.Handle) (string, error) {
	var token windows.Token
	if err := windows.OpenProcessToken(h, windows.TOKEN_QUERY, &token); err != nil {
		return "", fmt.Errorf("failed to open process token: %w", err)
	}
	defer token.Close()

	tokenUser, err := token.GetTokenUser()
	if err != nil {
		return "", fmt.Errorf("failed to get token user: %w", err)
	}

	return tokenUser.User.Sid.String(), nil
}

// openProcessHandleForSID opens a handle to pid for reading its token SID; only a zero handle is a real failure since OpenProcessHandle may return a valid handle with a non-nil error.
func openProcessHandleForSID(pid uint32) (windows.Handle, error) {
	h, _, err := OpenProcessHandle(int32(pid))
	if h == 0 {
		return 0, fmt.Errorf("failed to open process %d: %w", pid, err)
	}
	return h, nil
}

// GetSIDForPID returns the SID of the process owning pid; it does not validate pid identity, so connection owners must use GetSIDForConnectionOwner, which guards against PID reuse.
func GetSIDForPID(pid int32) (string, error) {
	ensureDebugPrivilege()

	h, err := openProcessHandleForSID(uint32(pid))
	if err != nil {
		return "", err
	}
	defer windows.Close(h)

	sid, err := sidFromProcessHandle(h)
	if err != nil {
		return "", fmt.Errorf("%w (pid %d)", err, pid)
	}
	return sid, nil
}

// GetSIDForConnectionOwner returns the SID owning the given loopback TCP connection, resolving race-free against PID reuse: find the PID, open (and thus pin) a handle to it, re-read the table to confirm the PID is unchanged, then read the SID.
func GetSIDForConnectionOwner(family uint32, localAddr net.IP, localPort int, remoteAddr net.IP, remotePort int) (string, error) {
	ensureDebugPrivilege()

	pid, ok, err := findConnectionOwnerPID(family, localAddr, localPort, remoteAddr, remotePort)
	if err != nil {
		return "", err
	}
	if !ok {
		return "", ErrConnectionOwnerNotFound
	}

	h, err := openProcessHandleForSID(pid)
	if err != nil {
		return "", err
	}
	defer windows.Close(h)

	// Re-validate with the handle held to reject PID reuse.
	pid2, ok2, err := findConnectionOwnerPID(family, localAddr, localPort, remoteAddr, remotePort)
	if err != nil {
		return "", err
	}
	if !ok2 || pid2 != pid {
		return "", ErrConnectionOwnerChanged
	}

	sid, err := sidFromProcessHandle(h)
	if err != nil {
		return "", fmt.Errorf("%w (pid %d)", err, pid)
	}
	return sid, nil
}
