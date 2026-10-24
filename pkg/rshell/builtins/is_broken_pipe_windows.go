// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build windows

package builtins

import (
	"errors"
	"syscall"
)

// windowsBrokenPipeErrnos are the Windows error codes os.Pipe's write side
// returns once the read end has closed. Go's own os.Pipe implementation and
// tests treat both as the broken-pipe condition (see os/pipe_test.go):
// ERROR_BROKEN_PIPE (109) is the classic CreatePipe/named-pipe case, and
// ERROR_NO_DATA (232) is what Go's anonymous os.Pipe commonly surfaces once
// the reader is gone. Referenced as bare syscall.Errno values (not
// golang.org/x/sys/windows constants) to avoid an extra dependency for two
// numeric checks.
//
// This file is windows-only: on Linux, errno 109 is ETOOMANYREFS ("too many
// references: can't splice") — a real, unrelated error — so these numeric
// codes must never be checked outside a build actually targeting Windows.
const (
	errnoBrokenPipeWindows = syscall.Errno(109) // ERROR_BROKEN_PIPE
	errnoNoDataWindows     = syscall.Errno(232) // ERROR_NO_DATA ("The pipe is being closed")
)

// isBrokenPipePlatform checks the Windows-specific broken-pipe errno values
// that have no equivalent on Unix (there is no EPIPE on Windows).
func isBrokenPipePlatform(err error) bool {
	return errors.Is(err, errnoBrokenPipeWindows) || errors.Is(err, errnoNoDataWindows)
}
