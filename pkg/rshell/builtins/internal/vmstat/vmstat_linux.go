// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build linux

package vmstat

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"math"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

// clockTicksPerSec is the kernel's clock-tick rate (sysconf(_SC_CLK_TCK)).
// This is 100 on essentially every Linux platform rshell targets; the
// procinfo package (used by ps) hardcodes the same value for the same
// reason: there is no allocation-free, syscall-free way to query
// _SC_CLK_TCK from pure Go without cgo.
const clockTicksPerSec = 100

// maxLineLen bounds the per-line buffer size when scanning /proc/{stat,
// meminfo,vmstat,loadavg}. Lines longer than this abort the scan for that
// file (best-effort: the fields already parsed are kept).
const maxLineLen = 1 << 16 // 64 KiB

// maxLines bounds the total number of lines scanned per file. Real kernels
// emit at most a few hundred lines even on very large machines (one line
// per CPU in /proc/stat); this is a defensive ceiling against a
// pathological /proc mount, not a realistic limit.
const maxLines = 100_000

var errTooManyLines = errors.New("line limit exceeded")

// readImpl assembles Stats from /proc/stat, /proc/meminfo, /proc/vmstat,
// and /proc/loadavg. Every file is opened directly via os.Open — this is
// the documented sandbox-bypass exception also used by diskstats,
// procnetsocket, and procsyskernel: the paths are hardcoded kernel
// pseudo-files, never derived from user input.
//
// Missing or unreadable files are tolerated: their field group is simply
// left out of the Partial mask so the caller can render "-" instead of a
// false zero.
func readImpl(ctx context.Context, procPath string) (Stats, error) {
	if err := ctx.Err(); err != nil {
		return Stats{}, err
	}

	var st Stats
	st.ClockTicksPerSec = clockTicksPerSec
	st.PageSize = uint64(os.Getpagesize())

	if foundProcs, foundSystem, foundCPU, err := readProcStat(ctx, filepath.Join(procPath, "stat"), &st); err == nil {
		if foundProcs {
			st.Partial |= FieldProcs
		}
		if foundSystem {
			st.Partial |= FieldSystem
		}
		if foundCPU {
			st.Partial |= FieldCPU
		}
	} else if ctxErr := ctx.Err(); ctxErr != nil {
		return Stats{}, ctxErr
	}
	if foundMemory, foundMemoryDetail, foundSwap, err := readProcMeminfo(ctx, filepath.Join(procPath, "meminfo"), &st); err == nil {
		if foundMemory {
			st.Partial |= FieldMemory
		}
		if foundMemoryDetail {
			st.Partial |= FieldMemoryDetail
		}
		if foundSwap {
			st.Partial |= FieldSwap
		}
	} else if ctxErr := ctx.Err(); ctxErr != nil {
		return Stats{}, ctxErr
	}
	if err := readProcVmstat(ctx, filepath.Join(procPath, "vmstat"), &st); err == nil {
		st.Partial |= FieldPaging
	} else if ctxErr := ctx.Err(); ctxErr != nil {
		return Stats{}, ctxErr
	}
	if err := readProcLoadavg(ctx, filepath.Join(procPath, "loadavg"), &st); err == nil {
		st.Partial |= FieldLoadAvg
	} else if ctxErr := ctx.Err(); ctxErr != nil {
		return Stats{}, ctxErr
	}
	// Uptime is best-effort and does not gate Partial: it only feeds the
	// since-boot rate-average computation in the builtin's single-sample
	// snapshot mode, and a missing /proc/uptime should not hide the rest
	// of the (successfully read) fields.
	if err := readProcUptime(ctx, filepath.Join(procPath, "uptime"), &st); err != nil {
		if ctxErr := ctx.Err(); ctxErr != nil {
			return Stats{}, ctxErr
		}
	}

	if st.Partial == 0 {
		return Stats{}, fmt.Errorf("no /proc/{stat,meminfo,vmstat,loadavg} data available under %s", procPath)
	}
	return st, nil
}

// scanBounded runs fn once per line of path, using a buffer capped at
// maxLineLen and a total-line ceiling of maxLines. It is the shared
// bounded-read primitive for every /proc file this package parses.
func scanBounded(ctx context.Context, path string, fn func(line string)) error {
	f, err := os.Open(path) //nolint:gosec // hardcoded kernel pseudo-file; see package doc.
	if err != nil {
		return err
	}
	defer f.Close() //nolint:errcheck

	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 0, 4096), maxLineLen)
	lines := 0
	for sc.Scan() {
		if err := ctx.Err(); err != nil {
			return err
		}
		if lines == maxLines {
			return errTooManyLines
		}
		lines++
		fn(sc.Text())
	}
	if err := sc.Err(); err != nil && err != io.EOF {
		return err
	}
	return nil
}

