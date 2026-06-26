// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux

// Package probe holds probe related files
package probe

import (
	"fmt"
	"math"
	"path/filepath"
	"strconv"
	"time"

	gopsutilprocess "github.com/shirou/gopsutil/v4/process"
	"golang.org/x/sys/unix"

	"github.com/DataDog/datadog-agent/pkg/security/secl/model"
	"github.com/DataDog/datadog-agent/pkg/security/seclog"
	"github.com/DataDog/datadog-agent/pkg/security/utils"
	utilkernel "github.com/DataDog/datadog-agent/pkg/util/kernel"
)

// startTimeTolerance covers jiffies precision + boot-time skew between
// the eBPF-captured ForkTime and gopsutil's /proc/<pid>/stat reading.
const startTimeTolerance = 2 * time.Second

// enrichKind selects which kind of values we re-resolve from /proc on a
// rule match. Used to share fetch + apply code between argv and envp.
type enrichKind int

const (
	enrichArgs enrichKind = iota
	enrichEnvs
)

func (k enrichKind) String() string {
	if k == enrichArgs {
		return "argv"
	}
	return "envp"
}

// EnrichRuleEvent re-resolves the matched process's argv and envp from
// /proc so the alert carries the full command line and environment rather
// than the length-capped values written by the kernel-side eBPF programs
// (see sharedconsts.MaxArgEnvSize / MaxArgsEnvsSize). Best-effort: silently
// keeps the cached truncated values if /proc is unreadable or the PID has
// been reused.
func (p *EBPFProbe) EnrichRuleEvent(ev *model.Event) {
	if ev == nil || ev.ProcessContext == nil {
		return
	}
	pr := &ev.ProcessContext.Process
	if pr.Pid == 0 {
		return
	}

	argsTruncated := pr.ArgsTruncated || (pr.ArgsEntry != nil && pr.ArgsEntry.Truncated)
	envsTruncated := pr.EnvsTruncated || (pr.EnvsEntry != nil && pr.EnvsEntry.Truncated)
	if !argsTruncated && !envsTruncated {
		return
	}

	// shared PID-reuse / process-gone guard: prefer a truncated-but-correct
	// alert over a complete-but-misattributed one.
	if !sameProcessAsCached(pr) {
		seclog.Tracef("rule-match enrichment skipped for pid %d: process gone or PID reused (cached inode=%d)",
			pr.Pid, pr.FileEvent.Inode)
		return
	}

	if argsTruncated {
		tryEnrich(pr, enrichArgs)
	}
	if envsTruncated {
		tryEnrich(pr, enrichEnvs)
	}
}

// tryEnrich reads the requested values from /proc and, if non-empty, swaps
// them into pr in place of the kernel-captured truncated values. Empty
// /proc data (e.g. for a process that rewrote its own argv memory) is
// treated as "no fuller data available" — we keep the truncated values
// rather than overwrite with nothing.
func tryEnrich(pr *model.Process, kind enrichKind) {
	full, err := fetchProcessValues(pr.Pid, kind)
	switch {
	case err != nil:
		seclog.Tracef("rule-match %s enrichment skipped for pid %d: %v", kind, pr.Pid, err)
	case len(full) == 0:
		seclog.Tracef("rule-match %s enrichment skipped for pid %d: empty /proc data", kind, pr.Pid)
	default:
		applyFullProcessValues(pr, full, kind)
	}
}

// fetchProcessValues reads the full argv or envp for pid from /proc.
func fetchProcessValues(pid uint32, kind enrichKind) ([]string, error) {
	switch kind {
	case enrichArgs:
		proc, err := gopsutilprocess.NewProcess(int32(pid))
		if err != nil {
			return nil, fmt.Errorf("gopsutil.NewProcess: %w", err)
		}
		return proc.CmdlineSlice()
	case enrichEnvs:
		// math.MaxInt disables the helper's element cap; we want the full env.
		envs, _, err := utils.EnvVars(nil, pid, math.MaxInt)
		return envs, err
	}
	return nil, fmt.Errorf("unknown enrich kind: %v", kind)
}

// applyFullProcessValues swaps in a fresh entry on pr carrying the
// /proc-derived values and clears the lazy resolution caches so field
// handlers re-derive on next access.
func applyFullProcessValues(pr *model.Process, values []string, kind enrichKind) {
	// fresh allocation rather than in-place mutation: ArgsEntry/EnvsEntry
	// carry unexported scrubber state that would otherwise leak across entries.
	switch kind {
	case enrichArgs:
		pr.ArgsEntry = &model.ArgsEntry{Values: values}
		pr.ArgsTruncated = false
		pr.Args = ""
		pr.Argv = nil
		if len(values) > 0 {
			pr.Argv0 = values[0]
		}
	case enrichEnvs:
		pr.EnvsEntry = &model.EnvsEntry{Values: values}
		pr.EnvsTruncated = false
		pr.Envs = nil
		pr.Envp = nil
	}
}

// sameProcessAsCached returns true when /proc/<pr.Pid> still describes the
// process the eBPF probe captured: inode of /proc/<pid>/exe matches the
// cached one and /proc/<pid>/stat start time matches ForkTime. The start
// time invariant defeats PID reuse even when the recycled PID re-execs the
// same binary. Falls back to inode-only when ForkTime is zero.
func sameProcessAsCached(pr *model.Process) bool {
	cachedInode := pr.FileEvent.Inode
	if cachedInode == 0 {
		return false
	}
	exePath := filepath.Join(utilkernel.ProcFSRoot(), strconv.FormatUint(uint64(pr.Pid), 10), "exe")
	var st unix.Stat_t
	if err := unix.Stat(exePath, &st); err != nil {
		return false
	}
	if st.Ino != cachedInode {
		return false
	}
	if !pr.ForkTime.IsZero() {
		proc, err := gopsutilprocess.NewProcess(int32(pr.Pid))
		if err != nil {
			return false
		}
		createMs, err := proc.CreateTime()
		if err != nil {
			return false
		}
		if delta := pr.ForkTime.Sub(time.UnixMilli(createMs)).Abs(); delta > startTimeTolerance {
			return false
		}
	}
	return true
}
