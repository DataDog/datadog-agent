// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package rssshrinker

import (
	"bufio"
	"os"
	"runtime/debug"
	"strconv"
	"strings"
	"syscall"
	"unsafe"
)

// MADV_PAGEOUT asks Linux to reclaim these pages.
//
//nolint:revive
const MADV_PAGEOUT = 21

// Shrink releases reclaimable memory to the OS on a best-effort basis.
func Shrink() {
	if isEnvEnabled(DisabledEnvVar) {
		return
	}

	// Release memory garbage collected by the Go runtime to Linux.
	debug.FreeOSMemory()

	// Optionally release C malloc arenas. This is disabled by default because it is
	// cgo/glibc-specific and is only intended for measuring incremental impact.
	mallocTrim()

	// Release clean file-backed memory to Linux. This is primarily intended to
	// improve RSS presentation for code/data touched during startup and no longer
	// actively used.
	pageOutFileBackedMemory()
}

// pageOutFileBackedMemory releases file-backed memory by advising Linux to page
// out clean read-only file mappings. It intentionally ignores per-mapping errors:
// older kernels may not support MADV_PAGEOUT, and individual VMAs can fail for
// reasons that should not affect process startup or runtime behavior.
func pageOutFileBackedMemory() {
	selfMap, err := os.Open("/proc/self/maps")
	if err != nil {
		return
	}
	defer selfMap.Close()

	scanner := bufio.NewScanner(selfMap)
	for scanner.Scan() {
		// Each line in /proc/self/maps has the following format:
		// address           perms offset  dev   inode       pathname
		// 00400000-0291f000 r--p 00000000 00:53 5009645     /opt/datadog-agent/bin/agent/agent
		// 0291f000-08038000 r-xp 0251f000 00:53 5009645     /opt/datadog-agent/bin/agent/agent
		// 08038000-0eb30000 r--p 07c38000 00:53 5009645     /opt/datadog-agent/bin/agent/agent
		// 0eb30000-0eb31000 r--p 0e730000 00:53 5009645     /opt/datadog-agent/bin/agent/agent
		// 0eb31000-0efed000 rw-p 0e731000 00:53 5009645     /opt/datadog-agent/bin/agent/agent
		fields := strings.Fields(scanner.Text())

		// If the 6th column is missing, the line is about an anonymous mapping.
		// We ignore it as we want to page out only file-backed memory.
		if len(fields) < 6 {
			continue
		}

		address, perms, pathname := fields[0], fields[1], strings.Join(fields[5:], " ")

		// Ignore pseudo-paths about stack, heap, vdso, named anonymous mapping, etc.
		if strings.HasPrefix(pathname, "[") {
			continue
		}

		// We only want to page out read-only memory.
		if len(perms) != 4 || perms[0] != 'r' || perms[1] != '-' {
			continue
		}

		beginStr, endStr, found := strings.Cut(address, "-")
		if !found {
			continue
		}

		begin, err := strconv.ParseUint(beginStr, 16, strconv.IntSize)
		if err != nil {
			continue
		}

		end, err := strconv.ParseUint(endStr, 16, strconv.IntSize)
		if err != nil {
			continue
		}

		if end <= begin {
			continue
		}

		// nolint:govet
		_ = syscall.Madvise(unsafe.Slice((*byte)(unsafe.Pointer(uintptr(begin))), uintptr(end-begin)), MADV_PAGEOUT)
	}
}
