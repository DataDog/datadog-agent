// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build linux

package executor

import (
	"os"
	"path/filepath"
	"runtime"
	"syscall"
	"testing"

	"github.com/stretchr/testify/require"
)

// unprivilegedID is the conventional "nobody" UID and GID.
const unprivilegedID = 65534

// connectAsUnprivilegedUser attempts to connect to address with the credentials
// of an unrelated, unprivileged user.
//
// The credentials are dropped with raw syscalls on a locked OS thread that is
// never unlocked, so only that thread changes identity and the runtime
// discards it when the goroutine exits.
func connectAsUnprivilegedUser(address string) error {
	result := make(chan error, 1)
	go func() {
		runtime.LockOSThread()
		result <- func() error {
			if _, _, errno := syscall.RawSyscall(syscall.SYS_SETGROUPS, 0, 0, 0); errno != 0 {
				return errno
			}
			if _, _, errno := syscall.RawSyscall(syscall.SYS_SETRESGID, unprivilegedID, unprivilegedID, unprivilegedID); errno != 0 {
				return errno
			}
			if _, _, errno := syscall.RawSyscall(syscall.SYS_SETRESUID, unprivilegedID, unprivilegedID, unprivilegedID); errno != 0 {
				return errno
			}
			fd, err := syscall.Socket(syscall.AF_UNIX, syscall.SOCK_STREAM, 0)
			if err != nil {
				return err
			}
			defer syscall.Close(fd)
			return syscall.Connect(fd, &syscall.SockaddrUnix{Name: address})
		}()
	}()
	return <-result
}

func TestListenRejectsOtherUsers(t *testing.T) {
	if os.Geteuid() != 0 {
		t.Skip("switching to another user requires root")
	}

	address := testListenAddr(t)
	// Let other users traverse the directory so the socket mode alone decides access.
	require.NoError(t, os.Chmod(filepath.Dir(address), 0o755))

	lis, err := Listen(address)
	require.NoError(t, err)
	t.Cleanup(func() { _ = lis.Close() })

	denied := connectAsUnprivilegedUser(address)

	// Control: the same user must be able to connect once the socket mode
	// allows it, otherwise the denial says nothing about the socket mode.
	require.NoError(t, os.Chmod(address, 0o666))
	if err := connectAsUnprivilegedUser(address); err != nil {
		t.Skipf("the unprivileged user cannot reach the test directory (%v); cross-user check is inconclusive", err)
	}
	require.ErrorIs(t, denied, syscall.EACCES)
}
