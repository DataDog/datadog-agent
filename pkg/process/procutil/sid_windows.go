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

// ErrConnectionOwnerNotFound means no process currently owns the queried TCP connection. Callers should
// treat it as "not found" (e.g. HTTP 404), not as an internal error.
var ErrConnectionOwnerNotFound = errors.New("no process owns the connection")

// ErrConnectionOwnerChanged means the connection's owning PID changed between the two table reads that
// bracket GetSIDForConnectionOwner's process-handle open — i.e. the original owner exited (tearing down
// its connection) and the PID may have been reused. The lookup is rejected rather than trusting a
// possibly-unrelated process's identity. Callers should treat it as "not found" too.
var ErrConnectionOwnerChanged = errors.New("connection owner changed during resolution (possible PID reuse)")

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

// ensureDebugPrivilege enables SeDebugPrivilege best-effort. process-agent runs as LocalSystem, which holds
// it by default, and it is what lets the token reads below reach hardened or other-user processes. It is not
// required to resolve a normal, same-user process (e.g. the test binary's own), and a token that simply
// lacks the privilege — such as a developer running these tests unelevated — makes AdjustTokenPrivileges
// report ERROR_NOT_ALL_ASSIGNED. So a failure here is logged and tolerated rather than fatal: if the
// privilege was genuinely needed, the OpenProcess/token read that follows fails on its own with
// ERROR_ACCESS_DENIED.
func ensureDebugPrivilege() {
	if err := enableDebugPrivilege(); err != nil {
		log.Debugf("proceeding without SeDebugPrivilege: %v", err)
	}
}

// sidFromProcessHandle reads the owning user's SID string (e.g. "S-1-5-21-...-1001") from an already-open
// process handle. The handle must grant at least PROCESS_QUERY_LIMITED_INFORMATION (as OpenProcessHandle
// returns), which is enough to open the process token for TOKEN_QUERY.
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

// openProcessHandleForSID opens a handle to pid suitable for reading its token SID. OpenProcessHandle can
// return a valid handle alongside a non-nil error: its own internal second OpenProcess call (an unrelated
// PROCESS_VM_READ upgrade attempt) may fail even though the first, already-successful
// PROCESS_QUERY_LIMITED_INFORMATION handle is all this function needs. Only a zero handle means no usable
// handle was obtained at all.
func openProcessHandleForSID(pid uint32) (windows.Handle, error) {
	h, _, err := OpenProcessHandle(int32(pid))
	if h == 0 {
		return 0, fmt.Errorf("failed to open process %d: %w", pid, err)
	}
	return h, nil
}

// GetSIDForPID returns the Windows SID string of the process owning pid, resolved using this process's own
// token privileges. It is meant to be called from process-agent, which runs as LocalSystem, so the token
// read succeeds even when pid belongs to a different, lower-privileged user. It performs no validation that
// pid still identifies the process the caller expected: a PID is not a stable identifier, so callers
// resolving the owner of a network connection must use GetSIDForConnectionOwner, which guards against PID
// reuse, rather than resolving a PID themselves and passing it here.
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

// GetSIDForConnectionOwner returns the SID of the process owning the loopback TCP connection whose local
// endpoint is localAddr:localPort and whose remote endpoint is remoteAddr:remotePort (family is
// windows.AF_INET or windows.AF_INET6). It resolves the owner race-free against PID reuse:
//
//  1. read the TCP table to find the owning PID;
//  2. open a handle to that PID — Windows will not recycle a PID while any handle to its process object
//     remains open, so holding this handle pins the identity for the rest of the call;
//  3. re-read the TCP table and confirm the connection still maps to the same PID. If the original owner
//     had exited between steps 1 and 2 its connection would be gone (TCP tears down when the owner dies)
//     and the PID could have been reused, so a mismatch here is rejected rather than trusted;
//  4. read the SID from the handle opened in step 2.
//
// This is the entry point callers must use to attribute a connection to an OS identity; see GetSIDForPID's
// note on why passing a separately-resolved PID is unsafe.
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

	// Re-validate with the handle held (see step 3 above).
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
