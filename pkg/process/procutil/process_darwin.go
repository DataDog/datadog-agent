// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build darwin

package procutil

import (
	"bytes"
	"errors"
	"fmt"
	"os/exec"
	"strconv"
	"strings"
	"time"

	"golang.org/x/sys/unix"
)

const (
	clockTicks = 100 // C.sysconf(C._SC_CLK_TCK)
)

// NewProcessProbe returns a Probe object
func NewProcessProbe(options ...Option) Probe {
	p := &probe{}
	for _, option := range options {
		option(p)
	}
	return p
}

// probe is an implementation of the process probe for macOS
type probe struct {
}

func (p *probe) Close() {}

func (p *probe) StatsForPIDs(_ []int32, _ time.Time) (map[int32]*Stats, error) {
	procs, err := p.ProcessesByPID(time.Now(), false)
	if err != nil {
		return nil, err
	}
	stats := make(map[int32]*Stats, len(procs))
	for pid, proc := range procs {
		stats[pid] = proc.Stats
	}
	return stats, nil
}

func (p *probe) ProcessesByPID(_ time.Time, _ bool) (map[int32]*Process, error) {
	return allProcesses()
}

func (p *probe) StatsWithPermByPID(_ []int32) (map[int32]*StatsWithPerm, error) {
	return nil, errors.New("StatsWithPermByPID is not implemented in this environment")
}

var allProcessFields = "pid,ppid,utime,stime,etime,state,rss,vsize,pagein,command"

// allProcesses collects all running processes via a single ps(1) invocation
// (for CPU/memory/cmdline data) combined with per-process sysctl(kern.proc.pid)
// calls (for name, UIDs, GIDs, nice). ps runs SUID on macOS and can therefore
// read data for processes owned by other users, the sysctl is world-readable.
func allProcesses() (map[int32]*Process, error) {
	result, err := callPs(allProcessFields, 0, false)
	if err != nil {
		return nil, err
	}

	kprocByPid := make(map[int32]*unix.KinfoProc)
	for _, r := range result {
		ipid, err := strconv.Atoi(r[0])
		if err != nil {
			return nil, fmt.Errorf("pid: %s", err)
		}
		pid := int32(ipid)
		kp, err := unix.SysctlKinfoProc("kern.proc.pid", int(pid))
		if err != nil {
			// The process may have exited between the ps(1) call and this sysctl
			// call (TOCTOU race).  On macOS the kernel returns success with 0
			// bytes written rather than ESRCH, which unix.SysctlKinfoProc
			// converts to EIO.
			if errors.Is(err, unix.EIO) || errors.Is(err, unix.ESRCH) {
				continue
			}
			return nil, fmt.Errorf("kproc: %w", err)
		}
		kprocByPid[pid] = kp
	}

	procs := make(map[int32]*Process)
	for _, r := range result {
		ipid, err := strconv.Atoi(r[0])
		if err != nil {
			return nil, fmt.Errorf("pid: %s", err)
		}
		pid := int32(ipid)
		k, ok := kprocByPid[pid]
		if !ok {
			// Skip processes where we don't have a kproc.
			// These would only happen for very short-lived processes.
			continue
		}
		ppid, err := strconv.Atoi(r[1])
		if err != nil {
			return nil, fmt.Errorf("ppid: %s", err)
		}
		utime, stime, err := makeTimeStat(r[2], r[3])
		if err != nil {
			return nil, fmt.Errorf("times: %s", err)
		}
		createTime, err := formatElapsedTime(r[4])
		if err != nil {
			return nil, fmt.Errorf("etime: %s", err)
		}
		rss, err := strconv.Atoi(r[6])
		if err != nil {
			return nil, err
		}
		vms, err := strconv.Atoi(r[7])
		if err != nil {
			return nil, err
		}
		pagein, err := strconv.Atoi(r[8])
		if err != nil {
			return nil, err
		}
		procs[pid] = &Process{
			Pid:     pid,
			Ppid:    int32(ppid),
			Name:    bytesToString(k.Proc.P_comm[:]),
			Cmdline: r[9:],
			Uids:    []int32{int32(k.Eproc.Ucred.Uid)},
			Gids: []int32{
				int32(k.Eproc.Pcred.P_rgid),
				int32(k.Eproc.Ucred.Ngroups),
				int32(k.Eproc.Pcred.P_svgid),
			},
			Stats: &Stats{
				CreateTime:  createTime,
				Status:      r[5],
				Nice:        int32(k.Proc.P_nice),
				CPUTime:     &CPUTimesStat{User: utime, System: stime},
				MemInfo:     &MemoryInfoStat{RSS: uint64(rss) * 1024, VMS: uint64(vms) * 1024, Swap: uint64(pagein)},
				IOStat:      &IOCountersStat{},
				CtxSwitches: &NumCtxSwitchesStat{},
			},
		}
	}

	return procs, nil
}

