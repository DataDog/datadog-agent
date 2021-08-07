// +build windows

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
	status = pdhutil.PdhCollectQueryData(p.hQuery)
	if status != 0 {
		err = fmt.Errorf("PdhCollectQueryData failed with 0x%x", status)
		return
	}

	p.procs = make(map[int32]*Process)
	p.initEnumSpecs()
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
			enumFunc:    p.mapParentPID,
		},
		pdhutil.CounterAllProcessPctUserTime: {
			format:   pdhutil.PDH_FMT_DOUBLE | pdhutil.PDH_FMT_NOCAP100,
			enumFunc: p.mapPctUserTime,
		},
		pdhutil.CounterAllProcessPctPrivilegedTime: {
			format:   pdhutil.PDH_FMT_DOUBLE | pdhutil.PDH_FMT_NOCAP100,
			enumFunc: p.mapPctPrivilegedTime,
		},
		pdhutil.CounterAllProcessWorkingSet: {
			format:   pdhutil.PDH_FMT_LARGE,
			enumFunc: p.mapWorkingSet,
		},
		pdhutil.CounterAllProcessPoolPagedBytes: {
			format:   pdhutil.PDH_FMT_LARGE,
			enumFunc: p.mapPoolPagedBytes,
		},
		pdhutil.CounterAllProcessThreadCount: {
			format:   pdhutil.PDH_FMT_LARGE,
			enumFunc: p.mapThreadCount,
		},
		pdhutil.CounterAllProcessHandleCount: {
			format:   pdhutil.PDH_FMT_LARGE,
			enumFunc: p.mapHandleCount,
		},
		pdhutil.CounterAllProcessIOReadOpsPerSec: {
			format:   pdhutil.PDH_FMT_DOUBLE,
			enumFunc: p.mapIOReadOpsPerSec,
		},
		pdhutil.CounterAllProcessIOWriteOpsPerSec: {
			format:   pdhutil.PDH_FMT_DOUBLE,
			enumFunc: p.mapIOWriteOpsPerSec,
		},
		pdhutil.CounterAllProcessIOReadBytesPerSec: {
			format:   pdhutil.PDH_FMT_DOUBLE,
			enumFunc: p.mapIOReadBytesPerSec,
		},
		pdhutil.CounterAllProcessIOWriteBytesPerSec: {
			format:   pdhutil.PDH_FMT_DOUBLE,
			enumFunc: p.mapIOWriteBytesPerSec,
		},
	}
}

func (p *probe) Close() {
	if p.hQuery != pdhutil.PDH_HQUERY(0) {
		pdhutil.PdhCloseQuery(p.hQuery)
		p.hQuery = pdhutil.PDH_HQUERY(0)
	}
}

func (p *probe) StatsForPIDs(pids []int32, now time.Time) (map[int32]*Stats, error) {
	err := p.enumCounters(false)
	if err != nil {
		return nil, err
	}
	statsToReturn := make(map[int32]*Stats)
	for _, pid := range pids {
		if proc, ok := p.procs[pid]; ok {
			statsToReturn[pid] = proc.Stats.DeepCopy()
		}
	}
	return statsToReturn, nil
}

func (p *probe) ProcessesByPID(now time.Time) (map[int32]*Process, error) {
	pids, err := getPIDs()
	if err != nil {
		return nil, err
	}

	knownPids := make(map[int32]struct{})
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
				CPUTime:     &CPUTimesStat{},
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

	err = p.enumCounters(true)
	if err != nil {
		return nil, err
	}

	procsToReturn := make(map[int32]*Process)

	for pid, proc := range p.procs {
		procsToReturn[pid] = proc.DeepCopy()
	}
	return procsToReturn, nil
}

func (p *probe) enumCounters(includeProcMeta bool) error {
	p.instanceToPID = make(map[string]int32)

	status := pdhutil.PdhCollectQueryData(p.hQuery)
	if status != 0 {
		return fmt.Errorf("PdhCollectQueryData failed with 0x%x", status)
	}

	err := p.formatter.Enum(p.counters[pdhutil.CounterAllProcessPID], pdhutil.PDH_FMT_LARGE, p.mapPID)
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
		if spec.processMeta && !includeProcMeta {
			continue
		}
		err := p.formatter.Enum(p.counters[counter], spec.format, spec.enumFunc)
		if err != nil {
			return err
		}
	}

	return nil
}

func (p *probe) StatsWithPermByPID(pids []int32) (map[int32]*StatsWithPerm, error) {
	return nil, fmt.Errorf("probe(Windows): StatsWithPermByPID is not implemented")
}

