// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build !windows && functionaltests

package testutils

import "golang.org/x/sys/unix"

// SyscallExists checks if a syscall exists by syscall code
func SyscallExists(syscall int) bool {
	ret, _, err := unix.Syscall(uintptr(syscall), 0, 0, 0)
	if int(ret) == -1 && err == unix.ENOSYS {
		return false
	}
	return true
}
