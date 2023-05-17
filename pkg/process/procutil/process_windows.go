// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build windows

package procutil

import (
	"bytes"
	"fmt"
	"strings"
	"time"

	"golang.org/x/sys/windows"

	"github.com/DataDog/datadog-agent/pkg/util/log"
	"github.com/DataDog/datadog-agent/pkg/util/winutil"
	"github.com/DataDog/datadog-agent/pkg/util/winutil/pdhutil"
)

var (
	counterPaths = []string{
		pdhutil.CounterAllProcessPID,
		pdhutil.CounterAllProcessParentPID,
		pdhutil.CounterAllProcessPctUserTime,
		pdhutil.CounterAllProcessPctPrivilegedTime,
		pdhutil.CounterAllProcessWorkingSet,
		pdhutil.CounterAllProcessPoolPagedBytes,
		pdhutil.CounterAllProcessThreadCount,
		pdhutil.CounterAllProcessHandleCount,
		pdhutil.CounterAllProcessIOReadOpsPerSec,
		pdhutil.CounterAllProcessIOWriteOpsPerSec,
		pdhutil.CounterAllProcessIOReadBytesPerSec,
		pdhutil.CounterAllProcessIOWriteBytesPerSec,
	}
)

// NewProcessProbe returns a Probe object
func NewProcessProbe(options ...Option) Probe {
	p := &probe{}
	p.init()
	return p
}

// probe implements Probe on Windows
type probe struct {
	hQuery    pdhutil.PDH_HQUERY
	counters  map[string]pdhutil.PDH_HCOUNTER
	formatter pdhutil.PdhFormatter
	enumSpecs map[string]counterEnumSpec
	initError error

	instanceToPID map[string]int32
	procs         map[int32]*Process
}

func (p *probe) init() {
	var err error

	defer func() {
		p.initError = err
		if err != nil {
			p.Close()
		}
	}()

	status := pdhutil.PdhOpenQuery(0, 0, &p.hQuery)
	if status != 0 {
		err = fmt.Errorf("PdhOpenQuery failed with 0x%x", status)
		return
	}

	p.counters = make(map[string]pdhutil.PDH_HCOUNTER)

	for _, path := range counterPaths {
		var hCounter pdhutil.PDH_HCOUNTER
		status = pdhutil.PdhAddEnglishCounter(p.hQuery, path, 0, &hCounter)
		if status != 0 {
			err = fmt.Errorf("PdhAddEnglishCounter for %s failed with 0x%x", path, status)
			return
		}
		p.counters[path] = hCounter
	}

	// Need to run PdhCollectQueryData once so that we have meaningful metrics on the first run
	err = pdhutil.PdhCollectQueryData(p.hQuery)
	if err != nil {
		return
	}

	p.procs = make(map[int32]*Process)
	p.initEnumSpecs()
	p.instanceToPID = make(map[string]int32)
}

type counterEnumSpec struct {
	format      uint32
	processMeta bool
	enumFunc    pdhutil.ValueEnumFunc
}

