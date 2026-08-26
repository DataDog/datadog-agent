// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build linux

package opener

import (
	"errors"
	"os"

	"golang.org/x/sys/unix"
)

const maxDirectIOAlignment = 1024 * 1024

func isOpenFlagsUnsupportedError(err error) bool {
	return errors.Is(err, unix.EINVAL) || errors.Is(err, unix.EOPNOTSUPP)
}

// openDirect opens path read-only with O_DIRECT for the short-lived descriptor
// used to compute a fingerprint. Normal log tailing keeps its existing open mode.
func openDirect(path string) (*os.File, error) {
	return os.OpenFile(path, os.O_RDONLY|unix.O_DIRECT, 0)
}

// directIOAlignments returns the memory and offset alignment requirements for
// direct I/O on this descriptor. Linux 6.1 and newer filesystems may report
// them through statx; where that query is unavailable or comes back empty, the
// conservative 4 KiB block size is assumed, which every filesystem the Agent is
// likely to read satisfies.
func directIOAlignments(file *os.File) (int, int) {
	if file == nil {
		return directIOAlignment, directIOAlignment
	}

	var stat unix.Statx_t
	if err := unix.Statx(int(file.Fd()), "", unix.AT_EMPTY_PATH|unix.AT_STATX_DONT_SYNC, unix.STATX_DIOALIGN, &stat); err != nil || stat.Mask&unix.STATX_DIOALIGN == 0 {
		return directIOAlignment, directIOAlignment
	}

	return validatedDirectIOAlignment(stat.Dio_mem_align), validatedDirectIOAlignment(stat.Dio_offset_align)
}

func validatedDirectIOAlignment(value uint32) int {
	if value == 0 || value > maxDirectIOAlignment || value&(value-1) != 0 {
		return directIOAlignment
	}
	return int(value)
}
