// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build linux

package dummymodeimpl

import "syscall"

// dummyProcAttr asks the kernel to kill ADP if the Agent dies. Everything else that stops ADP
// is best-effort Go code that does not run if the Agent is SIGKILLed or OOM-killed, in which
// case the child would be reparented to init and left holding a socket forever.
func dummyProcAttr() *syscall.SysProcAttr {
	return &syscall.SysProcAttr{Pdeathsig: syscall.SIGKILL}
}
