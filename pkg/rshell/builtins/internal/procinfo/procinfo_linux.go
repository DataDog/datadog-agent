// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build linux

package procinfo

import (
	"bufio"
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

// clkTck is the number of clock ticks per second. On modern Linux this is
// almost always 100, but we default to 100 and let procBootTime handle errors.
const clkTck = 100

const (
	maxProcStatBytes   = 1 << 20
	maxProcUptimeBytes = 4096
)

type linuxMetricInputs struct {
	requested   Metrics
	uptime      time.Duration
	uptimeValid bool
	memTotalKiB uint64
}

func readLinuxMetricInputs(procPath string, requested Metrics) linuxMetricInputs {
	inputs := linuxMetricInputs{requested: requested}
	if requested.Has(MetricElapsed) || requested.Has(MetricPCPU) {
		if uptime, err := procUptime(procPath); err == nil {
			inputs.uptime = uptime
			inputs.uptimeValid = true
		}
	}
	if requested.Has(MetricPMem) {
		inputs.memTotalKiB, _ = procMemTotalKiB(procPath)
	}
	return inputs
}

func listAll(ctx context.Context, procPath string, metrics Metrics) ([]ProcInfo, error) {
	entries, err := os.ReadDir(procPath)
	if err != nil {
		return nil, fmt.Errorf("ps: cannot read %s: %w", procPath, err)
	}

	btime, _ := procBootTime(procPath)
	metricInputs := readLinuxMetricInputs(procPath, metrics)
	var procs []ProcInfo
	for _, e := range entries {
		if ctx.Err() != nil {
			break
		}
		if len(procs) >= MaxProcesses {
			break
		}
		if !e.IsDir() {
			continue
		}
		pid, err := strconv.Atoi(e.Name())
		if err != nil {
			continue
		}
		info, err := readProc(procPath, pid, btime, metricInputs)
		if err != nil {
			continue
		}
		procs = append(procs, info)
	}
	return procs, nil
}

func getSession(ctx context.Context, procPath string, metrics Metrics) ([]ProcInfo, error) {
	all, err := listAll(ctx, procPath, metrics)
	if err != nil {
		return nil, err
	}
	// Build a map for quick lookup.
	byPID := make(map[int]ProcInfo, len(all))
	for _, p := range all {
		byPID[p.PID] = p
	}

	// Walk PPID chain from current process upward; collect session ancestors.
	// Note: if procPath points to a foreign PID namespace (e.g. a container),
	// our host PID is unlikely to appear there, so the session result will be
	// empty. This is expected — GetSession is designed for the current host.
	selfPID := os.Getpid()
	ancestors := collectAncestorPIDs(ctx, byPID, selfPID, 1)

	// Also include all processes that share our SID (best-effort; fall back to
	// ancestor chain only).
	var selfSID int
	if data, err := readBoundedProcFile(
		filepath.Join(procPath, strconv.Itoa(selfPID), "stat"),
		maxProcStatBytes,
	); err == nil {
		selfSID = parseSID(data)
	}

	var result []ProcInfo
	for _, p := range all {
		if ctx.Err() != nil {
			break
		}
		if ancestors[p.PID] {
			result = append(result, p)
			continue
		}
		if selfSID != 0 {
			if data, err := readBoundedProcFile(
				filepath.Join(procPath, strconv.Itoa(p.PID), "stat"),
				maxProcStatBytes,
			); err == nil {
				if parseSID(data) == selfSID {
					result = append(result, p)
				}
			}
		}
	}
	return result, nil
}

func getByPIDs(ctx context.Context, procPath string, pids []int, metrics Metrics) ([]ProcInfo, error) {
	fi, err := os.Stat(procPath)
	if err != nil {
		return nil, fmt.Errorf("ps: cannot read %s: %w", procPath, err)
	}
	if !fi.IsDir() {
		return nil, fmt.Errorf("ps: cannot read %s: not a directory", procPath)
	}
	btime, _ := procBootTime(procPath)
	metricInputs := readLinuxMetricInputs(procPath, metrics)
	var result []ProcInfo
	for _, pid := range pids {
		if ctx.Err() != nil {
			break
		}
		info, err := readProc(procPath, pid, btime, metricInputs)
		if err != nil {
			// ENOENT means the process no longer exists — skip silently.
			// Any other error (EACCES, I/O, etc.) indicates a configuration
			// or read failure and should be surfaced to the caller.
			if errors.Is(err, os.ErrNotExist) {
				continue
			}
			return nil, fmt.Errorf("ps: cannot read %s: %w", filepath.Join(procPath, strconv.Itoa(pid)), err)
		}
		result = append(result, info)
	}
	return result, nil
}

// readProc reads process info for a single PID from procPath.
func readProc(procPath string, pid int, btime int64, metricInputs linuxMetricInputs) (ProcInfo, error) {
	statData, err := readBoundedProcFile(
		filepath.Join(procPath, strconv.Itoa(pid), "stat"),
		maxProcStatBytes,
	)
	if err != nil {
		return ProcInfo{}, err
	}

	var info ProcInfo
	info.PID = pid

	// Parse /proc/stat. The format is:
	//   pid (comm) state ppid pgroup session tty_nr ...
	// The comm field may contain spaces and is delimited by parentheses.
	statStr := strings.TrimSpace(string(statData))
	openParen := strings.Index(statStr, "(")
	closeParen := strings.LastIndex(statStr, ")")
	if openParen < 0 || closeParen < 0 || closeParen <= openParen {
		return ProcInfo{}, fmt.Errorf("ps: malformed stat for pid %d", pid)
	}
	comm := statStr[openParen+1 : closeParen]
	rest := strings.Fields(statStr[closeParen+1:])
	// rest[0]=state, rest[1]=ppid, rest[2]=pgroup, rest[3]=session, rest[4]=tty_nr
	// rest[11]=utime, rest[12]=stime (1-indexed from after closeParen+1, so offset by 1)
	// Indices: state=0 ppid=1 pgroup=2 session=3 tty_nr=4 ... utime=11 stime=12
	//          cutime=13 cstime=14 ... starttime=19
	if len(rest) < 20 {
		return ProcInfo{}, fmt.Errorf("ps: short stat for pid %d", pid)
	}

	info.State = string(rest[0])
	info.PPID, _ = strconv.Atoi(rest[1])
	ttyNr, _ := strconv.ParseInt(rest[4], 10, 64)
	utime, utimeErr := strconv.ParseInt(rest[11], 10, 64)
	stime, stimeErr := strconv.ParseInt(rest[12], 10, 64)
	starttime, starttimeErr := strconv.ParseInt(rest[19], 10, 64)

	info.TTY = resolveTTY(ttyNr)

	// CPU time: (utime + stime) in clock ticks → HH:MM:SS.
	info.Time = "-"
	if utimeErr == nil && stimeErr == nil && utime >= 0 && stime >= 0 {
		maxInt64 := int64(^uint64(0) >> 1)
		if utime <= maxInt64-stime {
			if cpuTime, ok := ticksToDuration(utime + stime); ok {
				info.CPUTime = cpuTime
				info.Time = formatCPUTime(cpuTime)
				info.Available |= MetricCPUTime
			}
		}
	}

	// Start time.
	if t, ok := procStartTime(btime, starttime, starttimeErr); ok {
		info.StartTime = t
		info.Available |= MetricStartTime
		info.STime = formatStartTime(t, time.Now())
	} else {
		info.STime = "?"
	}

	if metricInputs.uptimeValid && starttimeErr == nil {
		if startDuration, ok := ticksToDuration(starttime); ok && metricInputs.uptime >= startDuration {
			info.Elapsed = metricInputs.uptime - startDuration
			info.Available |= MetricElapsed
		}
	}

	if metricInputs.requested.Has(MetricPCPU) &&
		info.Has(MetricCPUTime|MetricElapsed) &&
		info.Elapsed > 0 {
		info.PCPU = 100 * float64(info.CPUTime) / float64(info.Elapsed)
		info.CPU = boundedCPUInteger(info.PCPU)
		info.Available |= MetricPCPU
	}

	if len(rest) >= 22 {
		if metricInputs.requested.Has(MetricVSZ) {
			if vsizeBytes, parseErr := strconv.ParseUint(rest[20], 10, 64); parseErr == nil {
				info.VSZKiB = vsizeBytes / 1024
				info.Available |= MetricVSZ
			}
		}
		if metricInputs.requested.Has(MetricRSS) || metricInputs.requested.Has(MetricPMem) {
			if rssPages, parseErr := strconv.ParseUint(rest[21], 10, 64); parseErr == nil {
				pageSize := uint64(os.Getpagesize())
				if rssPages <= ^uint64(0)/pageSize {
					info.RSSKiB = rssPages * pageSize / 1024
					info.Available |= MetricRSS
				}
			}
		}
	}

	if metricInputs.requested.Has(MetricPMem) &&
		info.Has(MetricRSS) &&
		metricInputs.memTotalKiB > 0 {
		info.PMem = 100 * float64(info.RSSKiB) / float64(metricInputs.memTotalKiB)
		info.Available |= MetricPMem
	}

	// UID from procPath/pid/status.
	info.UID = readUID(procPath, pid)

	info.Cmd = truncateCmdName(comm)

	return info, nil
}

// resolveTTY maps tty_nr (from /proc/pid/stat) to a human-readable name.
// tty_nr encodes the controlling terminal's device number:
//
//	major = bits [15:8]
//	minor = bits [7:0] | (bits [31:20] << 8)
//
// We decode this directly instead of reading /proc/pid/fd/0 (which is stdin
// and may point to a redirected file rather than the controlling terminal).
func resolveTTY(ttyNr int64) string {
	if ttyNr == 0 {
		return "?"
	}
	major := (ttyNr >> 8) & 0xff
	minor := (ttyNr & 0xff) | ((ttyNr >> 20) << 8)
	switch {
	case major == 4 && minor < 64:
		// Virtual consoles: /dev/ttyN
		return fmt.Sprintf("tty%d", minor)
	case major == 4:
		// Serial terminals: /dev/ttySN
		return fmt.Sprintf("ttyS%d", minor-64)
	case major >= 136 && major <= 143:
		// Pseudo-terminal slaves: /dev/pts/N
		pts := (major-136)*256 + minor
		return fmt.Sprintf("pts/%d", pts)
	default:
		return "?"
	}
}

// readUID reads the real UID from procPath/pid/status.
func readUID(procPath string, pid int) string {
	data, err := os.ReadFile(filepath.Join(procPath, strconv.Itoa(pid), "status"))
	if err != nil {
		return "?"
	}
	scanner := bufio.NewScanner(bytes.NewReader(data))
	scanner.Buffer(make([]byte, 4096), 1<<20)
	for scanner.Scan() {
		line := scanner.Text()
		if strings.HasPrefix(line, "Uid:") {
			fields := strings.Fields(line)
			if len(fields) >= 2 {
				return fields[1] // real UID
			}
		}
	}
	return "?"
}

// procBootTime reads the boot time (seconds since epoch) from procPath/stat.
func procBootTime(procPath string) (int64, error) {
	f, err := os.Open(filepath.Join(procPath, "stat"))
	if err != nil {
		return 0, err
	}
	defer f.Close()
	scanner := bufio.NewScanner(f)
	scanner.Buffer(make([]byte, 4096), 1<<20)
	for scanner.Scan() {
		line := scanner.Text()
		if strings.HasPrefix(line, "btime ") {
			fields := strings.Fields(line)
			if len(fields) >= 2 {
				return strconv.ParseInt(fields[1], 10, 64)
			}
		}
	}
	return 0, fmt.Errorf("ps: btime not found in /proc/stat")
}

func procUptime(procPath string) (time.Duration, error) {
	data, err := readBoundedProcFile(filepath.Join(procPath, "uptime"), maxProcUptimeBytes)
	if err != nil {
		return 0, err
	}
	fields := strings.Fields(string(data))
	if len(fields) == 0 {
		return 0, fmt.Errorf("ps: uptime not found in /proc/uptime")
	}
	uptime, err := time.ParseDuration(fields[0] + "s")
	if err != nil || uptime < 0 {
		return 0, fmt.Errorf("ps: malformed /proc/uptime")
	}
	return uptime, nil
}

func readBoundedProcFile(path string, maxBytes int64) ([]byte, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	data, err := io.ReadAll(io.LimitReader(f, maxBytes+1))
	if err != nil {
		return nil, err
	}
	if int64(len(data)) > maxBytes {
		return nil, fmt.Errorf("data exceeds %d bytes", maxBytes)
	}
	return data, nil
}

func procMemTotalKiB(procPath string) (uint64, error) {
	f, err := os.Open(filepath.Join(procPath, "meminfo"))
	if err != nil {
		return 0, err
	}
	defer f.Close()
	scanner := bufio.NewScanner(f)
	scanner.Buffer(make([]byte, 4096), 1<<20)
	for scanner.Scan() {
		fields := strings.Fields(scanner.Text())
		if len(fields) >= 2 && fields[0] == "MemTotal:" {
			total, parseErr := strconv.ParseUint(fields[1], 10, 64)
			if parseErr != nil {
				return 0, parseErr
			}
			return total, nil
		}
	}
	if err := scanner.Err(); err != nil {
		return 0, err
	}
	return 0, fmt.Errorf("ps: MemTotal not found in /proc/meminfo")
}

func ticksToDuration(ticks int64) (time.Duration, bool) {
	const tickDuration = time.Second / clkTck
	if ticks < 0 || ticks > int64(^uint64(0)>>1)/int64(tickDuration) {
		return 0, false
	}
	return time.Duration(ticks) * tickDuration, true
}

func procStartTime(btime, startTicks int64, parseErr error) (time.Time, bool) {
	if btime <= 0 || parseErr != nil {
		return time.Time{}, false
	}
	offset, ok := ticksToDuration(startTicks)
	if !ok {
		return time.Time{}, false
	}
	offsetSeconds := int64(offset / time.Second)
	if btime > int64(^uint64(0)>>1)-offsetSeconds {
		return time.Time{}, false
	}
	return time.Unix(
		btime+offsetSeconds,
		int64(offset%time.Second),
	), true
}

// parseSID extracts the session ID (field 6 after comm) from /proc/pid/stat data.
func parseSID(data []byte) int {
	s := strings.TrimSpace(string(data))
	closeParen := strings.LastIndex(s, ")")
	if closeParen < 0 {
		return 0
	}
	rest := strings.Fields(s[closeParen+1:])
	// rest[3] = session
	if len(rest) >= 4 {
		sid, _ := strconv.Atoi(rest[3])
		return sid
	}
	return 0
}
