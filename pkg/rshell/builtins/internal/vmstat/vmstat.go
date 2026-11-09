// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

// Package vmstat reads virtual-memory, swap, IO-paging, and CPU pressure
// counters from the kernel and presents them as a normalised cross-platform
// Stats struct.
//
// This package lives under builtins/internal/ and is therefore exempt from
// the builtinAllowedSymbols allowlist check. It may use OS-specific APIs
// freely.
//
// # Sandbox bypass
//
// The Linux backend reads /proc/stat, /proc/meminfo, /proc/vmstat, and
// /proc/loadavg via os.Open directly, intentionally bypassing the
// AllowedPaths sandbox (callCtx.OpenFile). These paths are kernel-managed
// pseudo-files hardcoded by this package and never derived from user
// input, so AllowedPaths restrictions do not apply. This matches the
// documented exception used by the ss, ip route, and df builtins.
//
// # Platform coverage
//
// Linux populates every field group. macOS populates memory totals, swap
// totals, and load averages via sysctl(3) — the same darwin toolset the ss
// and df builtins already use (golang.org/x/sys/unix, no Mach calls). Per
// page-state memory breakdown (active/inactive/wired), CPU tick counters,
// and paging/interrupt/context-switch rates are not available through
// sysctl and are reported as unavailable with Partial cleared for those groups;
// see Stats.Partial. Other platforms return ErrNotSupported.
//
// # Memory and CPU bounds
//
// The Linux backend bounds both the per-line buffer size and the total
// number of lines scanned per pseudo-file, so a pathological /proc content
// cannot exhaust memory or CPU.
package vmstat

import (
	"context"
	"errors"
)

// ErrNotSupported is returned by Read on platforms where no backend is
// implemented.
var ErrNotSupported = errors.New("not supported on this platform")

// Fields is a bitmask identifying which groups of Stats are populated by
// the current platform's backend. Callers must not treat a field outside
// Stats.Partial as a genuine zero measurement.
type Fields uint32

const (
	// FieldProcs covers ProcsRunning / ProcsBlocked.
	FieldProcs Fields = 1 << iota
	// FieldMemory covers MemTotal only.
	FieldMemory
	// FieldSwap covers SwapTotal / SwapFree.
	FieldSwap
	// FieldPaging covers PagesInKB / PagesOutKB / SwapInPages / SwapOutPages.
	FieldPaging
	// FieldSystem covers Interrupts / ContextSwitches.
	FieldSystem
	// FieldCPU covers CPUUser / CPUNice / CPUSystem / CPUIdle / CPUIOWait /
	// CPUIRQ / CPUSoftIRQ / CPUSteal.
	FieldCPU
	// FieldLoadAvg covers LoadAvg1 / LoadAvg5 / LoadAvg15.
	FieldLoadAvg
	// FieldMemoryDetail covers MemFree / MemBuffers / MemCached / MemActive /
	// MemInactive. Split out from FieldMemory because macOS populates
	// MemTotal via hw.memsize but has no sysctl for the breakdown without a
	// Mach host_statistics64 call (see package doc): a caller that gated
	// derived values like "used memory" (MemTotal - MemFree) on FieldMemory
	// alone would silently treat the zeroed MemFree as real, reporting 100%
	// memory used on every Mac.
	FieldMemoryDetail
)

// AllFields is every field group. The Linux backend sets this.
const AllFields = FieldProcs | FieldMemory | FieldSwap | FieldPaging | FieldSystem | FieldCPU | FieldLoadAvg | FieldMemoryDetail

// Stats holds host-pressure counters. Unless noted, values are cumulative
// counters since boot (matching /proc/*'s semantics); the vmstat builtin is
// responsible for turning them into rates via repeated sampling.
type Stats struct {
	// ProcsRunning is the number of processes in a runnable state.
	ProcsRunning uint64
	// ProcsBlocked is the number of processes blocked on uninterruptible IO.
	ProcsBlocked uint64

	// Memory, in bytes.
	MemTotal    uint64
	MemFree     uint64
	MemBuffers  uint64
	MemCached   uint64
	MemActive   uint64
	MemInactive uint64

	// Swap, in bytes.
	SwapTotal uint64
	SwapFree  uint64

	// Paging counters, cumulative since boot.
	// PagesInKB / PagesOutKB are kibibytes paged in/out from/to disk
	// (matches /proc/vmstat's pgpgin/pgpgout, which are already KiB).
	PagesInKB  uint64
	PagesOutKB uint64
	// SwapInPages / SwapOutPages are page counts swapped in/out (matches
	// /proc/vmstat's pswpin/pswpout). Multiply by PageSize to get bytes.
	SwapInPages  uint64
	SwapOutPages uint64

	// System counters, cumulative since boot.
	Interrupts      uint64
	ContextSwitches uint64

	// CPU ticks, cumulative since boot. Divide by ClockTicksPerSec to get
	// seconds, or take deltas across two samples for a utilization ratio.
	CPUUser    uint64
	CPUNice    uint64
	CPUSystem  uint64
	CPUIdle    uint64
	CPUIOWait  uint64
	CPUIRQ     uint64
	CPUSoftIRQ uint64
	CPUSteal   uint64

	// ClockTicksPerSec is the unit for the CPU tick fields above.
	ClockTicksPerSec uint64
	// PageSize is the host's memory page size, in bytes.
	PageSize uint64

	// LoadAvg1 / LoadAvg5 / LoadAvg15 are the 1/5/15-minute load averages.
	LoadAvg1, LoadAvg5, LoadAvg15 float64

	// Uptime is the number of seconds since boot, used to compute
	// since-boot per-second averages for the rate columns (si/so/bi/bo/
	// in/cs/us/sy/id/wa/st) in a single-sample snapshot. Zero when
	// unavailable (e.g. macOS); callers must treat 0 as "unknown", not a
	// literal just-booted host.
	Uptime float64

	// Partial reports which field groups the current platform populated.
	// Fields outside this mask are always zero and must be presented as
	// "not available" (e.g. "-") rather than as a genuine zero reading.
	Partial Fields
}

// Read collects a point-in-time snapshot of host-pressure counters.
// procPath is the proc filesystem root (e.g. "/proc"); it is ignored on
// platforms that do not use /proc.
//
// On unsupported platforms it returns (Stats{}, ErrNotSupported).
func Read(ctx context.Context, procPath string) (Stats, error) {
	return readImpl(ctx, procPath)
}
