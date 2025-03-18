// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux && !cgo

package main

import (
	"syscall"
	"unsafe"

	"golang.org/x/sys/unix"
)

func setProcessName(process string) error {
	processName := make([]byte, len(process)+1)
	copy(processName, process)
	_, _, err := syscall.AllThreadsSyscall(unix.SYS_PRCTL, unix.PR_SET_NAME, uintptr(unsafe.Pointer(&processName[0])), 0)
	return err
}
