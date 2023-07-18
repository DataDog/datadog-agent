// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux

package model

import (
	"golang.org/x/sys/unix"
)

var (
	// ptraceArchConstants are the supported ptrace commands for the ptrace syscall on amd64
	// generate_constants:Ptrace constants,Ptrace constants are the supported ptrace commands for the ptrace syscall.
	ptraceArchConstants = map[string]uint32{
		"PTRACE_GETFPREGS":         unix.PTRACE_GETFPREGS,
		"PTRACE_SETFPREGS":         unix.PTRACE_SETFPREGS,
		"PTRACE_GETFPXREGS":        unix.PTRACE_GETFPXREGS,
		"PTRACE_SETFPXREGS":        unix.PTRACE_SETFPXREGS,
		"PTRACE_OLDSETOPTIONS":     unix.PTRACE_OLDSETOPTIONS,
		"PTRACE_GET_THREAD_AREA":   unix.PTRACE_GET_THREAD_AREA,
		"PTRACE_SET_THREAD_AREA":   unix.PTRACE_SET_THREAD_AREA,
		"PTRACE_ARCH_PRCTL":        unix.PTRACE_ARCH_PRCTL,
		"PTRACE_SYSEMU":            unix.PTRACE_SYSEMU,
		"PTRACE_SYSEMU_SINGLESTEP": unix.PTRACE_SYSEMU_SINGLESTEP,
		"PTRACE_SINGLEBLOCK":       unix.PTRACE_SINGLEBLOCK,
	}

	// mmapFlagConstants are the supported flags for the mmap syscall on amd64
	// generate_constants:MMap flags,MMap flags are the supported flags for the mmap syscall.
	mmapFlagArchConstants = map[string]uint64{
		"MAP_32BIT": unix.MAP_32BIT, /* only give out 32bit addresses */
	}
)
