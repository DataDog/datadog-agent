// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build darwin

package procinfo

import (
	"context"
	"encoding/binary"
	"fmt"
	"math/bits"
	"os"
	"syscall"
	"time"
	"unsafe"

	"golang.org/x/sys/unix"
)

// procInfoCallPidInfo and procPidTaskInfo select proc_pidinfo's
// PROC_PIDTASKINFO flavor through the raw SYS_PROC_INFO trap. This is the same
// XNU interface used by the Darwin procmaps backend; x/sys/unix does not wrap
// it.
const (
	procInfoCallPidInfo = 2
	procPidTaskInfo     = 4
	procTaskInfoSize    = 96
)

// darwinTaskInfo is the subset of XNU's proc_taskinfo used by ps. The kernel
// values are decoded explicitly rather than relying on a Go struct layout.
type darwinTaskInfo struct {
	virtualSize  uint64
	residentSize uint64
	totalUser    uint64
	totalSystem  uint64
}

type darwinMetricContext struct {
	now               time.Time
	totalMemoryBytes  uint64
	timebaseFrequency uint64
}

func listAll(ctx context.Context, _ string, metrics Metrics) ([]ProcInfo, error) {
	kprocs, err := unix.SysctlKinfoProcSlice("kern.proc.all")
	if err != nil {
		return nil, fmt.Errorf("ps: SysctlKinfoProcSlice: %w", err)
	}

	metricCtx := newDarwinMetricContext(metrics)
	procs := make([]ProcInfo, 0, min(len(kprocs), MaxProcesses))
	for i := range kprocs {
		if ctx.Err() != nil {
			break
		}
		if len(procs) >= MaxProcesses {
			break
		}
		info := kinfoToProc(&kprocs[i])
		if info.PID == 0 {
			continue
		}
		populateDarwinMetrics(&info, metrics, metricCtx, readDarwinTaskInfoBestEffort(info.PID, metrics))
		procs = append(procs, info)
	}
	return procs, nil
}

func getSession(ctx context.Context, procPath string, metrics Metrics) ([]ProcInfo, error) {
	all, err := listAll(ctx, procPath, metrics)
	if err != nil {
		return nil, err
	}

	// Build ancestor chain via PPID.
	byPID := make(map[int]ProcInfo, len(all))
	for _, p := range all {
		byPID[p.PID] = p
	}

	selfPID := os.Getpid()
	ancestors := collectAncestorPIDs(ctx, byPID, selfPID, 1)

	// Include all processes in the same session (getsid).
	selfSID, err := syscall.Getsid(0)
	if err != nil {
		selfSID = 0
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
			if _, err := unix.SysctlKinfoProc("kern.proc.pid", p.PID); err == nil {
				sid, serr := syscall.Getsid(p.PID)
				if serr == nil && sid == selfSID {
					result = append(result, p)
				}
			}
		}
	}
	return result, nil
}

func getByPIDs(ctx context.Context, _ string, pids []int, metrics Metrics) ([]ProcInfo, error) {
	metricCtx := newDarwinMetricContext(metrics)
	var result []ProcInfo
	for _, pid := range pids {
		if ctx.Err() != nil {
			break
		}
		kp, err := unix.SysctlKinfoProc("kern.proc.pid", pid)
		if err != nil {
			continue
		}
		info := kinfoToProc(kp)
		if info.PID == 0 {
			continue
		}
		populateDarwinMetrics(&info, metrics, metricCtx, readDarwinTaskInfoBestEffort(info.PID, metrics))
		result = append(result, info)
	}
	return result, nil
}

func kinfoToProc(kp *unix.KinfoProc) ProcInfo {
	pid := int(kp.Proc.P_pid)
	ppid := int(kp.Eproc.Ppid)
	uid := fmt.Sprintf("%d", kp.Eproc.Ucred.Uid)
	state := string([]byte{statByte(kp.Proc.P_stat)})
	tty := resolveTTY(kp.Eproc.Tdev)

	// Start time.
	startSec := kp.Proc.P_starttime.Sec
	startNsec := kp.Proc.P_starttime.Usec * 1000
	startTime := time.Unix(startSec, int64(startNsec))

	return ProcInfo{
		PID:       pid,
		PPID:      ppid,
		UID:       uid,
		State:     state,
		TTY:       tty,
		CPU:       0,
		STime:     formatStartTime(startTime, time.Now()),
		Time:      "-",
		Cmd:       darwinCommName(kp.Proc.P_comm[:]),
		StartTime: startTime,
	}
}

func newDarwinMetricContext(metrics Metrics) darwinMetricContext {
	metricCtx := darwinMetricContext{now: time.Now()}
	if metrics.Has(MetricPMem) {
		metricCtx.totalMemoryBytes, _ = unix.SysctlUint64("hw.memsize")
	}
	if metrics.Has(MetricCPUTime) || metrics.Has(MetricPCPU) {
		// proc_taskinfo reports CPU counters in Mach timebase units.
		// hw.tbfrequency is that counter's ticks-per-second frequency, so it
		// provides the required conversion without introducing a Mach RPC or
		// dynamic libSystem call.
		metricCtx.timebaseFrequency, _ = unix.SysctlUint64("hw.tbfrequency")
	}
	return metricCtx
}

