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

	pid, errno := forkExec1(argv0p, argvp, envvp, creds, prog)
	if errno != 0 {
		exit(errno)
	}

	return pid, nil
}

// forkExec1 does the actual forking and execing, it should have the smallest possible stack to not overflow
// because of the `nosplit`
//
//go:norace
//go:nosplit
func forkExec1(argv0 *byte, argv []*byte, envv []*byte, creds Creds, prog *syscall.SockFprog) (int, syscall.Errno) {
	syscall.ForkLock.Lock()

	// no more go runtime calls
	runtimeBeforeFork()

	pid, _, errno := syscall.RawSyscall6(syscall.SYS_CLONE, uintptr(syscall.SIGCHLD), 0, 0, 0, 0, 0)
	if errno != 0 || pid != 0 {
		// back to go runtime
		runtimeAfterFork()

		syscall.ForkLock.Unlock()
		return int(pid), errno
	}

	// in the child, no more go runtime calls
	runtimeAfterForkInChild()

	pid, _, errno = syscall.RawSyscall(syscall.SYS_GETPID, 0, 0, 0)
	if errno != 0 {
		return 0, errno
	}

	_, _, errno = syscall.RawSyscall(syscall.SYS_PTRACE, uintptr(syscall.PTRACE_TRACEME), 0, 0)
	if errno != 0 {
		return 0, errno
	}

	_, _, errno = syscall.RawSyscall6(syscall.SYS_PRCTL, unix.PR_SET_NO_NEW_PRIVS, 1, 0, 0, 0, 0)
	if errno != 0 {
		return 0, errno
	}

	const (
		mode  = 1
		tsync = 1
	)

	if prog != nil {
		_, _, errno = syscall.RawSyscall(unix.SYS_SECCOMP, mode, tsync, uintptr(unsafe.Pointer(prog)))
		if errno != 0 {
			return 0, errno
		}
	}

	_, _, errno = syscall.RawSyscall(syscall.SYS_KILL, pid, uintptr(syscall.SIGSTOP), 0)
	if errno != 0 {
		return 0, errno
	}

	if creds.GID != nil {
		_, _, errno = syscall.RawSyscall(syscall.SYS_SETGID, uintptr(*creds.GID), 0, 0)
		if errno != 0 {
			return 0, errno
		}

	}

	if creds.UID != nil {
		_, _, errno = syscall.RawSyscall(syscall.SYS_SETUID, uintptr(*creds.UID), 0, 0)
		if errno != 0 {
			return 0, errno
		}
	}

	_, _, errno = syscall.RawSyscall(syscall.SYS_EXECVE,
		uintptr(unsafe.Pointer(argv0)),
		uintptr(unsafe.Pointer(&argv[0])),
		uintptr(unsafe.Pointer(&envv[0])))

	return 0, errno
}

func exit(errno syscall.Errno) {
	_, _, _ = syscall.RawSyscall(syscall.SYS_EXIT, uintptr(errno), 0, 0)
}