// readProcStat parses procs_running, procs_blocked, intr, ctxt, and the
// aggregate "cpu " line from /proc/stat. It reports foundProcs/foundSystem/
// foundCPU independently so readImpl never sets a Partial bit for a field
// group whose source line was actually absent (e.g. a /proc/stat with
// "intr " but no "cpu " line must not mark CPU data as available).
func readProcStat(ctx context.Context, path string, st *Stats) (foundProcs, foundSystem, foundCPU bool, err error) {
	var foundRunning, foundBlocked, foundInterrupts, foundContexts bool
	err = scanBounded(ctx, path, func(line string) {
		switch {
		case strings.HasPrefix(line, "cpu "):
			fields := strings.Fields(line)
			// fields[0] == "cpu"; user nice system idle iowait irq softirq steal ...
			vals, ok := parseCPUFields(fields[1:])
			if !ok {
				return
			}
			st.CPUUser, st.CPUNice, st.CPUSystem, st.CPUIdle = vals[0], vals[1], vals[2], vals[3]
			st.CPUIOWait, st.CPUIRQ, st.CPUSoftIRQ, st.CPUSteal = vals[4], vals[5], vals[6], vals[7]
			foundCPU = true
		case strings.HasPrefix(line, "intr "):
			if v, ok := parseUint64Field(line); ok {
				st.Interrupts, foundInterrupts = v, true
			}
		case strings.HasPrefix(line, "ctxt "):
			if v, ok := parseUint64Field(line); ok {
				st.ContextSwitches, foundContexts = v, true
			}
		case strings.HasPrefix(line, "procs_running "):
			if v, ok := parseUint64Field(line); ok {
				st.ProcsRunning, foundRunning = v, true
			}
		case strings.HasPrefix(line, "procs_blocked "):
			if v, ok := parseUint64Field(line); ok {
				st.ProcsBlocked, foundBlocked = v, true
			}
		}
	})
	if err != nil {
		return false, false, false, err
	}
	foundProcs = foundRunning && foundBlocked
	foundSystem = foundInterrupts && foundContexts
	if !foundRunning && !foundBlocked && !foundInterrupts && !foundContexts && !foundCPU {
		return false, false, false, fmt.Errorf("vmstat: no recognised fields in %s", path)
	}
	return foundProcs, foundSystem, foundCPU, nil
}

// maxMeminfoKB is the largest KiB value that can be multiplied by 1024
// without overflowing uint64; it bounds the KiB-to-bytes conversion below.
const maxMeminfoKB = math.MaxUint64 / 1024

// readProcMeminfo parses the memory and swap fields from /proc/meminfo.
// Values in the file are in KiB; they are converted to bytes. It reports
// foundMemory (MemTotal only), foundMemoryDetail (the free/buffers/cached/
// active/inactive breakdown), and foundSwap independently so readImpl never
// sets a Partial bit for a field group whose lines were actually absent
// from the file.
func readProcMeminfo(ctx context.Context, path string, st *Stats) (foundMemory, foundMemoryDetail, foundSwap bool, err error) {
	var sreclaimable uint64
	var foundFree, foundBuffers, foundCached, foundActive, foundInactive bool
	var foundSwapTotal, foundSwapFree bool
	err = scanBounded(ctx, path, func(line string) {
		key, kb, ok := parseMeminfoLine(line)
		if !ok || kb > maxMeminfoKB {
			return
		}
		bytes := kb * 1024
		switch key {
		case "MemTotal":
			st.MemTotal, foundMemory = bytes, true
		case "MemFree":
			st.MemFree, foundFree = bytes, true
		case "Buffers":
			st.MemBuffers, foundBuffers = bytes, true
		case "Cached":
			st.MemCached, foundCached = bytes, true
		case "SReclaimable":
			sreclaimable = bytes
		case "Active":
			st.MemActive, foundActive = bytes, true
		case "Inactive":
			st.MemInactive, foundInactive = bytes, true
		case "SwapTotal":
			st.SwapTotal, foundSwapTotal = bytes, true
		case "SwapFree":
			st.SwapFree, foundSwapFree = bytes, true
		}
	})
	if err != nil {
		return false, false, false, err
	}
	foundMemoryDetail = foundFree && foundBuffers && foundCached && foundActive && foundInactive
	foundSwap = foundSwapTotal && foundSwapFree
	if !foundMemory && !foundFree && !foundBuffers && !foundCached && !foundActive && !foundInactive && !foundSwapTotal && !foundSwapFree {
		return false, false, false, fmt.Errorf("vmstat: no recognised fields in %s", path)
	}
	// SReclaimable (reclaimable slab memory) counts toward "cache" in the
	// procps/free accounting this repo already follows for the free
	// builtin (see builtins/internal/meminfo); Cached alone under-reports
	// reclaimable cache on hosts with nontrivial slab usage.
	st.MemCached = saturatingAddUint64(st.MemCached, sreclaimable)
	return foundMemory, foundMemoryDetail, foundSwap, nil
}

