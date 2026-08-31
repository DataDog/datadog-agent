// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build linux

package opener

import (
	"fmt"
	"os"

	"golang.org/x/sys/unix"
)

const maxDirectIOAlignment = 1024 * 1024

// allocDirectIOBuffer returns a MAP_SHARED buffer, freed with freeDirectIOBuffer. Go heap
// memory is forbidden: a fork (the Agent embeds CPython) corrupts an in-flight O_DIRECT read.
func allocDirectIOBuffer(size int) ([]byte, error) {
	buffer, err := unix.Mmap(-1, 0, size, unix.PROT_READ|unix.PROT_WRITE, unix.MAP_SHARED|unix.MAP_ANONYMOUS)
	if err != nil {
		return nil, fmt.Errorf("could not map a %d byte direct I/O buffer: %w", size, err)
	}
	return buffer, nil
}

func freeDirectIOBuffer(buffer []byte) {
	if len(buffer) == 0 {
		return
	}
	// munmap only rejects arguments this package controls, so a failure here
	// would be a bug rather than a runtime condition worth surfacing.
	_ = unix.Munmap(buffer)
}

// openDirect opens path read-only with O_DIRECT for the short-lived descriptor
// used to compute a fingerprint. Normal log tailing keeps its existing open mode.
func openDirect(path string) (*os.File, error) {
	return os.OpenFile(path, os.O_RDONLY|unix.O_DIRECT, 0)
}

// directIOAlignments returns the memory and offset alignment requirements for
// direct I/O on this descriptor. Linux 6.1 and newer filesystems may report
// them through statx. When that query is unavailable, the conservative 4 KiB
// block size is assumed. An explicit zero value means direct I/O is unsupported
// for this file and must not fall back to a buffered read.
func directIOAlignments(file *os.File) (int, int, error) {
	var stat unix.Statx_t
	if err := unix.Statx(int(file.Fd()), "", unix.AT_EMPTY_PATH|unix.AT_STATX_DONT_SYNC, unix.STATX_DIOALIGN, &stat); err != nil || stat.Mask&unix.STATX_DIOALIGN == 0 {
		return directIOAlignment, directIOAlignment, nil
	}

	memoryAlignment, err := validatedDirectIOAlignment(stat.Dio_mem_align)
	if err != nil {
		return 0, 0, fmt.Errorf("direct I/O for %q: invalid memory alignment %d: %w", file.Name(), stat.Dio_mem_align, err)
	}
	offsetAlignment, err := validatedDirectIOAlignment(stat.Dio_offset_align)
	if err != nil {
		return 0, 0, fmt.Errorf("direct I/O for %q: invalid offset alignment %d: %w", file.Name(), stat.Dio_offset_align, err)
	}
	return memoryAlignment, offsetAlignment, nil
}

func validatedDirectIOAlignment(value uint32) (int, error) {
	if value == 0 || value > maxDirectIOAlignment || value&(value-1) != 0 {
		return 0, fmt.Errorf("invalid direct I/O alignment %d", value)
	}
	return int(value), nil
}
