// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux || freebsd || openbsd || darwin

package procutil

import "syscall"

// PidExists returns true if the pid is still alive
func PidExists(pid int) bool {
	// the kill syscall will check for the existence of a process
	// if the signal is 0. See
	// https://man7.org/linux/man-pages/man2/kill.2.html
	if err := syscall.Kill(pid, 0); err == syscall.ESRCH {
		return false
	}

	return true
}
