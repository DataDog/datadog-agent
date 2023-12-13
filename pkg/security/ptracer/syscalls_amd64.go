// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux && amd64

package ptracer

import (
	"syscall"

	"github.com/DataDog/datadog-agent/pkg/security/secl/model/syscalls"
	"golang.org/x/sys/unix"
)

const (
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
func GetSyscallNr(regs syscall.PtraceRegs) syscalls.Syscall {
	return syscalls.Syscall(regs.Orig_rax)
}
