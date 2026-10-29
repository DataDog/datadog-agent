// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

// Package procinfo provides OS-specific process information for the ps builtin.
//
// This package is in builtins/internal/ and is therefore exempt from the
// builtinAllowedSymbols allowlist check. It may use OS-specific APIs freely.
package procinfo

import (
	"context"
	"fmt"
	"time"
	"unicode"
	"unicode/utf8"

	"github.com/DataDog/datadog-agent/pkg/rshell/builtins/internal/procpath"
)

// MaxProcesses caps slice allocation when listing all processes.
const MaxProcesses = 10_000

// MaxCmdLen caps the process name displayed in the CMD column.
const MaxCmdLen = 4096

// Metrics identifies optional process measurements. Callers request only the
// measurements needed by their selected output and sort fields, allowing the
// Darwin and Windows backends to skip unnecessary per-process queries.
type Metrics uint32

const (
	MetricStartTime Metrics = 1 << iota
	MetricCPUTime
	MetricElapsed
	MetricRSS
	MetricVSZ
	MetricPMem
	MetricPCPU
)

// Has reports whether all metrics in wanted are present.
func (m Metrics) Has(wanted Metrics) bool {
	return m&wanted == wanted
}

// ProcInfo holds information about a single process.
type ProcInfo struct {
	PID       int
	PPID      int
	UID       string // username or numeric UID string
	State     string // single char: R, S, D, Z, T, ...
	TTY       string // "?" if no controlling terminal
	CPU       int    // integer lifetime-average CPU percentage for -f's C column
	STime     string // start time (HH:MM or Mon DD)
	Time      string // cumulative CPU time HH:MM:SS
	Cmd       string // process command/executable name only; never argv
	StartTime time.Time
	CPUTime   time.Duration
	Elapsed   time.Duration
	RSSKiB    uint64
	VSZKiB    uint64
	PMem      float64
	PCPU      float64
	Available Metrics
}

// Has reports whether all requested measurements are available for this
// process. A metric may be unavailable for one row because the process exited
// or the operating system denied access; callers must not treat that as zero.
func (p ProcInfo) Has(wanted Metrics) bool {
	return p.Available.Has(wanted)
}

// boundedCPUInteger converts a floating-point CPU percentage into the native
// integer used by ps -f's C column without relying on implementation-specific
// out-of-range float-to-int conversions.
func boundedCPUInteger(cpu float64) int {
	if !(cpu > 0) {
		return 0
	}
	maxInt := int(^uint(0) >> 1)
	if cpu >= float64(maxInt) {
		return maxInt
	}
	return int(cpu)
}

func formatStartTime(start, now time.Time) string {
	start = start.Local()
	now = now.Local()
	if start.Day() == now.Day() && start.Month() == now.Month() && start.Year() == now.Year() {
		return start.Format("15:04")
	}
	return start.Format("Jan02")
}

func formatCPUTime(cpuTime time.Duration) string {
	totalSeconds := int64(cpuTime / time.Second)
	return fmt.Sprintf(
		"%02d:%02d:%02d",
		totalSeconds/3600,
		(totalSeconds%3600)/60,
		totalSeconds%60,
	)
}

// truncateCmdName keeps kernel-controlled process names on one printable line
// and caps their encoded size without splitting a UTF-8 sequence.
func truncateCmdName(name string) string {
	sanitized := make([]byte, 0, min(len(name), MaxCmdLen))
	for len(name) > 0 && len(sanitized) < MaxCmdLen {
		r, size := utf8.DecodeRuneInString(name)
		if (r == utf8.RuneError && size == 1) || !unicode.IsGraphic(r) {
			sanitized = append(sanitized, '?')
			name = name[size:]
			continue
		}
		if len(sanitized)+size > MaxCmdLen {
			break
		}
		sanitized = append(sanitized, name[:size]...)
		name = name[size:]
	}
	return string(sanitized)
}

// DefaultProcPath is the default path to the proc filesystem.
const DefaultProcPath = procpath.Default

// resolveProcPath returns procPath if non-empty, otherwise DefaultProcPath.
func resolveProcPath(procPath string) string {
	if procPath == "" {
		return DefaultProcPath
	}
	return procPath
}

// ListAll returns all running processes.
// procPath is the path to the proc filesystem (e.g. "/proc"); pass
// DefaultProcPath or an empty string to use the default.
func ListAll(ctx context.Context, procPath string) ([]ProcInfo, error) {
	return ListAllWithMetrics(ctx, procPath, 0)
}

// ListAllWithMetrics returns all running processes and requests the optional
// measurements in metrics.
func ListAllWithMetrics(ctx context.Context, procPath string, metrics Metrics) ([]ProcInfo, error) {
	return listAll(ctx, resolveProcPath(procPath), metrics)
}

// GetSession returns processes in the current process session
// (walks PPID chain from os.Getpid() upward to collect ancestors, plus
// any processes that share the same session ID when available).
// procPath is the path to the proc filesystem; pass DefaultProcPath or an
// empty string to use the default.
func GetSession(ctx context.Context, procPath string) ([]ProcInfo, error) {
	return GetSessionWithMetrics(ctx, procPath, 0)
}

// GetSessionWithMetrics returns current-session processes and requests the
// optional measurements in metrics.
func GetSessionWithMetrics(ctx context.Context, procPath string, metrics Metrics) ([]ProcInfo, error) {
	return getSession(ctx, resolveProcPath(procPath), metrics)
}

// GetByPIDs returns process info for the given PIDs.
// Missing PIDs are silently skipped.
// procPath is the path to the proc filesystem; pass DefaultProcPath or an
// empty string to use the default.
func GetByPIDs(ctx context.Context, procPath string, pids []int) ([]ProcInfo, error) {
	return GetByPIDsWithMetrics(ctx, procPath, pids, 0)
}

// GetByPIDsWithMetrics returns process info for the given PIDs and requests
// the optional measurements in metrics.
func GetByPIDsWithMetrics(ctx context.Context, procPath string, pids []int, metrics Metrics) ([]ProcInfo, error) {
	return getByPIDs(ctx, resolveProcPath(procPath), pids, metrics)
}
