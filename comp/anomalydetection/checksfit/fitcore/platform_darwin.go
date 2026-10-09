// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Adapted from the Fast IPC Toolkit's lib/go/fitcore (module `fit`) at commit
// 4961722de9009afdbbb711fc0adf14a8f6ff9277 (ddoghq-sandbox/celian-26q4-innov-fast-ipc-toolkit).
//
// Local changes: the darwin build requires cgo, unsupported platforms get a
// stub so this tree still compiles, and test files carry an explicit platform
// gate. The transport logic, framing, and ring layout are unchanged.

//go:build darwin && cgo

package fitcore

/*
#include <stdint.h>
#include <stddef.h>
#include <stdlib.h>
#include <sys/mman.h>
#include <sys/types.h>

// The os_sync_wait_on_address family is public libSystem API introduced in
// macOS 14.4. Weak imports keep the binary loadable on older systems, where
// the symbol stays NULL and the runtime check rejects the configuration
// instead of crashing or silently falling back.
extern int os_sync_wait_on_address(void *addr, uint64_t value, size_t size, uint32_t flags) __attribute__((weak_import));
extern int os_sync_wake_by_address_any(void *addr, size_t size, uint32_t flags) __attribute__((weak_import));

static int fit_shm_open(const char *name, int oflag, mode_t mode) {
	return shm_open(name, oflag, mode);
}
static int fit_shm_unlink(const char *name) {
	return shm_unlink(name);
}
static int fit_sync_available(void) {
	return os_sync_wait_on_address != NULL && os_sync_wake_by_address_any != NULL;
}
static int fit_sync_wait(void *addr, uint64_t value) {
	return os_sync_wait_on_address(addr, value, 4, 0x1);
}
static int fit_sync_wake(void *addr) {
	return os_sync_wake_by_address_any(addr, 4, 0x1);
}
*/
import "C"

import (
	"errors"
	"syscall"
	"unsafe"
)

// checkOSVersion reports the platform requirements for shared address waits:
// macOS 14.4 or newer. The check uses weak symbol availability, which is
// exactly the runtime capability the transport needs.
func checkOSVersion() error {
	if C.fit_sync_available() == 0 {
		return errors.New("shared address waits require macOS 14.4 or newer")
	}
	return nil
}

// shmOpen opens or creates a POSIX shared-memory object through libSystem.
func shmOpen(name string, flag int, mode uint32) (int, error) {
	cName := C.CString(name)
	defer C.free(unsafe.Pointer(cName))
	ret, errno := C.fit_shm_open(cName, C.int(flag), C.mode_t(mode))
	if ret < 0 {
		return -1, errno
	}
	return int(ret), nil
}

// shmUnlink removes a POSIX shared-memory object name.
func shmUnlink(name string) error {
	cName := C.CString(name)
	defer C.free(unsafe.Pointer(cName))
	ret, errno := C.fit_shm_unlink(cName)
	if ret < 0 {
		return errno
	}
	return nil
}

// shmExpectedBackingSize returns the backing size macOS reports for a POSIX
// shm object: the logical length rounded up to the page size.
func shmExpectedBackingSize(regionSize int) (int64, error) {
	page := syscall.Getpagesize()
	if page <= 0 {
		return 0, errors.New("cannot determine shared-memory page size")
	}
	return int64((regionSize + page - 1) / page * page), nil
}

// shmModeAllowed checks the owner/mode policy macOS reports for POSIX shm:
// zero permission bits, or no group or other bits set.
func shmModeAllowed(mode uint32) bool {
	return mode == 0 || mode&0o077 == 0
}

// waitWord blocks until the 32-bit shared word differs from expected, using
// the shared os_sync_wait_on_address flag on the aligned write index. A
// value mismatch returns successfully; recheck the queue rather than
// interpreting the return as a message count. Interrupted and early returns
// are successful returns too.
func waitWord(word *uint32, expected uint32) error {
	ret, errno := C.fit_sync_wait(unsafe.Pointer(word), C.uint64_t(expected))
	if ret >= 0 {
		return nil
	}
	if errno == syscall.EINTR || errno == syscall.ENOMEM || errno == syscall.EFAULT {
		return nil
	}
	return errno
}

// wakeWord wakes one waiter on the shared write index. A wake reporting
// ENOENT means there is no waiter and is normal.
func wakeWord(word *uint32) error {
	ret, errno := C.fit_sync_wake(unsafe.Pointer(word))
	if ret >= 0 {
		return nil
	}
	if errno == syscall.ENOENT {
		return nil
	}
	return errno
}
