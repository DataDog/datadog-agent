// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build darwin

package procmaps

import (
	"context"
	"encoding/binary"
	"fmt"
	"syscall"
	"unsafe"

	"golang.org/x/sys/unix"
)

// procInfoCallPidInfo and procPidRegionInfo select proc_pidinfo's
// PROC_PIDREGIONINFO flavor via the raw SYS_PROC_INFO trap. Neither the
// call number nor the flavor is exposed by golang.org/x/sys/unix, which
// wraps only BSD syscalls — this is XNU's Mach/BSD-hybrid libproc
// interface, reached the same way Darwin's libproc.h does internally.
const (
	procInfoCallPidInfo = 0x2
	procPidRegionInfo   = 7
)

// procRegionInfoSize is sizeof(struct proc_regioninfo) on Darwin: 20
// leading uint32/uint64 fields (protection through depth) followed by two
// uint64 fields (address, size), verified empirically against a live
// process on this platform — the kernel packs the trailing uint64 pair
// immediately after the last uint32 field with no alignment padding.
const procRegionInfoSize = 96

// procRegionAddrOff and procRegionSizeOff are proc_regioninfo's
// pri_address/pri_size byte offsets, verified empirically (see
// procRegionInfoSize).
const (
	procRegionProtOff = 0
	procRegionAddrOff = 80
	procRegionSizeOff = 88
)

func readImpl(ctx context.Context, _ string, pid int, extended bool) (string, []Mapping, error) {
	// proc_regioninfo reports resident/dirty counts for a region's whole
	// shadow chain, not the private Rss/Dirty split pmap's extended
	// columns expect (see the package doc comment); report clearly
	// rather than misrepresent the numbers.
	if extended {
		return "", nil, ErrExtendedNotSupported
	}
	if err := ctx.Err(); err != nil {
		return "", nil, err
	}
	if pid <= 0 {
		return "", nil, ErrNoSuchProcess
	}

	kp, err := unix.SysctlKinfoProc("kern.proc.pid", pid)
	if err != nil {
		return "", nil, ErrNoSuchProcess
	}
	name := darwinCommName(kp.Proc.P_comm[:])

	var mappings []Mapping
	var addr uint64
	buf := make([]byte, procRegionInfoSize)
	for {
		if err := ctx.Err(); err != nil {
			return "", nil, err
		}
		n, _, errno := syscall.Syscall6(
			uintptr(syscall.SYS_PROC_INFO),
			procInfoCallPidInfo,
			uintptr(pid),
			procPidRegionInfo,
			uintptr(addr),
			uintptr(unsafe.Pointer(&buf[0])),
			uintptr(len(buf)),
		)
		if errno != 0 {
			// The kernel rejects the very first call with EPERM when the
			// caller lacks privilege to inspect pid (e.g. a root-owned
			// process from a non-root caller) — surface that distinctly
			// rather than reporting a privileged process as having zero
			// mappings. Once the walk is underway, any error (typically
			// EINVAL) means the address has run past the last region,
			// which is the normal end-of-walk signal.
			if addr == 0 && errno == syscall.EPERM {
				return "", nil, fmt.Errorf("read process memory regions: %w", errno)
			}
			break
		}
		if n == 0 {
			break
		}
		if n < procRegionInfoSize {
			return "", nil, ErrMalformedData
		}

		protection := binary.LittleEndian.Uint32(buf[procRegionProtOff : procRegionProtOff+4])
		start := binary.LittleEndian.Uint64(buf[procRegionAddrOff : procRegionAddrOff+8])
		size := binary.LittleEndian.Uint64(buf[procRegionSizeOff : procRegionSizeOff+8])
		if size > ^uint64(0)-start {
			return "", nil, ErrMalformedData
		}
		end := start + size
		if end <= addr {
			return "", nil, ErrMalformedData
		}

		if len(mappings) >= MaxMappings {
			return "", nil, ErrMappingLimitExceeded
		}
		mappings = append(mappings, Mapping{
			Start: start,
			End:   end,
			Perms: darwinProtToMode(protection),
			Name:  "[ anon ]",
		})
		addr = end
	}
	return name, mappings, nil
}

// darwinCommName truncates a fixed-size comm buffer at its first NUL byte,
// mirroring builtins/internal/procinfo/procinfo_darwin.go's helper of the
// same name (duplicated locally rather than shared to keep procmaps
// self-contained; both packages read the same kernel field).
func darwinCommName(comm []byte) string {
	n := 0
	for n < len(comm) && comm[n] != 0 {
		n++
	}
	return string(comm[:n])
}

// darwinProtToMode converts a proc_regioninfo pri_protection bitmask
// (VM_PROT_READ=1, VM_PROT_WRITE=2, VM_PROT_EXECUTE=4) into pmap's 5-char
// Mode column. macOS has no shared/private distinction analogous to
// Linux's maps 's'/'p' flag at this level of detail, so the last two
// columns stay fixed dashes, matching the Windows backend's convention.
func darwinProtToMode(protection uint32) string {
	b := []byte("-----")
	if protection&0x1 != 0 {
		b[0] = 'r'
	}
	if protection&0x2 != 0 {
		b[1] = 'w'
	}
	if protection&0x4 != 0 {
		b[2] = 'x'
	}
	return string(b)
}
