// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build unix

package read

import "golang.org/x/sys/unix"

// pollInputNonConsuming reports whether a Read on the given file
// descriptor would return without blocking — meaning either input
// is buffered (POLLIN) OR the peer has closed and Read would see
// EOF (POLLHUP). Both are treated as "available" because bash 5.2's
// `read -t 0` returns 0 in either case: data-ready and EOF-ready
// scripts can both legitimately rely on `-t 0` as a non-consuming
// readiness check, and bash treats EOF as an "I can complete the
// read without waiting" condition.
//
// Verified empirically against bash 5.2.0:
//   - printf with no output piped into `read -t 0` returns rc=0
//     (closed pipe, POLLHUP only, EOF-ready).
//   - A still-open producer that has not yet written data into the
//     pipe makes `read -t 0` return rc=1 (poll times out, neither
//     POLLIN nor POLLHUP is set).
//
// The supported return reports whether the platform implementation
// could perform the check; on platforms without a non-blocking poll
// syscall the caller must fall back to a consume-based probe.
func pollInputNonConsuming(fd uintptr) (available, supported bool) {
	fds := []unix.PollFd{{Fd: int32(fd), Events: unix.POLLIN}}
	n, err := unix.Poll(fds, 0)
	if err != nil {
		// EINTR or similar: report not-available rather than guessing.
		return false, true
	}
	if n == 0 {
		return false, true
	}
	return fds[0].Revents&(unix.POLLIN|unix.POLLHUP) != 0, true
}
