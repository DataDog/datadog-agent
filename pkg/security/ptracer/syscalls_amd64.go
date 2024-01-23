// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux && amd64

package ptracer

import (
	"syscall"
)

const (
	OpenNr           = 2   // OpenNr defines the syscall ID for amd64
	OpenatNr         = 257 // OpenatNr defines the syscall ID for amd64
	Openat2Nr        = 437 // Openat2Nr defines the syscall ID for amd64
	CreatNr          = 85  // CreatNr defines the syscall ID for amd64
	NameToHandleAtNr = 303 // NameToHandleAtNr defines the syscall ID for amd64
	OpenByHandleAtNr = 304 // OpenByHandleAtNr defines the syscall ID for amd64
	ExecveNr         = 59  // ExecveNr defines the syscall ID for amd64
	ExecveatNr       = 322 // ExecveatNr defines the syscall ID for amd64
	CloneNr          = 56  // CloneNr defines the syscall ID for amd64
	ForkNr           = 57  // ForkNr defines the syscall ID for amd64
	VforkNr          = 58  // VforkNr defines the syscall ID for amd64
	ExitNr           = 60  // ExitNr defines the syscall ID for amd64
	FcntlNr          = 72  // FcntlNr defines the syscall ID for amd64
	DupNr            = 32  // DupNr defines the syscall ID for amd64
	Dup2Nr           = 33  // Dup2Nr defines the syscall ID for amd64
	Dup3Nr           = 292 // Dup3Nr defines the syscall ID for amd64
	ChdirNr          = 80  // ChdirNr defines the syscall ID for amd64
	FchdirNr         = 81  // FchdirNr defines the syscall ID for amd64
	SetuidNr         = 105 // SetuidNr defines the syscall ID for amd64
	SetgidNr         = 106 // SetgidNr defines the syscall ID for amd64
	SetreuidNr       = 113 // SetreuidNr defines the syscall ID for amd64
	SetregidNr       = 114 // SetregidNr defines the syscall ID for amd64
	SetresuidNr      = 117 // SetresuidNr defines the syscall ID for amd64
	SetresgidNr      = 119 // SetresgidNr defines the syscall ID for amd64
	SetfsuidNr       = 122 // SetfsuidNr defines the syscall ID for amd64
	SetfsgidNr       = 123 // SetfsgidNr defines the syscall ID for amd64
	CloseNr          = 3   // CloseNr defines the syscall ID for amd64
	MemfdCreateNr    = 319 // MemfdCreateNr defines the syscall ID for amd64
	CapsetNr         = 126 // CapsetNr defines the syscall ID for amd64
	UnlinkNr         = 87  // UnlinkNr defines the syscall ID for amd64
	UnlinkatNr       = 263 // UnlinkatNr defines the syscall ID for amd64
	RmdirNr          = 84  // RmdirNr defines the syscall ID for amd64
	RenameNr         = 82  // RenameNr defines the syscall ID for amd64
	RenameAtNr       = 264 // RenameAtNr defines the syscall ID for amd64
	RenameAt2Nr      = 316 // RenameAt2Nr defines the syscall ID for amd64
	Clone3Nr         = 435 // Clone3Nr defines the syscall ID for amd64
)

var (
	// PtracedSyscalls defines the list of syscall we want to ptrace
	PtracedSyscalls = []string{
		"open",
		"openat",
		"openat2",
		"creat",
		"name_to_handle_at",
		"open_by_handle_at",
		"fork",
		"vfork",
		"clone",
		"execve",
		"execveat",
		"exit",
		"fcntl",
		"dup",
		"dup2",
		"dup3",
		"chdir",
		"fchdir",
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
		"unlink",
		"unlinkat",
		"rmdir",
		"rename",
		"renameat",
		"renameat2",
	}
)

// https://github.com/torvalds/linux/blob/v5.0/arch/x86/entry/entry_64.S#L126
func (t *Tracer) argToRegValue(regs syscall.PtraceRegs, arg int) uint64 {
	switch arg {
	case 0:
		return regs.Rdi
	case 1:
		return regs.Rsi
	case 2:
		return regs.Rdx
	case 3:
		return regs.R10
	case 4:
		return regs.R8
	case 5:
		return regs.R9
	}

	return 0
}

// ReadRet reads and returns the return value
func (t *Tracer) ReadRet(regs syscall.PtraceRegs) int64 {
	return int64(regs.Rax)
}

// GetSyscallNr returns the given syscall number
func GetSyscallNr(regs syscall.PtraceRegs) int {
	return int(regs.Orig_rax)
}
