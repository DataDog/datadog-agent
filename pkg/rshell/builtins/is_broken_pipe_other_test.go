// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build !windows

package builtins

import (
	"syscall"
	"testing"
)

// TestIsBrokenPipeDoesNotMisclassifyLinuxErrno109 is the regression test
// for the bug this platform split fixes: errno 109 is ETOOMANYREFS on
// Linux ("too many references: can't splice"), a real and unrelated
// error, not a Windows ERROR_BROKEN_PIPE. Checking that numeric value
// unconditionally (as an earlier version of IsBrokenPipe did) would have
// let a genuine ETOOMANYREFS write failure be silently misreported as a
// closed pipe on Linux.
func TestIsBrokenPipeDoesNotMisclassifyLinuxErrno109(t *testing.T) {
	if IsBrokenPipe(syscall.Errno(109)) {
		t.Error("IsBrokenPipe must not treat errno 109 (ETOOMANYREFS on Linux) as a broken pipe outside Windows")
	}
	if IsBrokenPipe(syscall.Errno(232)) {
		t.Error("IsBrokenPipe must not treat errno 232 as a broken pipe outside Windows")
	}
}