func readDarwinTaskInfoBestEffort(pid int, metrics Metrics) *darwinTaskInfo {
	taskMetrics := MetricCPUTime | MetricRSS | MetricVSZ | MetricPMem | MetricPCPU
	if metrics&taskMetrics == 0 {
		return nil
	}
	taskInfo, err := readDarwinTaskInfo(pid)
	if err != nil {
		// proc_pidinfo commonly returns EPERM for processes owned by another
		// user, and a process can disappear between enumeration and this
		// call. Preserve the base kinfo row and leave only these metrics
		// unavailable.
		return nil
	}
	return &taskInfo
}

func readDarwinTaskInfo(pid int) (darwinTaskInfo, error) {
	var buf [procTaskInfoSize]byte
	n, _, errno := syscall.Syscall6(
		uintptr(syscall.SYS_PROC_INFO),
		procInfoCallPidInfo,
		uintptr(pid),
		procPidTaskInfo,
		0,
		uintptr(unsafe.Pointer(&buf[0])),
		uintptr(len(buf)),
	)
	if errno != 0 {
		return darwinTaskInfo{}, errno
	}
	if n != procTaskInfoSize {
		return darwinTaskInfo{}, fmt.Errorf("unexpected proc_taskinfo size: got %d, want %d", n, procTaskInfoSize)
	}
	return decodeDarwinTaskInfo(buf[:])
}

func decodeDarwinTaskInfo(buf []byte) (darwinTaskInfo, error) {
	if len(buf) < procTaskInfoSize {
		return darwinTaskInfo{}, fmt.Errorf("short proc_taskinfo: got %d bytes, want %d", len(buf), procTaskInfoSize)
	}
	return darwinTaskInfo{
		virtualSize:  binary.LittleEndian.Uint64(buf[0:8]),
		residentSize: binary.LittleEndian.Uint64(buf[8:16]),
		totalUser:    binary.LittleEndian.Uint64(buf[16:24]),
		totalSystem:  binary.LittleEndian.Uint64(buf[24:32]),
	}, nil
}

func populateDarwinMetrics(info *ProcInfo, metrics Metrics, metricCtx darwinMetricContext, taskInfo *darwinTaskInfo) {
	startValid := !info.StartTime.IsZero() && info.StartTime.Unix() > 0
	if metrics.Has(MetricStartTime) && startValid {
		info.Available |= MetricStartTime
	}

	elapsedValid := startValid && !metricCtx.now.Before(info.StartTime)
	if elapsedValid {
		info.Elapsed = metricCtx.now.Sub(info.StartTime)
		if metrics.Has(MetricElapsed) {
			info.Available |= MetricElapsed
		}
	}

	if taskInfo == nil {
		return
	}

	info.RSSKiB = taskInfo.residentSize / 1024
	info.VSZKiB = taskInfo.virtualSize / 1024
	if metrics.Has(MetricRSS) {
		info.Available |= MetricRSS
	}
	if metrics.Has(MetricVSZ) {
		info.Available |= MetricVSZ
	}
	if metrics.Has(MetricPMem) && metricCtx.totalMemoryBytes > 0 {
		info.PMem = float64(taskInfo.residentSize) * 100 / float64(metricCtx.totalMemoryBytes)
		info.Available |= MetricPMem
	}

	cpuTicks := taskInfo.totalUser + taskInfo.totalSystem
	if cpuTicks < taskInfo.totalUser {
		return
	}
	cpuTime, ok := darwinTicksToDuration(cpuTicks, metricCtx.timebaseFrequency)
	if !ok {
		return
	}
	info.CPUTime = cpuTime
	info.Time = formatCPUTime(cpuTime)
	if metrics.Has(MetricCPUTime) {
		info.Available |= MetricCPUTime
	}
	if metrics.Has(MetricPCPU) && elapsedValid && info.Elapsed > 0 {
		info.PCPU = float64(cpuTime) * 100 / float64(info.Elapsed)
		info.CPU = boundedCPUInteger(info.PCPU)
		info.Available |= MetricPCPU
	}
}

// darwinTicksToDuration converts Mach timebase ticks using the system's
// ticks-per-second frequency without overflowing intermediate arithmetic.
func darwinTicksToDuration(ticks, frequency uint64) (time.Duration, bool) {
	if frequency == 0 {
		return 0, false
	}

	const nanosPerSecond = uint64(time.Second)
	const maxDuration = uint64(1<<63 - 1)

	seconds := ticks / frequency
	if seconds > maxDuration/nanosPerSecond {
		return 0, false
	}
	totalNanos := seconds * nanosPerSecond

	high, low := bits.Mul64(ticks%frequency, nanosPerSecond)
	fractionNanos, _ := bits.Div64(high, low, frequency)
	if fractionNanos > maxDuration-totalNanos {
		return 0, false
	}
	return time.Duration(totalNanos + fractionNanos), true
}

func darwinCommName(comm []byte) string {
	n := 0
	for n < len(comm) && comm[n] != 0 {
		n++
	}
	return truncateCmdName(string(comm[:n]))
}

// statByte converts the Darwin p_stat value to a single-character state.
func statByte(stat int8) byte {
	switch stat {
	case 1: // SIDL
		return 'I'
	case 2: // SRUN
		return 'R'
	case 3: // SSLEEP
		return 'S'
	case 4: // SSTOP
		return 'T'
	case 5: // SZOMB
		return 'Z'
	default:
		return '?'
	}
}

// resolveTTY returns a human-readable TTY name from a Darwin dev_t.
func resolveTTY(tdev int32) string {
	if tdev == 0 || tdev == -1 {
		return "?"
	}
	// Major/minor encoding differs on macOS. Return numeric form.
	return fmt.Sprintf("%d", tdev)
}
