// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build windows

package procmaps

import (
	"context"
	"errors"
	"fmt"
	"math"
	"path/filepath"

	"golang.org/x/sys/windows"
)

// memInfoBufSize is the buffer size passed to VirtualQueryEx. The real
// MEMORY_BASIC_INFORMATION struct is 40-48 bytes depending on architecture
// and Windows version; Windows only writes what it needs and accepts a
// larger buffer, so a fixed generous size avoids depending on unsafe.Sizeof.
const memInfoBufSize = 64

// maxWindowsPathUTF16 accommodates Windows' maximum extended-length path and
// avoids silently accepting GetModuleFileNameEx's truncated MAX_PATH result.
const maxWindowsPathUTF16 = 32_768

// Well-known MEMORY_BASIC_INFORMATION Type values (winnt.h). Not exposed as
// named constants by golang.org/x/sys/windows.
const (
	memImage  = 0x1000000
	memMapped = 0x40000
)

func readImpl(ctx context.Context, _ string, pid int, extended bool) (string, []Mapping, error) {
	// Windows has no per-region Rss/Dirty breakdown without a per-page
	// working-set walk (see the package doc comment); report clearly
	// rather than fabricate zeros.
	if extended {
		return "", nil, ErrExtendedNotSupported
	}
	if err := ctx.Err(); err != nil {
		return "", nil, err
	}

	if pid <= 0 || int64(pid) > int64(math.MaxUint32) {
		return "", nil, ErrNoSuchProcess
	}

	// PROCESS_VM_READ is required by GetModuleFileNameEx below even though
	// this backend never calls ReadProcessMemory itself.
	h, err := windows.OpenProcess(windows.PROCESS_QUERY_INFORMATION|windows.PROCESS_VM_READ, false, uint32(pid))
	if err != nil {
		if errors.Is(err, windows.ERROR_INVALID_PARAMETER) {
			return "", nil, ErrNoSuchProcess
		}
		return "", nil, fmt.Errorf("open process: %w", err)
	}
	defer windows.CloseHandle(h)

	name, err := mainModuleName(h)
	if err != nil {
		return "", nil, err
	}

	var mappings []Mapping
	var addr uintptr
	// No separate iteration cap is needed beyond the MaxMappings check
	// below: each iteration strictly advances addr to the end of the
	// region just read (rejecting non-advancing/overflowing regions as
	// ErrMalformedData above), so the loop is bounded by the process's
	// real address space and the executor's 30s timeout.
	for {
		if err := ctx.Err(); err != nil {
			return "", nil, err
		}
		var mbi windows.MemoryBasicInformation
		if err := windows.VirtualQueryEx(h, addr, &mbi, memInfoBufSize); err != nil {
			if errors.Is(err, windows.ERROR_INVALID_PARAMETER) && addr != 0 {
				break
			}
			return "", nil, fmt.Errorf("query process memory: %w", err)
		}
		if mbi.RegionSize == 0 {
			return "", nil, ErrMalformedData
		}
		if mbi.RegionSize > ^uintptr(0)-mbi.BaseAddress {
			return "", nil, ErrMalformedData
		}
		end := mbi.BaseAddress + mbi.RegionSize
		if end <= addr {
			return "", nil, ErrMalformedData
		}
		if mbi.State == windows.MEM_COMMIT {
			if len(mappings) >= MaxMappings {
				return "", nil, ErrMappingLimitExceeded
			}
			mappings = append(mappings, Mapping{
				Start: uint64(mbi.BaseAddress),
				End:   uint64(end),
				Perms: protectToMode(mbi.Protect),
				Name:  regionName(mbi.Type),
			})
		}
		addr = end
	}
	return name, mappings, nil
}

// mainModuleName returns the base file name of the process's main module
// (its executable), never the full command line.
func mainModuleName(h windows.Handle) (string, error) {
	buf := make([]uint16, maxWindowsPathUTF16)
	if err := windows.GetModuleFileNameEx(h, 0, &buf[0], uint32(len(buf))); err != nil {
		return "", fmt.Errorf("read process name: %w", err)
	}
	return filepath.Base(windows.UTF16ToString(buf)), nil
}

// protectToMode converts a PAGE_* protection constant into pmap's 5-char
// Mode column (read, write, execute, then two fixed dashes — Windows has
// no shared/private distinction analogous to Linux's maps 's'/'p' flag at
// this level of detail).
func protectToMode(protect uint32) string {
	base := protect &^ uint32(windows.PAGE_GUARD|windows.PAGE_NOCACHE|windows.PAGE_WRITECOMBINE)
	r, w, x := false, false, false
	switch base {
	case windows.PAGE_READONLY:
		r = true
	case windows.PAGE_READWRITE, windows.PAGE_WRITECOPY:
		r, w = true, true
	case windows.PAGE_EXECUTE:
		x = true
	case windows.PAGE_EXECUTE_READ:
		r, x = true, true
	case windows.PAGE_EXECUTE_READWRITE, windows.PAGE_EXECUTE_WRITECOPY:
		r, w, x = true, true, true
	}
	b := []byte("-----")
	if r {
		b[0] = 'r'
	}
	if w {
		b[1] = 'w'
	}
	if x {
		b[2] = 'x'
	}
	return string(b)
}

// regionName labels a committed region by its MEMORY_BASIC_INFORMATION
// Type, since Windows does not cheaply expose a per-region backing file
// path (that requires GetMappedFileNameW, which golang.org/x/sys/windows
// does not wrap) — see the package doc comment.
func regionName(typ uint32) string {
	switch typ {
	case memImage:
		return "[image]"
	case memMapped:
		return "[ mapped ]"
	default:
		return "[ anon ]"
	}
}
