// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

// Package meminfo reads host memory and swap usage from the kernel and
// presents it as a normalised cross-platform Info struct.
//
// This package lives under builtins/internal/ and is therefore exempt from
// the builtinAllowedSymbols allowlist check. It may use OS-specific APIs
// freely.
//
// # Sandbox bypass
//
// The Linux backend reads /proc/meminfo via os.Open directly, intentionally
// bypassing the AllowedPaths sandbox (callCtx.OpenFile). The path is a
// kernel-managed pseudo-file that is hardcoded by this package and never
// derived from user-supplied input, so AllowedPaths restrictions do not
// apply. This matches the documented exception used by the ss, ip route,
// and df builtins. No process- or environment-scoped data is read (no PIDs,
// no argv, no environ) — only aggregate, host-wide counters.
//
// # Platform support
//
// Only Linux is supported; macOS and Windows return ErrNotSupported.
//
// macOS has no sysctl(3)-exposed equivalent of the page-level
// active/inactive/wired/free breakdown that GNU free reports: that data is
// only available through the Mach host_statistics64 RPC, which requires
// either cgo or dynamic libSystem symbol resolution (neither of which any
// existing builtin in this repo uses).
//
// Windows' GlobalMemoryStatusEx is easy to call, but it has no equivalent
// of buffers/cache/shared/available in the Linux sense — reporting those
// columns as a literal 0 would read as "this host has no page cache"
// rather than "this field does not exist on this platform", which could
// mislead an agent's memory-pressure diagnosis. Rather than ship a
// half-mapped or ambiguous result, both platforms fail clearly, matching
// the existing "ip route" precedent for commands that read Linux-only
// kernel state.
package meminfo

import (
	"context"
	"errors"
)

// Info describes host memory and swap usage, in bytes. Fields map directly
// to the /proc/meminfo keys of the same name; only Linux is implemented
// (see the package doc comment for why macOS and Windows are not).
type Info struct {
	// MemTotal is total usable RAM.
	MemTotal uint64

	// MemFree is memory not used for anything at all (kernel or userspace).
	MemFree uint64

	// MemAvailable is the kernel's own estimate of memory available for
	// starting new applications, without swapping (Linux ≥3.14). When the
	// kernel does not report it, Read falls back to
	// MemFree+Buffers+Cached, the pre-3.14 approximation.
	MemAvailable uint64

	// Buffers is memory in raw block-device buffers.
	Buffers uint64

	// Cached is page-cache memory, minus Shared.
	Cached uint64

	// SReclaimable is reclaimable slab memory (part of "buff/cache" in
	// modern free output).
	SReclaimable uint64

	// Shared is memory used by tmpfs and shared memory segments (Shmem).
	Shared uint64

	// SwapTotal is total swap space.
	SwapTotal uint64

	// SwapFree is unused swap space.
	SwapFree uint64
}

// ErrNotSupported is returned by Read on platforms without a backend.
var ErrNotSupported = errors.New("not supported on this platform")

// Read returns current host memory and swap usage.
func Read(ctx context.Context) (Info, error) {
	return readImpl(ctx)
}

// saturatingAdd returns a + b, clamped to uint64 max on overflow.
func saturatingAdd(a, b uint64) uint64 {
	if a > ^uint64(0)-b {
		return ^uint64(0)
	}
	return a + b
}