// bytesToString converts a null-terminated byte slice to a string
func bytesToString(orig []byte) string {
	if index := bytes.IndexByte(orig, 0); index != -1 {
		return string(orig[:index])
	}
	return string(orig)
}

// callPs runs the ps command and returns the output split into fields.
// If pid is 0, returns data for all processes.
func callPs(arg string, pid int32, threadOption bool) ([][]string, error) {
	bin, err := exec.LookPath("ps")
	if err != nil {
		return [][]string{}, err
	}

	var cmd []string
	if pid == 0 {
		cmd = []string{"axwww", "-o", arg}
	} else if threadOption {
		cmd = []string{"-x", "-o", arg, "-M", "-p", strconv.Itoa(int(pid))}
	} else {
		cmd = []string{"-x", "-o", arg, "-p", strconv.Itoa(int(pid))}
	}
	out, err := exec.Command(bin, cmd...).Output()
	if err != nil {
		return [][]string{}, err
	}
	lines := strings.Split(string(out), "\n")

	var ret [][]string
	for _, l := range lines[1:] {
		var lr []string
		for _, r := range strings.Split(l, " ") {
			if r == "" {
				continue
			}
			lr = append(lr, strings.TrimSpace(r))
		}
		if len(lr) != 0 {
			ret = append(ret, lr)
		}
	}

	return ret, nil
}

// convertCPUTimes converts a ps CPU time string (e.g. "1:23.45") to seconds.
// Vendored from github.com/DataDog/gopsutil/process.convertCPUTimes.
func convertCPUTimes(s string) (ret float64, err error) {
	var t int
	var _tmp string
	if strings.Contains(s, ":") {
		_t := strings.Split(s, ":")
		hour, err := strconv.Atoi(_t[0])
		if err != nil {
			return ret, err
		}
		t += hour * 60 * 100
		_tmp = _t[1]
	} else {
		_tmp = s
	}

	_t := strings.Split(_tmp, ".")
	h, err := strconv.Atoi(_t[0])
	if err != nil {
		return ret, err
	}
	t += h * 100
	h, err = strconv.Atoi(_t[1])
	if err != nil {
		return ret, err
	}
	t += h
	return float64(t) / clockTicks, nil
}

// makeTimeStat converts ps utime and stime strings to float64 seconds each.
// Vendored from github.com/DataDog/gopsutil/process.makeTimeStat.
func makeTimeStat(strUtime, strStime string) (float64, float64, error) {
	utime, err := convertCPUTimes(strUtime)
	if err != nil {
		return 0, 0, err
	}
	stime, err := convertCPUTimes(strStime)
	if err != nil {
		return 0, 0, err
	}
	return utime, stime, nil
}

// formatElapsedTime converts a ps elapsed time string to a Unix millisecond timestamp.
// Vendored from github.com/DataDog/gopsutil/process.formatElapsedTime.
func formatElapsedTime(etime string) (int64, error) {
	elapsedSegments := strings.Split(strings.Replace(etime, "-", ":", 1), ":")
	var elapsedDurations []time.Duration
	for i := len(elapsedSegments) - 1; i >= 0; i-- {
		p, err := strconv.ParseInt(elapsedSegments[i], 10, 0)
		if err != nil {
			return 0, err
		}
		elapsedDurations = append(elapsedDurations, time.Duration(p))
	}

	elapsed := time.Duration(elapsedDurations[0]) * time.Second
	if len(elapsedDurations) > 1 {
		elapsed += time.Duration(elapsedDurations[1]) * time.Minute
	}
	if len(elapsedDurations) > 2 {
		elapsed += time.Duration(elapsedDurations[2]) * time.Hour
	}
	if len(elapsedDurations) > 3 {
		elapsed += time.Duration(elapsedDurations[3]) * time.Hour * 24
	}

	start := time.Now().Add(-elapsed)
	return start.Unix() * 1000, nil
}