// saturatingAddUint64 returns a + b, clamped to uint64 max on overflow.
func saturatingAddUint64(a, b uint64) uint64 {
	sum := a + b
	if sum < a {
		return math.MaxUint64
	}
	return sum
}

// parseMeminfoLine splits a "Key:      123 kB" line into its key and
// numeric value (in KiB). Returns ok=false for malformed lines.
func parseMeminfoLine(line string) (key string, kb uint64, ok bool) {
	k, rest, found := strings.Cut(line, ":")
	if !found || k == "" {
		return "", 0, false
	}
	fields := strings.Fields(rest)
	if len(fields) != 2 || fields[1] != "kB" {
		return "", 0, false
	}
	v, err := strconv.ParseUint(fields[0], 10, 64)
	if err != nil {
		return "", 0, false
	}
	return k, v, true
}

// readProcVmstat parses pgpgin/pgpgout/pswpin/pswpout from /proc/vmstat.
func readProcVmstat(ctx context.Context, path string, st *Stats) error {
	var foundIn, foundOut, foundSwapIn, foundSwapOut bool
	err := scanBounded(ctx, path, func(line string) {
		fields := strings.Fields(line)
		if len(fields) != 2 {
			return
		}
		v, err := strconv.ParseUint(fields[1], 10, 64)
		if err != nil {
			return
		}
		switch fields[0] {
		case "pgpgin":
			st.PagesInKB, foundIn = v, true
		case "pgpgout":
			st.PagesOutKB, foundOut = v, true
		case "pswpin":
			st.SwapInPages, foundSwapIn = v, true
		case "pswpout":
			st.SwapOutPages, foundSwapOut = v, true
		}
	})
	if err != nil {
		return err
	}
	if !foundIn && !foundOut && !foundSwapIn && !foundSwapOut {
		return fmt.Errorf("vmstat: no recognised fields in %s", path)
	}
	if !foundIn || !foundOut || !foundSwapIn || !foundSwapOut {
		return fmt.Errorf("vmstat: incomplete paging fields in %s", path)
	}
	return nil
}

// readProcLoadavg parses the three load averages from /proc/loadavg
// ("0.12 0.34 0.56 1/234 5678").
func readProcLoadavg(ctx context.Context, path string, st *Stats) error {
	found := false
	err := scanBounded(ctx, path, func(line string) {
		fields := strings.Fields(line)
		if len(fields) < 3 {
			return
		}
		l1, err1 := strconv.ParseFloat(fields[0], 64)
		l5, err5 := strconv.ParseFloat(fields[1], 64)
		l15, err15 := strconv.ParseFloat(fields[2], 64)
		if err1 != nil || err5 != nil || err15 != nil ||
			math.IsNaN(l1) || math.IsNaN(l5) || math.IsNaN(l15) ||
			math.IsInf(l1, 0) || math.IsInf(l5, 0) || math.IsInf(l15, 0) ||
			l1 < 0 || l5 < 0 || l15 < 0 {
			return
		}
		st.LoadAvg1, st.LoadAvg5, st.LoadAvg15 = l1, l5, l15
		found = true
	})
	if err != nil {
		return err
	}
	if !found {
		return fmt.Errorf("vmstat: no recognised fields in %s", path)
	}
	return nil
}

// readProcUptime parses the first field of /proc/uptime (seconds since
// boot). The second field (idle-time seconds) is ignored.
func readProcUptime(ctx context.Context, path string, st *Stats) error {
	found := false
	err := scanBounded(ctx, path, func(line string) {
		fields := strings.Fields(line)
		if len(fields) < 1 {
			return
		}
		v, err := strconv.ParseFloat(fields[0], 64)
		if err != nil || math.IsNaN(v) || math.IsInf(v, 0) || v < 0 {
			return
		}
		st.Uptime = v
		found = true
	})
	if err != nil {
		return err
	}
	if !found {
		return fmt.Errorf("vmstat: no recognised fields in %s", path)
	}
	return nil
}

// parseUint64Field parses the single numeric value out of a "key value"
// line (e.g. "ctxt 987654").
func parseUint64Field(line string) (uint64, bool) {
	fields := strings.Fields(line)
	if len(fields) < 2 {
		return 0, false
	}
	v, err := strconv.ParseUint(fields[1], 10, 64)
	if err != nil {
		return 0, false
	}
	return v, true
}

// parseCPUFields parses the aggregate CPU counters. The first four fields
// have existed since Linux first exposed /proc/stat and are required; the
// newer trailing fields remain optional for compatibility with old kernels.
func parseCPUFields(fields []string) ([8]uint64, bool) {
	var out [8]uint64
	if len(fields) < 4 {
		return out, false
	}
	for i := 0; i < len(out) && i < len(fields); i++ {
		v, err := strconv.ParseUint(fields[i], 10, 64)
		if err != nil {
			if i < 4 {
				return [8]uint64{}, false
			}
			continue
		}
		out[i] = v
	}
	return out, true
}
