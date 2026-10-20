// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build darwin

package vmstat

import (
	"context"

	"golang.org/x/sys/unix"
)

// readImpl assembles Stats on macOS from sysctl(3) only — the same darwin
// toolset the ss and df builtins already use (golang.org/x/sys/unix,
// SysctlRaw + manual byte decoding). No Mach host_statistics64 call is
// made: it would be a new, undocumented syscall surface, and no existing
// rshell builtin uses it. Per-page memory breakdown (active/inactive/
// wired), CPU tick counters, and paging/interrupt/context-switch rates are
// therefore unavailable on macOS; Partial reflects that.
//
// procPath is unused on darwin.
func readImpl(ctx context.Context, _ string) (Stats, error) {
	if err := ctx.Err(); err != nil {
		return Stats{}, err
	}
	var st Stats
	st.PageSize = uint64(unix.Getpagesize())

	memOK := readMemory(&st)
	if err := ctx.Err(); err != nil {
		return Stats{}, err
	}
	swapOK := readSwap(&st)
	if err := ctx.Err(); err != nil {
		return Stats{}, err
	}
	loadOK := readLoadAvg(&st)
	if err := ctx.Err(); err != nil {
		return Stats{}, err
	}

	if memOK {
		st.Partial |= FieldMemory
	}
	if swapOK {
		st.Partial |= FieldSwap
	}
	if loadOK {
		st.Partial |= FieldLoadAvg
	}
	if st.Partial == 0 {
		return Stats{}, ErrNotSupported
	}
	return st, nil
}

// readMemory populates MemTotal via hw.memsize. macOS has no sysctl
// exposing system-wide free/buffers/cached without Mach host_statistics64
// (see package doc), so MemFree/MemBuffers/MemCached/MemActive/MemInactive
// stay zero.
func readMemory(st *Stats) bool {
	total, err := unix.SysctlUint64("hw.memsize")
	if err != nil {
		return false
	}
	st.MemTotal = total
	return true
}

// xswUsageSize is sizeof(struct xsw_usage) from <sys/sysctl.h>:
//
//	struct xsw_usage {
//	    u_int64_t xsu_total;
//	    u_int64_t xsu_avail;
//	    u_int64_t xsu_used;
//	    u_int32_t xsu_pagesize;
//	    boolean_t xsu_encrypted; // 4-byte int
//	};
const xswUsageSize = 8 + 8 + 8 + 4 + 4

// readSwap populates SwapTotal/SwapFree via vm.swapusage.
func readSwap(st *Stats) bool {
	data, err := unix.SysctlRaw("vm.swapusage")
	if err != nil {
		return false
	}
	return decodeSwapUsage(data, st)
}

// decodeSwapUsage decodes a raw vm.swapusage sysctl reply (struct
// xsw_usage) into st. Split out from readSwap so tests can exercise the
// byte layout with a synthetic buffer instead of live sysctl output.
func decodeSwapUsage(data []byte, st *Stats) bool {
	if len(data) < xswUsageSize {
		return false
	}
	total := readU64LE(data, 0)
	available := readU64LE(data, 8)
	used := readU64LE(data, 16)
	if available > total || used > total {
		return false
	}
	st.SwapTotal = total
	st.SwapFree = available
	return true
}

// loadavgStructSize is sizeof(struct loadavg) from <sys/resource.h>:
//
//	struct loadavg {
//	    fixpt_t ldavg[3]; // u_int32_t, fixed-point scaled by fscale
//	    long    fscale;   // 8 bytes on 64-bit darwin, at offset 16 after
//	                      // 4 bytes of padding to satisfy long's 8-byte
//	                      // alignment
//	};
const loadavgStructSize = 24

// readLoadAvg populates LoadAvg1/5/15 via vm.loadavg.
func readLoadAvg(st *Stats) bool {
	data, err := unix.SysctlRaw("vm.loadavg")
	if err != nil {
		return false
	}
	return decodeLoadAvg(data, st)
}

// decodeLoadAvg decodes a raw vm.loadavg sysctl reply (struct loadavg)
// into st. Split out from readLoadAvg so tests can exercise the byte
// layout with a synthetic buffer instead of live sysctl output.
func decodeLoadAvg(data []byte, st *Stats) bool {
	if len(data) < loadavgStructSize {
		return false
	}
	fscale := readU64LE(data, 16)
	if fscale == 0 {
		return false
	}
	l1 := readU32LE(data, 0)
	l5 := readU32LE(data, 4)
	l15 := readU32LE(data, 8)
	st.LoadAvg1 = float64(l1) / float64(fscale)
	st.LoadAvg5 = float64(l5) / float64(fscale)
	st.LoadAvg15 = float64(l15) / float64(fscale)
	return true
}

// readU32LE reads a little-endian uint32 from data at offset off. Returns 0
// if the read would run past the end of data.
func readU32LE(data []byte, off int) uint32 {
	if off+4 > len(data) {
		return 0
	}
	return uint32(data[off]) |
		uint32(data[off+1])<<8 |
		uint32(data[off+2])<<16 |
		uint32(data[off+3])<<24
}

// readU64LE reads a little-endian uint64 from data at offset off. Returns 0
// if the read would run past the end of data.
func readU64LE(data []byte, off int) uint64 {
	if off+8 > len(data) {
		return 0
	}
	return uint64(readU32LE(data, off)) | uint64(readU32LE(data, off+4))<<32
}
