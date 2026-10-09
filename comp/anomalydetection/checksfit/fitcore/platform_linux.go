// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Adapted from the Fast IPC Toolkit's lib/go/fitcore (module `fit`): the transport
// comes from commit 4961722de9009afdbbb711fc0adf14a8f6ff9277, and the broadcast
// transport from commit 788233d2ffcc1e9d19b8e8202ca7908b64c64687
// (ddoghq-sandbox/celian-26q4-innov-fast-ipc-toolkit).
//
// Local changes: the darwin build requires cgo, unsupported platforms get a
// stub so this tree still compiles, and test files carry an explicit platform
// gate. The transport logic, framing, and ring layout are unchanged.

//go:build linux

package fitcore

import (
	"os"
	"syscall"
	"unsafe"
)

// Futex operation numbers from linux/futex.h. The stdlib syscall package
// exports SYS_FUTEX but not these operation constants.
const (
	futexOpWait = 0 // FUTEX_WAIT
	futexOpWake = 1 // FUTEX_WAKE
)

// checkOSVersion reports the platform requirements for shared address waits.
// Linux futexes are available on every supported kernel.
func checkOSVersion() error {
	return nil
}

// shmOpen opens or creates a POSIX shared-memory object. On Linux, POSIX shm
// objects live in the /dev/shm tmpfs, so shm_open is an ordinary open of
// /dev/shm/name, exactly what glibc does. O_NOFOLLOW and O_CLOEXEC mirror
// glibc's hardening: a planted symlink cannot redirect the producer's open,
// and the descriptor never leaks into an exec'd child.
func shmOpen(name string, flag int, mode uint32) (int, error) {
	flag |= syscall.O_NOFOLLOW | syscall.O_CLOEXEC
	fd, err := syscall.Open("/dev/shm"+name, flag, mode)
	if err != nil {
		return -1, err
	}
	return fd, nil
}

// shmUnlink removes a POSIX shared-memory object name.
func shmUnlink(name string) error {
	if err := os.Remove("/dev/shm" + name); err != nil {
		return err
	}
	return nil
}

// shmExpectedBackingSize returns the backing size Linux reports for a POSIX
// shm object: the exact logical length.
func shmExpectedBackingSize(regionSize int) (int64, error) {
	return int64(regionSize), nil
}

// shmModeAllowed checks the owner/mode policy Linux reports for POSIX shm:
// no group or other permission bits may be set.
func shmModeAllowed(mode uint32) bool {
	return mode&0o077 == 0
}

// waitWord blocks until the 32-bit shared word differs from expected, using a
// shared (non-private) futex wait on the aligned write index. EAGAIN means the
// value changed; EINTR means an interruption; both return so the caller
// rechecks the queue.
func waitWord(word *uint32, expected uint32) error {
	_, _, errno := syscall.Syscall6(
		syscall.SYS_FUTEX,
		uintptr(unsafe.Pointer(word)),
		futexOpWait,
		uintptr(expected),
		0, 0, 0,
	)
	if errno == 0 {
		return nil
	}
	if errno == syscall.EAGAIN || errno == syscall.EINTR {
		return nil
	}
	return errno
}

// wakeWord wakes one waiter on the shared write index using a shared
// (non-private) futex wake. Waking zero waiters is not an error.
func wakeWord(word *uint32) error {
	_, _, errno := syscall.Syscall6(
		syscall.SYS_FUTEX,
		uintptr(unsafe.Pointer(word)),
		futexOpWake,
		1,
		0, 0, 0,
	)
	if errno == 0 {
		return nil
	}
	return errno
}

// wakeAllWord wakes every waiter on a shared word. A broadcast publication must
// wake all subscribers because they share one write cursor.
func wakeAllWord(word *uint32) error {
	_, _, errno := syscall.Syscall6(
		syscall.SYS_FUTEX,
		uintptr(unsafe.Pointer(word)),
		futexOpWake,
		uintptr(1<<31-1),
		0, 0, 0,
	)
	if errno == 0 {
		return nil
	}
	return errno
}
