// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux && arm64

package ptracer

import (
	"syscall"
)

const (
	OpenatNr         = 56  // OpenatNr defines the syscall ID for arm64
	Openat2Nr        = 437 // Openat2Nr defines the syscall ID for amd64
	NameToHandleAtNr = 264 // NameToHandleAtNr defines the syscall ID for amd64
	OpenByHandleAtNr = 265 // OpenByHandleAtNr defines the syscall ID for amd64
	ExecveNr         = 221 // ExecveNr defines the syscall ID for arm64
	ExecveatNr       = 281 // ExecveatNr defines the syscall ID for arm64
	CloneNr          = 220 // CloneNr defines the syscall ID for arm64
	ExitNr           = 93  // ExitNr defines the syscall ID for arm64
	FcntlNr          = 25  // FcntlNr defines the syscall ID for arm64
	DupNr            = 23  // DupNr defines the syscall ID for arm64
	Dup3Nr           = 24  // Dup3Nr defines the syscall ID for arm64
	ChdirNr          = 49  // ChdirNr defines the syscall ID for arm64
	FchdirNr         = 50  // FchdirNr defines the syscall ID for arm64
	SetuidNr         = 146 // SetuidNr defines the syscall ID for arm64
	SetgidNr         = 144 // SetgidNr defines the syscall ID for arm64
	SetreuidNr       = 145 // SetreuidNr defines the syscall ID for arm64
	SetregidNr       = 143 // SetregidNr defines the syscall ID for arm64
	SetresuidNr      = 147 // SetresuidNr defines the syscall ID for arm64
	SetresgidNr      = 149 // SetresgidNr defines the syscall ID for arm64
	SetfsuidNr       = 151 // SetfsuidNr defines the syscall ID for arm64
	SetfsgidNr       = 152 // SetfsgidNr defines the syscall ID for arm64
	CloseNr          = 57  // CloseNr defines the syscall ID for arm64
	MemfdCreateNr    = 279 // MemfdCreateNr defines the syscall ID for arm64
	CapsetNr         = 91  // CapsetNr defines the syscall ID for arm64
	UnlinkatNr       = 35  // UnlinkatNr defines the syscall ID for arm64
	RenameAtNr       = 38  // RenameAtNr defines the syscall ID for arm64
	RenameAt2Nr      = 276 // RenameAt2Nr defines the syscall ID for arm64
	Clone3Nr         = 435 // Clone3Nr defines the syscall ID for amd64

	OpenNr   = -1 // OpenNr not available on arm64
	ForkNr   = -2 // ForkNr not available on arm64
	VforkNr  = -3 // VforkNr not available on arm64
	Dup2Nr   = -4 // Dup2Nr not available on arm64
	CreatNr  = -5 // CreatNr not available on arm64
	UnlinkNr = -6 // UnlinkNr not available on arm64
	RmdirNr  = -7 // RmdirNr not available on arm64
	RenameNr = -8 // RenameNr not available on arm64
)

var (
	// PtracedSyscalls defines the list of syscall we want to ptrace
	PtracedSyscalls = []string{
		"openat",
		"openat2",
		"name_to_handle_at",
		"open_by_handle_at",
		"clone",
		"execve",
		"execveat",
		"exit",
		"fcntl",
		"dup",
		"dup3",
		"chdir",
		"fchdir",
		"setuid",
		"setgid",
		"setuid",
		"setgid",
		"setreuid",
		"setregid",
		"setresuid",
		"setresgid",
		"setfsuid",
		"setfsgid",
		"close",
		"memfd_create",
		"capset",
		"unlinkat",
		"renameat",
		"renameat2",
	}
)

func (t *Tracer) argToRegValue(regs syscall.PtraceRegs, arg int) uint64 {
	if arg >= 0 && arg <= 5 {
		return regs.Regs[arg]
	}
	return 0
}

// ReadRet reads and returns the return value
func (t *Tracer) ReadRet(regs syscall.PtraceRegs) int64 {
	return int64(regs.Regs[0])
}

// GetSyscallNr returns the given syscall number
func GetSyscallNr(regs syscall.PtraceRegs) int {
	return int(regs.Regs[8])
}