func (p *probe) initEnumSpecs() {
	p.enumSpecs = map[string]counterEnumSpec{
		pdhutil.CounterAllProcessParentPID: {
			format:      pdhutil.PDH_FMT_LARGE,
			processMeta: true,
			enumFunc:    valueToUint64(p.mapParentPID),
		},
		pdhutil.CounterAllProcessPctUserTime: {
			format:   pdhutil.PDH_FMT_DOUBLE,
			enumFunc: valueToFloat64(p.mapPctUserTime),
		},
		pdhutil.CounterAllProcessPctPrivilegedTime: {
			format:   pdhutil.PDH_FMT_DOUBLE,
			enumFunc: valueToFloat64(p.mapPctPrivilegedTime),
		},
		pdhutil.CounterAllProcessWorkingSet: {
			format:   pdhutil.PDH_FMT_LARGE,
			enumFunc: valueToUint64(p.mapWorkingSet),
		},
		pdhutil.CounterAllProcessPoolPagedBytes: {
			format:   pdhutil.PDH_FMT_LARGE,
			enumFunc: valueToUint64(p.mapPoolPagedBytes),
		},
		pdhutil.CounterAllProcessThreadCount: {
			format:   pdhutil.PDH_FMT_LARGE,
			enumFunc: valueToUint64(p.mapThreadCount),
		},
		pdhutil.CounterAllProcessHandleCount: {
			format:   pdhutil.PDH_FMT_LARGE,
			enumFunc: valueToUint64(p.mapHandleCount),
		},
		pdhutil.CounterAllProcessIOReadOpsPerSec: {
			format:   pdhutil.PDH_FMT_DOUBLE,
			enumFunc: valueToFloat64(p.mapIOReadOpsPerSec),
		},
		pdhutil.CounterAllProcessIOWriteOpsPerSec: {
			format:   pdhutil.PDH_FMT_DOUBLE,
			enumFunc: valueToFloat64(p.mapIOWriteOpsPerSec),
		},
		pdhutil.CounterAllProcessIOReadBytesPerSec: {
			format:   pdhutil.PDH_FMT_DOUBLE,
			enumFunc: valueToFloat64(p.mapIOReadBytesPerSec),
		},
		pdhutil.CounterAllProcessIOWriteBytesPerSec: {
			format:   pdhutil.PDH_FMT_DOUBLE,
			enumFunc: valueToFloat64(p.mapIOWriteBytesPerSec),
		},
	}
}

func valueToFloat64(fn func(string, float64)) pdhutil.ValueEnumFunc {
	return func(instance string, value pdhutil.PdhCounterValue) {
		fn(instance, value.Double)
	}
}

func valueToUint64(fn func(string, uint64)) pdhutil.ValueEnumFunc {
	return func(instance string, value pdhutil.PdhCounterValue) {
		fn(instance, uint64(value.Large))
	}
}

func (p *probe) Close() {
	if p.hQuery != pdhutil.PDH_HQUERY(0) {
		pdhutil.PdhCloseQuery(p.hQuery)
		p.hQuery = pdhutil.PDH_HQUERY(0)
	}
}

func (p *probe) StatsForPIDs(pids []int32, now time.Time) (map[int32]*Stats, error) {
	err := p.enumCounters(false, true)
	if err != nil {
		return nil, err
	}
	statsToReturn := make(map[int32]*Stats, len(pids))
	for _, pid := range pids {
		if proc, ok := p.procs[pid]; ok {
			statsToReturn[pid] = proc.Stats.DeepCopy()
		}
	}
	return statsToReturn, nil
}

func (p *probe) ProcessesByPID(now time.Time, collectStats bool) (map[int32]*Process, error) {
	// TODO: reuse PIDs slice across runs
	pids, err := getPIDs()
	if err != nil {
		return nil, err
	}

	knownPids := make(map[int32]struct{}, len(p.procs))
	for pid := range p.procs {
		knownPids[pid] = struct{}{}
	}

	for _, pid := range pids {
		if pid == 0 {
			// this is the "system idle process".  We'll never be able to open it,
			// which will cause us to thrash WMI once per check, which we don't
			// want to do
			continue
		}

		delete(knownPids, pid)

		if _, ok := p.procs[pid]; ok {
			// Process already known, no need to collect metadata
			continue
		}

		proc := &Process{
			Pid: int32(pid),
			Stats: &Stats{
				CPUPercent:  &CPUPercentStat{},
				MemInfo:     &MemoryInfoStat{},
				CtxSwitches: &NumCtxSwitchesStat{},
				IORateStat:  &IOCountersRateStat{},
			},
		}

		err := fillProcessDetails(pid, proc)

		if err != nil {
			continue
		}

		p.procs[pid] = proc
	}

	for pid := range knownPids {
		proc := p.procs[pid]
		log.Debugf("removing process %v %v", pid, proc.Exe)
		delete(p.procs, pid)
	}

	err = p.enumCounters(true, collectStats)
	if err != nil {
		return nil, err
	}

	procsToReturn := make(map[int32]*Process, len(p.procs))

	for pid, proc := range p.procs {
		procsToReturn[pid] = proc.DeepCopy()
	}
	return procsToReturn, nil
}

