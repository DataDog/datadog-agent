// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux && arm64

package ptracer

import "syscall"

func syscallTestRegs(nr int, arg0 uint64) syscall.PtraceRegs {
	var regs syscall.PtraceRegs
	regs.Regs[8] = uint64(nr)
	regs.Regs[0] = arg0
	return regs
}
