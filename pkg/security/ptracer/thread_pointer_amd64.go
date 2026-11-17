// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux && amd64

package ptracer

import (
	"syscall"
)

// threadPointer returns %fs, which PTRACE_GETREGS already fetched.
func threadPointer(_ int, regs *syscall.PtraceRegs) (uint64, error) {
	return regs.Fs_base, nil
}