func (p *probe) enumCounters(collectMeta bool, collectStats bool) error {
	// Reuse map's capacity across runs
	for k := range p.instanceToPID {
		delete(p.instanceToPID, k)
	}

	err := pdhutil.PdhCollectQueryData(p.hQuery)
	if err != nil {
		return err
	}

	ignored := []string{
		"_Total", // Total sum
		"Idle",   // System Idle process
	}

	err = p.formatter.Enum(pdhutil.CounterAllProcessPID, p.counters[pdhutil.CounterAllProcessPID], pdhutil.PDH_FMT_LARGE, ignored, valueToUint64(p.mapPID))

	if err != nil {
		return err
	}

	// handle case when instanceToPID does not contain some previously collected process PIDs
	missingPids := make(map[int32]struct{})
	for _, pid := range p.instanceToPID {
		if _, ok := p.procs[pid]; !ok {
			missingPids[pid] = struct{}{}
		}
	}

	for pid := range missingPids {
		delete(p.procs, pid)
	}

	for counter, spec := range p.enumSpecs {
		if spec.processMeta && !collectMeta ||
			!spec.processMeta && !collectStats {
			continue
		}
		err := p.formatter.Enum(counter, p.counters[counter], spec.format, ignored, spec.enumFunc)
		if err != nil {
			return err
		}
	}

	return nil
}

func (p *probe) StatsWithPermByPID(pids []int32) (map[int32]*StatsWithPerm, error) {
	return nil, fmt.Errorf("probe(Windows): StatsWithPermByPID is not implemented")
}

func (p *probe) getProc(instance string) *Process {
	pid, ok := p.instanceToPID[instance]
	if !ok {
		log.Debugf("proc - no pid for instance %s", instance)
		return nil
	}

	proc, ok := p.procs[pid]
	if !ok {
		log.Debugf("proc - no process for pid %d (instance=%s)", pid, instance)
		return nil
	}
	return proc
}

func (p *probe) mapToProc(instance string, fn func(proc *Process)) {
	if proc := p.getProc(instance); proc != nil {
		fn(proc)
	}
}

func (p *probe) mapToStatFloat64(instance string, v float64, fn func(pid int32, proc *Stats, instance string, v float64)) {
	if proc := p.getProc(instance); proc != nil {
		fn(proc.Pid, proc.Stats, instance, v)
	}
}

func (p *probe) mapToStatUint64(instance string, v uint64, fn func(pid int32, proc *Stats, instance string, v uint64)) {
	if proc := p.getProc(instance); proc != nil {
		fn(proc.Pid, proc.Stats, instance, v)
	}
}

func (p *probe) mapPID(instance string, pid uint64) {
	p.instanceToPID[instance] = int32(pid)
}

func (p *probe) setProcParentPID(proc *Process, instance string, pid int32) {
	proc.Ppid = pid
}

func (p *probe) mapParentPID(instance string, v uint64) {
	p.mapToProc(instance, func(proc *Process) {
		p.setProcParentPID(proc, instance, int32(v))
	})
}

func (p *probe) traceStats(pid int32) bool {
	// TODO: in a future PR introduce an Option to configure tracing of stats for individual PIDs
	return false
}

func (p *probe) setProcOpenFdCount(pid int32, stats *Stats, instance string, v uint64) {
	if p.traceStats(pid) {
		log.Tracef("FdCount[%s,pid=%d] %d", instance, pid, v)
	}
	stats.OpenFdCount = int32(v)
}

func (p *probe) mapHandleCount(instance string, v uint64) {
	p.mapToStatUint64(instance, v, p.setProcOpenFdCount)
}

func (p *probe) setProcNumThreads(pid int32, stats *Stats, instance string, v uint64) {
	if p.traceStats(pid) {
		log.Tracef("NumThreads[%s,pid=%d] %d", instance, pid, v)
	}
	stats.NumThreads = int32(v)
}

func (p *probe) mapThreadCount(instance string, v uint64) {
	p.mapToStatUint64(instance, v, p.setProcNumThreads)
}

