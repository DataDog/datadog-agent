package checks

import (
	"errors"
	"sync"
	"time"

	agentutil "github.com/DataDog/datadog-agent/pkg/util"
	"github.com/DataDog/datadog-agent/pkg/util/containers"
	"github.com/DataDog/datadog-agent/pkg/util/log"
	"github.com/DataDog/gopsutil/cpu"
	"github.com/DataDog/gopsutil/process"

	model "github.com/DataDog/agent-payload/process"
	"github.com/DataDog/datadog-agent/pkg/process/config"
	"github.com/DataDog/datadog-agent/pkg/process/statsd"
	"github.com/DataDog/datadog-agent/pkg/process/util"
)

const emptyCtrID = ""

// Process is a singleton ProcessCheck.
var Process = &ProcessCheck{}

var errEmptyCPUTime = errors.New("empty CPU time information returned")

// ProcessCheck collects full state, including cmdline args and related metadata,
// for live and running processes. The instance will store some state between
// checks that will be used for rates, cpu calculations, etc.
type ProcessCheck struct {
	sync.Mutex

	sysInfo         *model.SystemInfo
	lastCPUTime     cpu.TimesStat
	lastProcs       map[int32]*process.FilledProcess
	lastCtrRates    map[string]util.ContainerRateMetrics
	lastCtrIDForPID map[int32]string
	lastRun         time.Time
	networkID       string
}

// Init initializes the singleton ProcessCheck.
func (p *ProcessCheck) Init(_ *config.AgentConfig, info *model.SystemInfo) {
	p.sysInfo = info

	networkID, err := agentutil.GetNetworkID()
	if err != nil {
		log.Infof("no network ID detected: %s", err)
	}
	p.networkID = networkID
}

// Name returns the name of the ProcessCheck.
func (p *ProcessCheck) Name() string { return "process" }

// RealTime indicates if this check only runs in real-time mode.
func (p *ProcessCheck) RealTime() bool { return false }

// Run runs the ProcessCheck to collect a list of running processes and relevant
// stats for each. On most POSIX systems this will use a mix of procfs and other
// OS-specific APIs to collect this information. The bulk of this collection is
// abstracted into the `gopsutil` library.
// Processes are split up into a chunks of at most 100 processes per message to
// limit the message size on intake.
// See agent.proto for the schema of the message and models used.
func (p *ProcessCheck) Run(cfg *config.AgentConfig, groupID int32) ([]model.MessageBody, error) {
	p.Lock()
	defer p.Unlock()

	start := time.Now()
	cpuTimes, err := cpu.Times(false)
	if err != nil {
		return nil, err
	}
	if len(cpuTimes) == 0 {
		return nil, errEmptyCPUTime
	}
	procs, err := getAllProcesses(cfg)
	if err != nil {
		return nil, err
	}
	ctrList, _ := util.GetContainers()

	// Keep track of containers addresses
	LocalResolver.LoadAddrs(ctrList)

	// End check early if this is our first run.
	if p.lastProcs == nil {
		p.lastProcs = procs
		p.lastCPUTime = cpuTimes[0]
		p.lastCtrRates = util.ExtractContainerRateMetric(ctrList)
		p.lastCtrIDForPID = ctrIDForPID(ctrList)
		p.lastRun = time.Now()
		return nil, nil
	}

	procsByCtr := fmtProcesses(cfg, procs, p.lastProcs, ctrList, cpuTimes[0], p.lastCPUTime, p.lastRun)
	ctrs := fmtContainers(ctrList, p.lastCtrRates, p.lastRun)

	messages, totalProcs, totalContainers := createProcCtrMessages(procsByCtr, ctrs, cfg, p.sysInfo, groupID, p.networkID)

	// Store the last state for comparison on the next run.
	// Note: not storing the filtered in case there are new processes that haven't had a chance to show up twice.
	p.lastProcs = procs
	p.lastCtrRates = util.ExtractContainerRateMetric(ctrList)
	p.lastCPUTime = cpuTimes[0]
	p.lastRun = time.Now()
	p.lastCtrIDForPID = ctrIDForPID(ctrList)

	statsd.Client.Gauge("datadog.process.containers.host_count", float64(totalContainers), []string{}, 1) //nolint:errcheck
	statsd.Client.Gauge("datadog.process.processes.host_count", float64(totalProcs), []string{}, 1)       //nolint:errcheck
	log.Debugf("collected processes in %s", time.Now().Sub(start))
	return messages, nil
}

