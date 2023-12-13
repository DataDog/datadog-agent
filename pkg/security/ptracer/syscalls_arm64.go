// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux && arm64

package ptracer

import (
	"syscall"

	"github.com/DataDog/datadog-agent/pkg/security/secl/model/syscalls"
	"golang.org/x/sys/unix"
)

const (
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
func GetSyscallNr(regs syscall.PtraceRegs) syscalls.Syscall {
	return syscalls.Syscall(regs.Regs[8])
}
