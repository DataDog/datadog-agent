// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

// Package procmaps reads per-process virtual memory mappings for the pmap
// builtin and presents them as a normalised cross-platform slice of Mapping.
//
// This package lives under builtins/internal/ and is therefore exempt from
// the builtinAllowedSymbols allowlist check. It may use OS-specific APIs
// freely.
//
// # Sandbox bypass
//
// The Linux backend reads <ProcPath>/<pid>/maps and smaps via os.Open
// directly. ProcPath is fixed by the embedding application, and the
// remaining path is derived only from the numeric PID, which selects a
// kernel-managed pseudo-file rather than an arbitrary script-supplied path.
// This matches the documented exception used by the ss, ip route, df, free,
// and ps builtins: AllowedPaths restrictions do not apply to these trusted
// kernel pseudo-filesystem reads. Like ps, this backend trusts ProcPath to
// be a genuine procfs and does not verify that comm/maps/smaps are not
// symlinks before opening them.
//
// No process argv or environment data is read. The process "header" name
// reported alongside mappings is the short comm/executable name (from
// /proc/<pid>/comm on Linux, or the main module's base file name on
// Windows), never the full command line — matching the same
// argv-exposure restriction the ps builtin already enforces.
//
// # Platform support
//
// Linux reads /proc/<pid>/maps (basic) or /proc/<pid>/smaps (extended, for
// per-mapping Rss/Dirty). Windows enumerates the process's committed
// virtual memory regions with VirtualQueryEx and labels each region by its
// MEMORY_BASIC_INFORMATION type ("[image]", "[ mapped ]", "[ anon ]")
// rather than a resolved file path — per-region file paths require
// GetMappedFileNameW, which golang.org/x/sys/windows does not wrap, and
// per-region Rss/Dirty needs a per-page working-set walk that neither
// package exposes; Read returns ErrExtendedNotSupported for extended mode
// on Windows rather than reporting fabricated zeros.
//
// macOS enumerates regions via the proc_pidinfo(PROC_PIDREGIONINFO) kernel
// call, reached through the raw syscall.SYS_PROC_INFO trap (golang.org/x/sys/unix
// wraps only BSD syscalls, not this Mach/BSD-hybrid libproc interface). The
// short process name comes from golang.org/x/sys/unix.SysctlKinfoProc, the
// same primitive builtins/internal/procinfo already uses for ps on darwin.
// Extended mode (-x) returns ErrExtendedNotSupported: proc_regioninfo
// reports resident/dirty page counts for the whole region's shadow chain,
// not the current snapshot's private Rss/Dirty split that Linux's smaps
// and pmap's extended columns expect, so reporting it would misrepresent
// the numbers rather than merely omit them.
package procmaps

import (
	"context"
	"errors"
)

// MaxMappings bounds the number of mappings returned for a single process,
// preventing unbounded memory growth from a pathological or adversarial
// mapping count.
const MaxMappings = 16_384

// Mapping describes a single virtual memory region.
type Mapping struct {
	// Start and End are the region's inclusive-start/exclusive-end virtual
	// addresses.
	Start, End uint64

	// Perms is a short permission string, e.g. "r-x-" (read, no-write,
	// execute, no-private-copy-on-write marker) — Linux reports it
	// directly from /proc/pid/maps; Windows synthesizes it from the
	// region's PAGE_* protection constant.
	Perms string

	// Name is the mapping's label: a file base name, a bracketed special
	// name such as "[heap]"/"[stack]", or "[ anon ]" for anonymous
	// private memory with no backing file.
	Name string

	// HasRSS reports whether RSS and Dirty are populated (extended mode
	// on a platform that supports it).
	HasRSS  bool
	RSSKB   uint64
	DirtyKB uint64
}

// SizeKB returns the mapping's size in kibibytes.
func (m Mapping) SizeKB() uint64 {
	if m.End <= m.Start {
		return 0
	}
	return (m.End - m.Start) / 1024
}

// ErrNotSupported is returned by Read on platforms without a backend.
var ErrNotSupported = errors.New("not supported on this platform")

// ErrExtendedNotSupported is returned by Read when extended is true on a
// platform that has a basic backend but cannot report per-mapping Rss/Dirty.
var ErrExtendedNotSupported = errors.New("extended mode not supported on this platform")

// ErrNoSuchProcess is returned by Read when pid does not name a running
// process.
var ErrNoSuchProcess = errors.New("no such process")

// ErrMappingLimitExceeded is returned when a process has more mappings than
// MaxMappings. Returning an error instead of a silently truncated slice keeps
// pmap from presenting incomplete output as a successful snapshot.
var ErrMappingLimitExceeded = errors.New("mapping limit exceeded")

// ErrMalformedData is returned when a proc maps backend yields structurally
// invalid data. This normally indicates a misconfigured ProcPath rather than a
// real kernel proc filesystem.
var ErrMalformedData = errors.New("malformed process memory map data")

// Read returns the short process name and current memory mappings for pid.
// procPath is the proc filesystem root (e.g. "/proc"); it is ignored on
// platforms that do not use a proc filesystem. When extended is true,
// per-mapping RSS and Dirty are populated if the platform backend supports
// it (see ErrExtendedNotSupported).
func Read(ctx context.Context, procPath string, pid int, extended bool) (name string, mappings []Mapping, err error) {
	return readImpl(ctx, procPath, pid, extended)
}