func createProcCtrMessages(
	procsByCtr map[string][]*model.Process,
	containers []*model.Container,
	cfg *config.AgentConfig,
	sysInfo *model.SystemInfo,
	groupID int32,
	networkID string,
) ([]model.MessageBody, int, int) {
	totalProcs, totalContainers := 0, 0
	msgs := make([]*model.CollectorProc, 0)

	// we first split non-container processes in chunks
	chunks := chunkProcesses(procsByCtr[emptyCtrID], cfg.MaxPerMessage)
	for _, c := range chunks {
		msgs = append(msgs, &model.CollectorProc{
			HostName:          cfg.HostName,
			NetworkId:         networkID,
			Info:              sysInfo,
			Processes:         c,
			GroupId:           groupID,
			ContainerHostType: cfg.ContainerHostType,
		})
	}

	ctrProcs := make([]*model.Process, 0)
	ctrs := make([]*model.Container, 0, len(containers))
	for _, ctr := range containers {
		if procs, ok := procsByCtr[ctr.Id]; ok {
			ctrProcs = append(ctrProcs, procs...)
		}
		ctrs = append(ctrs, ctr)
	}

	if len(ctrs) > 0 {
		msgs = append(msgs, &model.CollectorProc{
			HostName:          cfg.HostName,
			NetworkId:         networkID,
			Info:              sysInfo,
			Processes:         ctrProcs,
			Containers:        ctrs,
			GroupId:           groupID,
			ContainerHostType: cfg.ContainerHostType,
		})
	}

	// fill in GroupSize for each CollectorProc and convert them to final messages
	// also count containers and processes
	messages := make([]model.MessageBody, 0, len(msgs))
	for _, m := range msgs {
		m.GroupSize = int32(len(msgs))
		messages = append(messages, m)
		totalProcs += len(m.Processes)
		totalContainers += len(m.Containers)
	}

	return messages, totalProcs, totalContainers
}

// chunkProcesses split non-container processes into chunks and return a list of chunks
func chunkProcesses(procs []*model.Process, size int) [][]*model.Process {
	chunkCount := len(procs) / size
	if chunkCount*size < len(procs) {
		chunkCount++
	}
	chunks := make([][]*model.Process, 0, chunkCount)

	for i := 0; i < len(procs); i += size {
		end := i + size
		if end > len(procs) {
			end = len(procs)
		}
		chunks = append(chunks, procs[i:end])
	}

	return chunks
}

func ctrIDForPID(ctrList []*containers.Container) map[int32]string {
	ctrIDForPID := make(map[int32]string, len(ctrList))
	for _, c := range ctrList {
		for _, p := range c.Pids {
			ctrIDForPID[p] = c.ID
		}
	}
	return ctrIDForPID
}

// fmtProcesses goes through each process, converts them to process object and group them by containers
// non-container processes would be in a single group with key as empty string ""
func fmtProcesses(
	cfg *config.AgentConfig,
	procs, lastProcs map[int32]*process.FilledProcess,
	ctrList []*containers.Container,
	syst2, syst1 cpu.TimesStat,
	lastRun time.Time,
) map[string][]*model.Process {
	ctrIDForPID := ctrIDForPID(ctrList)

	procsByCtr := make(map[string][]*model.Process)

	for _, fp := range procs {
		if skipProcess(cfg, fp, lastProcs) {
			continue
		}

		// Hide blacklisted args if the Scrubber is enabled
		fp.Cmdline = cfg.Scrubber.ScrubProcessCommand(fp)

		proc := &model.Process{
			Pid:                    fp.Pid,
			Command:                formatCommand(fp),
			User:                   formatUser(fp),
			Memory:                 formatMemory(fp),
			Cpu:                    formatCPU(fp, fp.CpuTime, lastProcs[fp.Pid].CpuTime, syst2, syst1),
			CreateTime:             fp.CreateTime,
			OpenFdCount:            fp.OpenFdCount,
			State:                  model.ProcessState(model.ProcessState_value[fp.Status]),
			IoStat:                 formatIO(fp, lastProcs[fp.Pid].IOStat, lastRun),
			VoluntaryCtxSwitches:   uint64(fp.CtxSwitches.Voluntary),
			InvoluntaryCtxSwitches: uint64(fp.CtxSwitches.Involuntary),
			ContainerId:            ctrIDForPID[fp.Pid],
		}
		_, ok := procsByCtr[proc.ContainerId]
		if !ok {
			procsByCtr[proc.ContainerId] = make([]*model.Process, 0)
		}
		procsByCtr[proc.ContainerId] = append(procsByCtr[proc.ContainerId], proc)
	}

	cfg.Scrubber.IncrementCacheAge()

	return procsByCtr
}