func (p *probe) setProcCPUTimeUser(pid int32, stats *Stats, instance string, v float64) {
	if p.traceStats(pid) {
		log.Tracef("CPU.User[%s,pid=%d] %f", instance, pid, v)
	}
	stats.CPUPercent.UserPct = v
}

func (p *probe) mapPctUserTime(instance string, v float64) {
	p.mapToStatFloat64(instance, v, p.setProcCPUTimeUser)
}

func (p *probe) setProcCPUTimeSystem(pid int32, stats *Stats, instance string, v float64) {
	if p.traceStats(pid) {
		log.Tracef("CPU.System[%s,pid=%d] %f", instance, pid, v)
	}
	stats.CPUPercent.SystemPct = v
}

func (p *probe) mapPctPrivilegedTime(instance string, v float64) {
	p.mapToStatFloat64(instance, v, p.setProcCPUTimeSystem)
}

func (p *probe) setProcMemRSS(pid int32, stats *Stats, instance string, v uint64) {
	if p.traceStats(pid) {
		log.Tracef("Mem.RSS[%s,pid=%d] %d", instance, pid, v)
	}
	stats.MemInfo.RSS = v
}

func (p *probe) mapWorkingSet(instance string, v uint64) {
	p.mapToStatUint64(instance, v, p.setProcMemRSS)
}

func (p *probe) setProcMemVMS(pid int32, stats *Stats, instance string, v uint64) {
	if p.traceStats(pid) {
		log.Tracef("Mem.VMS[%s,pid=%d] %d", instance, pid, v)
	}
	stats.MemInfo.VMS = v
}

func (p *probe) mapPoolPagedBytes(instance string, v uint64) {
	p.mapToStatUint64(instance, v, p.setProcMemVMS)
}

func (p *probe) setProcIOReadOpsRate(pid int32, stats *Stats, instance string, v float64) {
	if p.traceStats(pid) {
		log.Tracef("ReadRate[%s,pid=%d] %f", instance, pid, v)
	}
	stats.IORateStat.ReadRate = v
}

func (p *probe) mapIOReadOpsPerSec(instance string, v float64) {
	p.mapToStatFloat64(instance, v, p.setProcIOReadOpsRate)
}

func (p *probe) setProcIOWriteOpsRate(pid int32, stats *Stats, instance string, v float64) {
	if p.traceStats(pid) {
		log.Tracef("WriteRate[%s,pid=%d] %f", instance, pid, v)
	}
	stats.IORateStat.WriteRate = v
}

func (p *probe) mapIOWriteOpsPerSec(instance string, v float64) {
	p.mapToStatFloat64(instance, v, p.setProcIOWriteOpsRate)
}

func (p *probe) setProcIOReadBytesRate(pid int32, stats *Stats, instance string, v float64) {
	if p.traceStats(pid) {
		log.Tracef("ReadBytesRate[%s,pid=%d] %f", instance, pid, v)
	}
	stats.IORateStat.ReadBytesRate = v
}

func (p *probe) mapIOReadBytesPerSec(instance string, v float64) {
	p.mapToStatFloat64(instance, v, p.setProcIOReadBytesRate)
}

func (p *probe) setProcIOWriteBytesRate(pid int32, stats *Stats, instance string, v float64) {
	if p.traceStats(pid) {
		log.Tracef("WriteBytesRate[%s,pid=%d] %f", instance, pid, v)
	}
	stats.IORateStat.WriteBytesRate = v
}

func (p *probe) mapIOWriteBytesPerSec(instance string, v float64) {
	p.mapToStatFloat64(instance, v, p.setProcIOWriteBytesRate)
}

func getPIDs() ([]int32, error) {
	var read uint32
	var psSize uint32 = 1024

	for {
		buf := make([]uint32, psSize)
		if err := windows.EnumProcesses(buf, &read); err != nil {
			return nil, err
		}
		if uint32(len(buf)) == read {
			psSize += 1024
			continue
		}
		pids := make([]int32, read)
		for i := range pids {
			pids[i] = int32(buf[i])
		}
		return pids, nil
	}
}

