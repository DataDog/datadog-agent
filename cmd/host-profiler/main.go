// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux

// host-profiler is a standalone binary that runs the Host Profiler.
package main

import (
	"fmt"
	"os"
	"syscall"
	"unsafe"

	"golang.org/x/sys/unix"

	"github.com/DataDog/datadog-agent/cmd/host-profiler/command"
	"github.com/DataDog/datadog-agent/cmd/internal/runcmd"
	"github.com/DataDog/datadog-agent/pkg/util/flavor"
	_ "github.com/DataDog/datadog-agent/pkg/version"
)

type linuxCapHeader struct {
	version uint32
	pid     int32
}

type linuxCapData struct {
	effective   uint32
	permitted   uint32
	inheritable uint32
}

// raiseAmbient raises cap into the ambient set so child processes inherit it in their effective set.
// PR_CAP_AMBIENT_RAISE requires the cap to be in both permitted and inheritable; we set inheritable
// via capset(2) first (allowed as long as the cap is already in the permitted set).
func raiseAmbient(cap uintptr) error {
	const version3 = 0x20080522
	hdr := linuxCapHeader{version: version3}
	var data [2]linuxCapData
	if _, _, e := syscall.RawSyscall(syscall.SYS_CAPGET, uintptr(unsafe.Pointer(&hdr)), uintptr(unsafe.Pointer(&data[0])), 0); e != 0 {
		return e
	}
	if cap < 32 {
		data[0].inheritable |= 1 << cap
	} else {
		data[1].inheritable |= 1 << (cap - 32)
	}
	if _, _, e := syscall.RawSyscall(syscall.SYS_CAPSET, uintptr(unsafe.Pointer(&hdr)), uintptr(unsafe.Pointer(&data[0])), 0); e != 0 {
		return e
	}
	return unix.Prctl(unix.PR_CAP_AMBIENT, unix.PR_CAP_AMBIENT_RAISE, cap, 0, 0)
}

func main() {
	// Pass CAP_SYS_PTRACE to child processes (e.g. objcopy) via the ambient set so they can
	// access target binaries through /proc/<pid>/fd/<n>. No-ops silently when running as root
	// (children already inherit caps) or when the cap is not in the permitted set (no file caps).
	_ = raiseAmbient(unix.CAP_SYS_PTRACE)

	// File capabilities cause the kernel to set dumpable=0, which would prevent child
	// processes (e.g. objcopy) from opening /proc/<self>/fd/<n> even with CAP_SYS_PTRACE.
	// Explicitly restore dumpable=1 so our fds remain accessible to children.
	if err := unix.Prctl(unix.PR_SET_DUMPABLE, 1, 0, 0, 0); err != nil {
		fmt.Fprintf(os.Stderr, "failed to set PR_SET_DUMPABLE: %v\n", err)
		os.Exit(1)
	}

	// Prevent this process and all children from gaining new privileges via
	// setuid binaries or file capabilities. Inherited across fork/exec.
	if err := unix.Prctl(unix.PR_SET_NO_NEW_PRIVS, 1, 0, 0, 0); err != nil {
		fmt.Fprintf(os.Stderr, "failed to set PR_SET_NO_NEW_PRIVS: %v\n", err)
		os.Exit(1)
	}

	flavor.SetFlavor(flavor.HostProfiler)
	os.Exit(runcmd.Run(command.MakeRootCommand()))
}
