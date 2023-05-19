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
	// ptraceArchConstants are the supported ptrace commands for the ptrace syscall on arm
	// generate_constants:Ptrace constants,Ptrace constants are the supported ptrace commands for the ptrace syscall.
	ptraceArchConstants = map[string]uint32{
		"PTRACE_GETCRUNCHREGS":   unix.PTRACE_GETCRUNCHREGS,
		"PTRACE_GETFDPIC":        unix.PTRACE_GETFDPIC,
		"PTRACE_GETFDPIC_EXEC":   unix.PTRACE_GETFDPIC_EXEC,
		"PTRACE_GETFDPIC_INTERP": unix.PTRACE_GETFDPIC_INTERP,
		"PTRACE_GETFPREGS":       unix.PTRACE_GETFPREGS,
		"PTRACE_GETHBPREGS":      unix.PTRACE_GETHBPREGS,
		"PTRACE_GETVFPREGS":      unix.PTRACE_GETVFPREGS,
		"PTRACE_GETWMMXREGS":     unix.PTRACE_GETWMMXREGS,
		"PTRACE_GET_THREAD_AREA": unix.PTRACE_GET_THREAD_AREA,
		"PTRACE_OLDSETOPTIONS":   unix.PTRACE_OLDSETOPTIONS,
		"PTRACE_SETCRUNCHREGS":   unix.PTRACE_SETCRUNCHREGS,
		"PTRACE_SETFPREGS":       unix.PTRACE_SETFPREGS,
		"PTRACE_SETHBPREGS":      unix.PTRACE_SETHBPREGS,
		"PTRACE_SETVFPREGS":      unix.PTRACE_SETVFPREGS,
		"PTRACE_SETWMMXREGS":     unix.PTRACE_SETWMMXREGS,
		"PTRACE_SET_SYSCALL":     unix.PTRACE_SET_SYSCALL,
	}

	mmapFlagArchConstants = map[string]uint64{}
)
