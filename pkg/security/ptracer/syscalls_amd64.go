// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux && amd64

package ptracer

import (
	"syscall"

	"golang.org/x/sys/unix"
)

const (
	OpenNr     = 2   // OpenNr defines the syscall ID for amd64
	OpenatNr   = 257 // OpenatNr defines the syscall ID for amd64
	Openat2Nr  = 437 // Openat2Nr defines the syscall ID for amd64
	ExecveNr   = 59  // ExecveNr defines the syscall ID for amd64
	ExecveatNr = 322 // ExecveatNr defines the syscall ID for amd64
	CloneNr    = 56  // CloneNr defines the syscall ID for amd64
	ForkNr     = 57  // ForkNr defines the syscall ID for amd64
	VforkNr    = 58  // VforkNr defines the syscall ID for amd64
	ExitNr     = 60  // ExitNr defines the syscall ID for amd64
	FcntlNr    = 72  // FcntlNr defines the syscall ID for amd64
	DupNr      = 32  // DupNr defines the syscall ID for amd64
	Dup2Nr     = 33  // Dup2Nr defines the syscall ID for amd64
	Dup3Nr     = 292 // Dup3Nr defines the syscall ID for amd64
	ChdirNr    = 80  // ChdirNr defines the syscall ID for amd64
	FchdirNr   = 81  // FchdirNr defines the syscall ID for amd64
	SetuidNr   = 105 // SetuidNr defines the syscall ID for amd64
	SetgidNr   = 106 // SetgidNr defines the syscall ID for amd64
	SetreuidNr = 113 // SetreuidNr defines the syscall ID for amd64
	SetregidNr = 114 // SetregidNr defines the syscall ID for amd64

	ptraceFlags = 0 |
		syscall.PTRACE_O_TRACEVFORK |
		syscall.PTRACE_O_TRACEFORK |
		syscall.PTRACE_O_TRACECLONE |
		syscall.PTRACE_O_TRACEEXEC |
		syscall.PTRACE_O_TRACESYSGOOD |
		unix.PTRACE_O_TRACESECCOMP
)

var (
	// PtracedSyscalls defines the list of syscall we want to ptrace
	PtracedSyscalls = []string{
		"open",
		"openat",
		"openat2",
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
