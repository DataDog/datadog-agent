// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux

package ptracer

import (
	"syscall"
	"unsafe"

	"golang.org/x/sys/unix"
)

//go:linkname runtimeBeforeFork syscall.runtime_BeforeFork
func runtimeBeforeFork()

//go:linkname runtimeAfterFork syscall.runtime_AfterFork
func runtimeAfterFork()

//go:linkname runtimeAfterForkInChild syscall.runtime_AfterForkInChild
func runtimeAfterForkInChild()

//go:norace
//nolint:unused
func forkExec(argv0 string, argv []string, envv []string, creds Creds, prog *syscall.SockFprog) (int, error) {
	argv0p, err := syscall.BytePtrFromString(argv0)
	if err != nil {
		return 0, err
	}

	argvp, err := syscall.SlicePtrFromStrings(argv)
	if err != nil {
		return 0, err
	}

	envvp, err := syscall.SlicePtrFromStrings(envv)
	if err != nil {
		return 0, err
	}

	syscall.ForkLock.Lock()

	// no more go runtime calls
	runtimeBeforeFork()

	pid, _, errno := syscall.RawSyscall6(syscall.SYS_CLONE, uintptr(syscall.SIGCHLD), 0, 0, 0, 0, 0)
	if errno != 0 || pid != 0 {
		// back to go runtime
		runtimeAfterFork()

		syscall.ForkLock.Unlock()

		if errno != 0 {
			err = errno
		}
		return int(pid), err
	}

	// in the child, no more go runtime calls
	runtimeAfterForkInChild()

	pid, _, errno = syscall.RawSyscall(syscall.SYS_GETPID, 0, 0, 0)
	if errno != 0 {
		exit(errno)
	}

	_, _, errno = syscall.RawSyscall(syscall.SYS_PTRACE, uintptr(syscall.PTRACE_TRACEME), 0, 0)
	if errno != 0 {
		exit(errno)
	}

	_, _, errno = syscall.RawSyscall6(syscall.SYS_PRCTL, unix.PR_SET_NO_NEW_PRIVS, 1, 0, 0, 0, 0)
	if errno != 0 {
		exit(errno)
	}

	const (
		mode  = 1
		tsync = 1
	)

	_, _, errno = syscall.RawSyscall(unix.SYS_SECCOMP, mode, tsync, uintptr(unsafe.Pointer(prog)))
	if errno != 0 {
		exit(errno)
	}

	_, _, errno = syscall.RawSyscall(syscall.SYS_KILL, pid, uintptr(syscall.SIGSTOP), 0)
	if errno != 0 {
		exit(errno)
	}

	if creds.GID != nil {
		_, _, errno = syscall.RawSyscall(syscall.SYS_SETGID, uintptr(*creds.GID), 0, 0)
		if errno != 0 {
			exit(errno)
		}

	}

	if creds.UID != nil {
		_, _, errno = syscall.RawSyscall(syscall.SYS_SETUID, uintptr(*creds.UID), 0, 0)
		if errno != 0 {
			exit(errno)
		}
	}

	_, _, err = syscall.RawSyscall(syscall.SYS_EXECVE,
		uintptr(unsafe.Pointer(argv0p)),
		uintptr(unsafe.Pointer(&argvp[0])),
		uintptr(unsafe.Pointer(&envvp[0])))

	return 0, err
}

//nolint:unused
func exit(errno syscall.Errno) {
	for {
		_, _, _ = syscall.RawSyscall(syscall.SYS_EXIT, uintptr(errno), 0, 0)
	}
}
