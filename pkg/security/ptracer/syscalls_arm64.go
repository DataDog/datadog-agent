// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux && arm64

package ptracer

import (
	"syscall"

	"golang.org/x/sys/unix"
)

const (
	OpenatNr   = 56  // OpenatNr defines the syscall ID for arm64
	Openat2Nr  = 437 // Openat2Nr defines the syscall ID for amd64
	ExecveNr   = 221 // ExecveNr defines the syscall ID for arm64
	ExecveatNr = 281 // ExecveatNr defines the syscall ID for arm64
	CloneNr    = 220 // CloneNr defines the syscall ID for arm64
	ExitNr     = 93  // ExitNr defines the syscall ID for arm64
	FcntlNr    = 25  // FcntlNr defines the syscall ID for arm64
	DupNr      = 23  // DupNr defines the syscall ID for amd64
	Dup3Nr     = 24  // Dup3Nr defines the syscall ID for amd64
	ChdirNr    = 49  // ChdirNr defines the syscall ID for amd64
	FchdirNr   = 50  // FchdirNr defines the syscall ID for amd64

	OpenNr  = 9990 // OpenNr not available on arm64
	ForkNr  = 9991 // ForkNr not available on arm64
	VforkNr = 9992 // VforkNr not available on arm64
	Dup2Nr  = 9993 // Dup2Nr not available on arm64

	ptraceFlags = 0 |
		syscall.PTRACE_O_TRACECLONE |
		syscall.PTRACE_O_TRACEEXEC |
		syscall.PTRACE_O_TRACESYSGOOD |
		unix.PTRACE_O_TRACESECCOMP
)

var (
	// PtracedSyscalls defines the list of syscall we want to ptrace
	PtracedSyscalls = []string{
		"openat",
		"openat2",
		"clone",
		"execve",
		"execveat",
		"exit",
		"fcntl",
		"dup",
		"dup3",
		"chdir",
		"fchdir",
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