func fillProcessDetails(pid int32, proc *Process) error {
	procHandle, err := OpenProcessHandle(pid)
	if err != nil {
		return err
	}
	defer windows.Close(procHandle)

	userName, usererr := GetUsernameForProcess(procHandle)
	if usererr != nil {
		log.Debugf("Couldn't get process username %v %v", pid, err)
	}
	proc.Username = userName

	cmdParams := getProcessCommandParams(procHandle)

	proc.Cmdline = ParseCmdLineArgs(cmdParams.CmdLine)
	if len(cmdParams.CmdLine) > 0 && len(proc.Cmdline) == 0 {
		log.Warnf("Failed to parse the cmdline:%s for pid:%d", cmdParams.CmdLine, pid)
	}

	proc.Exe = cmdParams.ImagePath

	var CPU windows.Rusage
	if err := windows.GetProcessTimes(procHandle, &CPU.CreationTime, &CPU.ExitTime, &CPU.KernelTime, &CPU.UserTime); err != nil {
		log.Errorf("Could not get process times for %v %v", pid, err)
		return err
	}

	ctime := CPU.CreationTime.Nanoseconds() / 1000000
	proc.Stats.CreateTime = ctime
	return nil
}

func getProcessCommandParams(procHandle windows.Handle) *winutil.ProcessCommandParams {
	var err error
	if cmdParams, err := winutil.GetCommandParamsForProcess(procHandle, true); err == nil {
		return cmdParams
	}

	log.Debugf("Error retrieving command params %v", err)
	if imagePath, err := winutil.GetImagePathForProcess(procHandle); err == nil {
		return &winutil.ProcessCommandParams{
			CmdLine:   imagePath,
			ImagePath: imagePath,
		}
	}

	log.Debugf("Error retrieving exe path %v", err)
	return &winutil.ProcessCommandParams{}
}

// OpenProcessHandle attempts to open process handle for reading process memory with fallback to query basic info
func OpenProcessHandle(pid int32) (windows.Handle, error) {
	// 0x1000 is PROCESS_QUERY_LIMITED_INFORMATION, but that constant isn't
	//        defined in x/sys/windows
	// 0x10   is PROCESS_VM_READ
	procHandle, err := windows.OpenProcess(0x1010, false, uint32(pid))
	if err != nil {
		log.Debugf("Couldn't open process with PROCESS_VM_READ %v %v", pid, err)
		procHandle, err = windows.OpenProcess(0x1000, false, uint32(pid))
		if err != nil {
			log.Debugf("Couldn't open process %v %v", pid, err)
			return windows.Handle(0), err
		}
	}
	return procHandle, nil
}

// GetUsernameForProcess returns username for a process
func GetUsernameForProcess(h windows.Handle) (name string, err error) {
	err = nil
	var t windows.Token
	err = windows.OpenProcessToken(h, windows.TOKEN_QUERY, &t)
	if err != nil {
		log.Debugf("Failed to open process token %v", err)
		return
	}
	defer t.Close()
	tokenUser, err := t.GetTokenUser()

	user, domain, _, err := tokenUser.User.Sid.LookupAccount("")
	if nil != err {
		return "", err
	}
	return domain + "\\" + user, err
}

// ParseCmdLineArgs parses command line arguments to a slice
func ParseCmdLineArgs(cmdline string) (res []string) {
	blocks := strings.Split(cmdline, " ")
	findCloseQuote := false
	donestring := false

	var stringInProgress bytes.Buffer
	for _, b := range blocks {
		numquotes := strings.Count(b, "\"")
		if numquotes == 0 {
			stringInProgress.WriteString(b)
			if !findCloseQuote {
				donestring = true
			} else {
				stringInProgress.WriteString(" ")
			}

		} else if numquotes == 1 {
			stringInProgress.WriteString(b)
			if findCloseQuote {
				donestring = true
			} else {
				findCloseQuote = true
				stringInProgress.WriteString(" ")
			}

		} else if numquotes == 2 {
			stringInProgress.WriteString(b)
			donestring = true
		} else {
			log.Warnf("unexpected quotes in string, giving up (%v)", cmdline)
			return res
		}

		if donestring {
			res = append(res, stringInProgress.String())
			stringInProgress.Reset()
			findCloseQuote = false
			donestring = false
		}
	}

	return res
}
