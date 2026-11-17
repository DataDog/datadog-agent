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

// threadPointer returns TPIDR_EL0, which is not in regs.
func threadPointer(tid int, _ *syscall.PtraceRegs) (uint64, error) {
	// NT_ARM_TLS only fills the first word of the buffer
	var tls unix.PtraceRegsArm64
	if err := unix.PtraceGetRegSetArm64(tid, unix.NT_ARM_TLS, &tls); err != nil {
		return 0, err
	}
	return tls.Regs[0], nil
}