func formatCommand(fp *process.FilledProcess) *model.Command {
	return &model.Command{
		Args:   fp.Cmdline,
		Cwd:    fp.Cwd,
		Root:   "",    // TODO
		OnDisk: false, // TODO
		Ppid:   fp.Ppid,
		Exe:    fp.Exe,
	}
}

func formatIO(fp *process.FilledProcess, lastIO *process.IOCountersStat, before time.Time) *model.IOStat {
	// This will be nill for Mac
	if fp.IOStat == nil {
		return &model.IOStat{}
	}

	diff := time.Now().Unix() - before.Unix()
	if before.IsZero() || diff <= 0 {
		return &model.IOStat{}
	}
	// Reading 0 as a counter means the file could not be opened due to permissions. We distinguish this from a real 0 in rates.
	var readRate float32
	readRate = -1
	if fp.IOStat.ReadCount != 0 {
		readRate = calculateRate(fp.IOStat.ReadCount, lastIO.ReadCount, before)
	}
	var writeRate float32
	writeRate = -1
	if fp.IOStat.WriteCount != 0 {
		writeRate = calculateRate(fp.IOStat.WriteCount, lastIO.WriteCount, before)
	}
	var readBytesRate float32
	readBytesRate = -1
	if fp.IOStat.ReadBytes != 0 {
		readBytesRate = calculateRate(fp.IOStat.ReadBytes, lastIO.ReadBytes, before)
	}
	var writeBytesRate float32
	writeBytesRate = -1
	if fp.IOStat.WriteBytes != 0 {
		writeBytesRate = calculateRate(fp.IOStat.WriteBytes, lastIO.WriteBytes, before)
	}
	return &model.IOStat{
		ReadRate:       readRate,
		WriteRate:      writeRate,
		ReadBytesRate:  readBytesRate,
		WriteBytesRate: writeBytesRate,
	}
}

func formatMemory(fp *process.FilledProcess) *model.MemoryStat {
	ms := &model.MemoryStat{
		Rss:  fp.MemInfo.RSS,
		Vms:  fp.MemInfo.VMS,
		Swap: fp.MemInfo.Swap,
	}

	if fp.MemInfoEx != nil {
		ms.Shared = fp.MemInfoEx.Shared
		ms.Text = fp.MemInfoEx.Text
		ms.Lib = fp.MemInfoEx.Lib
		ms.Data = fp.MemInfoEx.Data
		ms.Dirty = fp.MemInfoEx.Dirty
	}
	return ms
}

// skipProcess will skip a given process if it's blacklisted or hasn't existed
// for multiple collections.
func skipProcess(
	cfg *config.AgentConfig,
	fp *process.FilledProcess,
	lastProcs map[int32]*process.FilledProcess,
) bool {
	if len(fp.Cmdline) == 0 {
		return true
	}
	if config.IsBlacklisted(fp.Cmdline, cfg.Blacklist) {
		return true
	}
	if _, ok := lastProcs[fp.Pid]; !ok {
		// Skipping any processes that didn't exist in the previous run.
		// This means short-lived processes (<2s) will never be captured.
		return true
	}
	return false
}

// filterCtrIDsByPIDs uses lastCtrIDForPID and filter down only the pid -> cid that we need
func (p *ProcessCheck) filterCtrIDsByPIDs(pids []int32) map[int32]string {
	p.Lock()
	defer p.Unlock()

	ctrByPid := make(map[int32]string)
	for _, pid := range pids {
		if cid, ok := p.lastCtrIDForPID[pid]; ok {
			ctrByPid[pid] = cid
		}
	}
	return ctrByPid
}

func (p *ProcessCheck) createTimesforPIDs(pids []int32) map[int32]int64 {
	p.Lock()
	defer p.Unlock()

	createTimeForPID := make(map[int32]int64)
	for _, pid := range pids {
		if p, ok := p.lastProcs[pid]; ok {
			createTimeForPID[pid] = p.CreateTime
		}
	}
	return createTimeForPID
}