func (p *probe) mapToProc(instance string, fn func(proc *Process)) {
	pid, ok := p.instanceToPID[instance]
	if !ok {
		// TODO: log
		return
	}

	proc, ok := p.procs[pid]
	if !ok {
		// TODO: log
		return
	}

	fn(proc)
}

func (p *probe) mapToStat(instance string, fn func(proc *Stats)) {
	pid, ok := p.instanceToPID[instance]
	if !ok {
		// TODO: log
		return
	}

	proc, ok := p.procs[pid]
	if !ok {
		// TODO: log
		return
	}

	fn(proc.Stats)
}

func (p *probe) mapPID(instance string, value pdhutil.PdhCounterValue) {
	p.instanceToPID[instance] = int32(value.Large)
}

func (p *probe) mapParentPID(instance string, value pdhutil.PdhCounterValue) {
	p.mapToProc(instance, func(proc *Process) {
		proc.Ppid = int32(value.Large)
	})
}

func (p *probe) mapHandleCount(instance string, value pdhutil.PdhCounterValue) {
	p.mapToStat(instance, func(stat *Stats) {
		stat.OpenFdCount = int32(value.Large)
	})
}

func (p *probe) mapThreadCount(instance string, value pdhutil.PdhCounterValue) {
	p.mapToStat(instance, func(stat *Stats) {
		stat.NumThreads = int32(value.Large)
	})
}

func (p *probe) mapPctUserTime(instance string, value pdhutil.PdhCounterValue) {
	p.mapToStat(instance, func(stat *Stats) {
		stat.CPUTime.User = value.Double
	})
}

func (p *probe) mapPctPrivilegedTime(instance string, value pdhutil.PdhCounterValue) {
	p.mapToStat(instance, func(stat *Stats) {
		stat.CPUTime.System = value.Double
	})
}

func (p *probe) mapWorkingSet(instance string, value pdhutil.PdhCounterValue) {
	p.mapToStat(instance, func(stat *Stats) {
		stat.MemInfo.RSS = uint64(value.Large)
	})
}

func (p *probe) mapPoolPagedBytes(instance string, value pdhutil.PdhCounterValue) {
	p.mapToStat(instance, func(stat *Stats) {
		stat.MemInfo.VMS = uint64(value.Large)
	})
}

func (p *probe) mapIOReadOpsPerSec(instance string, value pdhutil.PdhCounterValue) {
	p.mapToStat(instance, func(stat *Stats) {
		stat.IORateStat.ReadRate = value.Double
	})
}

func (p *probe) mapIOWriteOpsPerSec(instance string, value pdhutil.PdhCounterValue) {
	p.mapToStat(instance, func(stat *Stats) {
		stat.IORateStat.WriteRate = value.Double
	})
}

func (p *probe) mapIOReadBytesPerSec(instance string, value pdhutil.PdhCounterValue) {
	p.mapToStat(instance, func(stat *Stats) {
		stat.IORateStat.ReadBytesRate = value.Double
	})
}

func (p *probe) mapIOWriteBytesPerSec(instance string, value pdhutil.PdhCounterValue) {
	p.mapToStat(instance, func(stat *Stats) {
		stat.IORateStat.WriteBytesRate = value.Double
	})
}

func getPIDs() ([]int32, error) {
	var read uint32
	var psSize uint32 = 1024
	const dwordSize uint32 = 4

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
	procHandle, err := openProcHandle(pid)
	if err != nil {
		return err
	}
	defer windows.Close(procHandle)

	userName, usererr := getUsernameForProcess(procHandle)
	if usererr != nil {
		log.Debugf("Couldn't get process username %v %v", pid, err)
	}
	proc.Username = userName

	var imagePath, cmdline string
	if cmdParams, cmderr := winutil.GetCommandParamsForProcess(procHandle, true); cmderr != nil {
		log.Debugf("Error retrieving command params %v", cmderr)
	} else {
		imagePath = cmdParams.ImagePath
		cmdline = cmdParams.CmdLine
	}

	proc.Cmdline = parseCmdLineArgs(cmdline)
	proc.Exe = imagePath

	var CPU windows.Rusage
	if err := windows.GetProcessTimes(procHandle, &CPU.CreationTime, &CPU.ExitTime, &CPU.KernelTime, &CPU.UserTime); err != nil {
		log.Errorf("Could not get process times for %v %v", pid, err)
		return err
	}

	ctime := CPU.CreationTime.Nanoseconds() / 1000000
	proc.Stats.CreateTime = ctime
	return nil
}

// TODO: deduplicate
func openProcHandle(pid int32) (windows.Handle, error) {
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

// TODO: deduplicate
func getUsernameForProcess(h windows.Handle) (name string, err error) {
	name = ""
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

// TODO: deduplicate
func parseCmdLineArgs(cmdline string) (res []string) {
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
