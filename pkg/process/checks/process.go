// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package checks

import (
	"context"
	"errors"
	"time"

	model "github.com/DataDog/agent-payload/v5/process"
	"github.com/DataDog/datadog-agent/pkg/process/config"
	"github.com/DataDog/datadog-agent/pkg/process/net"
	"github.com/DataDog/datadog-agent/pkg/process/procutil"
	"github.com/DataDog/datadog-agent/pkg/process/statsd"
	"github.com/DataDog/datadog-agent/pkg/process/util"
	"github.com/DataDog/datadog-agent/pkg/util/cloudproviders"
	"github.com/DataDog/datadog-agent/pkg/util/log"
	"github.com/DataDog/gopsutil/cpu"
	"go.uber.org/atomic"
)

const emptyCtrID = ""

// Process is a singleton ProcessCheck.
var Process = &ProcessCheck{
	createTimes: &atomic.Value{},
}

var _ CheckWithRealTime = (*ProcessCheck)(nil)

var errEmptyCPUTime = errors.New("empty CPU time information returned")

// ProcessCheck collects full state, including cmdline args and related metadata,
// for live and running processes. The instance will store some state between
// checks that will be used for rates, cpu calculations, etc.
type ProcessCheck struct {
	probe procutil.Probe

	sysInfo                    *model.SystemInfo
	lastCPUTime                cpu.TimesStat
	lastProcs                  map[int32]*procutil.Process
	lastRun                    time.Time
	containerProvider          util.ContainerProvider
	lastContainerRates         map[string]*util.ContainerRateMetrics
	realtimeLastContainerRates map[string]*util.ContainerRateMetrics
	networkID                  string

	realtimeLastCPUTime cpu.TimesStat
	realtimeLastProcs   map[int32]*procutil.Stats
	realtimeLastRun     time.Time

	notInitializedLogLimit *util.LogLimit

	// lastPIDs is []int32 that holds PIDs that the check fetched last time,
	// will be reused by RT process collection to get stats
	lastPIDs []int32

	// Create times by PID used in the network check
	createTimes *atomic.Value

	// SysprobeProcessModuleEnabled tells the process check wheither to use the RemoteSystemProbeUtil to gather privileged process stats
	SysprobeProcessModuleEnabled bool

	maxBatchSize  int
	maxBatchBytes int
}

// Init initializes the singleton ProcessCheck.
func (p *ProcessCheck) Init(_ *config.AgentConfig, info *model.SystemInfo) {
	p.sysInfo = info
	p.probe = getProcessProbe()
	p.containerProvider = util.GetSharedContainerProvider()

	p.notInitializedLogLimit = util.NewLogLimit(1, time.Minute*10)

	networkID, err := cloudproviders.GetNetworkID(context.TODO())
	if err != nil {
		log.Infof("no network ID detected: %s", err)
	}
	p.networkID = networkID

	p.maxBatchSize = getMaxBatchSize()
	p.maxBatchBytes = getMaxBatchBytes()
}

// Name returns the name of the ProcessCheck.
func (p *ProcessCheck) Name() string { return config.ProcessCheckName }

// RealTimeName returns the name of the RTProcessCheck
func (p *ProcessCheck) RealTimeName() string { return config.RTProcessCheckName }

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
	result, err := p.run(cfg, groupID, false)
	if err != nil {
		return nil, err
	}

	return result.Standard, nil
}

// Cleanup frees any resource held by the ProcessCheck before the agent exits
func (p *ProcessCheck) Cleanup() {}

func (p *ProcessCheck) run(cfg *config.AgentConfig, groupID int32, collectRealTime bool) (*RunResult, error) {
	start := time.Now()
	cpuTimes, err := cpu.Times(false)
	if err != nil {
		return nil, err
	}
	if len(cpuTimes) == 0 {
		return nil, errEmptyCPUTime
	}

	// TODO: deduplicate system probe or WithPermission with RT collection
	var sysProbeUtil *net.RemoteSysProbeUtil
	// if the Process module is disabled, we allow Probe to collect
	// fields that require elevated permission to collect with best effort
	if !p.SysprobeProcessModuleEnabled {
		procutil.WithPermission(true)(p.probe)
	} else {
		procutil.WithPermission(false)(p.probe)
		if pu, err := net.GetRemoteSystemProbeUtil(); err == nil {
			sysProbeUtil = pu
		} else if p.notInitializedLogLimit.ShouldLog() {
			log.Warnf("could not initialize system-probe connection in process check: %v (will only log every 10 minutes)", err)
		}
	}

	procs, err := p.probe.ProcessesByPID(time.Now(), true)
	if err != nil {
		return nil, err
	}

	// stores lastPIDs to be used by RTProcess
	p.lastPIDs = p.lastPIDs[:0]
	for pid := range procs {
		p.lastPIDs = append(p.lastPIDs, pid)
	}

	if sysProbeUtil != nil {
		mergeProcWithSysprobeStats(p.lastPIDs, procs, sysProbeUtil)
	}

	var containers []*model.Container
	var pidToCid map[int]string
	var lastContainerRates map[string]*util.ContainerRateMetrics
	cacheValidity := cacheValidityNoRT
	if collectRealTime {
		cacheValidity = cacheValidityRT
	}

	containers, lastContainerRates, pidToCid, err = p.containerProvider.GetContainers(cacheValidity, p.lastContainerRates)
	if err == nil {
		p.lastContainerRates = lastContainerRates
	} else {
		log.Debugf("Unable to gather stats for containers, err: %v", err)
	}

	// Keep track of containers addresses
	LocalResolver.LoadAddrs(containers, pidToCid)

	// End check early if this is our first run.
	if p.lastProcs == nil {
		p.lastProcs = procs
		p.lastCPUTime = cpuTimes[0]
		p.lastRun = time.Now()
		p.storeCreateTimes()

		if collectRealTime {
			p.realtimeLastCPUTime = p.lastCPUTime
			p.realtimeLastProcs = procsToStats(p.lastProcs)
			p.realtimeLastRun = p.lastRun
		}
		return &RunResult{}, nil
	}

	connsByPID := Connections.getLastConnectionsByPID()
	procsByCtr := fmtProcesses(cfg, procs, p.lastProcs, pidToCid, cpuTimes[0], p.lastCPUTime, p.lastRun, connsByPID)
	messages, totalProcs, totalContainers := createProcCtrMessages(procsByCtr, containers, cfg, p.maxBatchSize, p.maxBatchBytes, p.sysInfo, groupID, p.networkID)

	// Store the last state for comparison on the next run.
	// Note: not storing the filtered in case there are new processes that haven't had a chance to show up twice.
	p.lastProcs = procs
	p.lastCPUTime = cpuTimes[0]
	p.lastRun = time.Now()
	p.storeCreateTimes()

	result := &RunResult{
		Standard: messages,
	}
	if collectRealTime {
		stats := procsToStats(p.lastProcs)

		if p.realtimeLastProcs != nil {
			// TODO: deduplicate chunking with RT collection
			chunkedStats := fmtProcessStats(cfg, p.maxBatchSize, stats, p.realtimeLastProcs, pidToCid, cpuTimes[0], p.realtimeLastCPUTime, p.realtimeLastRun, connsByPID)
			groupSize := len(chunkedStats)
			chunkedCtrStats := convertAndChunkContainers(containers, groupSize)

			messages := make([]model.MessageBody, 0, groupSize)
			for i := 0; i < groupSize; i++ {
				messages = append(messages, &model.CollectorRealTime{
					HostName:          cfg.HostName,
					Stats:             chunkedStats[i],
					ContainerStats:    chunkedCtrStats[i],
					GroupId:           groupID,
					GroupSize:         int32(groupSize),
					NumCpus:           int32(len(p.sysInfo.Cpus)),
					TotalMemory:       p.sysInfo.TotalMemory,
					ContainerHostType: cfg.ContainerHostType,
				})
			}
			result.RealTime = messages
		}

		p.realtimeLastCPUTime = p.lastCPUTime
		p.realtimeLastProcs = stats
		p.realtimeLastRun = p.lastRun
	}

	statsd.Client.Gauge("datadog.process.containers.host_count", float64(totalContainers), []string{}, 1) //nolint:errcheck
	statsd.Client.Gauge("datadog.process.processes.host_count", float64(totalProcs), []string{}, 1)       //nolint:errcheck
	log.Debugf("collected processes in %s", time.Now().Sub(start))

	return result, nil
}

func procsToStats(procs map[int32]*procutil.Process) map[int32]*procutil.Stats {
	stats := map[int32]*procutil.Stats{}
	for pid, proc := range procs {
		stats[pid] = proc.Stats
	}
	return stats
}

// RunWithOptions collects process data (regular metadata + stats) and/or realtime process data (stats only)
// Messages are grouped as RunResult instances with CheckName identifying the type
func (p *ProcessCheck) RunWithOptions(cfg *config.AgentConfig, nextGroupID func() int32, options RunOptions) (*RunResult, error) {
	if options.RunStandard {
		log.Tracef("Running process check")
		return p.run(cfg, nextGroupID(), options.RunRealTime)
	}

	if options.RunRealTime {
		log.Tracef("Running rtprocess check")
		return p.runRealtime(cfg, nextGroupID())
	}
	return nil, errors.New("invalid run options for check")
}

func createProcCtrMessages(
	procsByCtr map[string][]*model.Process,
	containers []*model.Container,
	cfg *config.AgentConfig,
	maxBatchSize int,
	maxBatchWeight int,
	sysInfo *model.SystemInfo,
	groupID int32,
	networkID string,
) ([]model.MessageBody, int, int) {
	collectorProcs, totalProcs, totalContainers := chunkProcessesAndContainers(procsByCtr, containers, maxBatchSize, maxBatchWeight)
	// fill in GroupSize for each CollectorProc and convert them to final messages
	// also count containers and processes
	messages := make([]model.MessageBody, 0, len(collectorProcs))
	for _, m := range collectorProcs {
		m.GroupSize = int32(len(collectorProcs))
		m.HostName = cfg.HostName
		m.NetworkId = networkID
		m.Info = sysInfo
		m.GroupId = groupID
		m.ContainerHostType = cfg.ContainerHostType

		messages = append(messages, m)
	}

	log.Tracef("Created %d process messages", len(messages))

	return messages, totalProcs, totalContainers
}

func chunkProcessesAndContainers(
	procsByCtr map[string][]*model.Process,
	containers []*model.Container,
	maxChunkSize int,
	maxChunkWeight int,
) ([]*model.CollectorProc, int, int) {
	chunker := &collectorProcChunker{}

	totalProcs := len(procsByCtr[emptyCtrID])

	chunkProcessesBySizeAndWeight(procsByCtr[emptyCtrID], nil, maxChunkSize, maxChunkWeight, chunker)

	totalContainers := len(containers)
	for _, ctr := range containers {
		procs := procsByCtr[ctr.Id]
		totalProcs += len(procs)

		chunkProcessesBySizeAndWeight(procs, ctr, maxChunkSize, maxChunkWeight, chunker)
	}
	return chunker.collectorProcs, totalProcs, totalContainers
}

// fmtProcesses goes through each process, converts them to process object and group them by containers
// non-container processes would be in a single group with key as empty string ""
func fmtProcesses(
	cfg *config.AgentConfig,
	procs, lastProcs map[int32]*procutil.Process,
	ctrByProc map[int]string,
	syst2, syst1 cpu.TimesStat,
	lastRun time.Time,
	connsByPID map[int32][]*model.Connection,
) map[string][]*model.Process {
	procsByCtr := make(map[string][]*model.Process)
	connCheckIntervalS := int(cfg.CheckIntervals[config.ConnectionsCheckName] / time.Second)

	for _, fp := range procs {
		if skipProcess(cfg, fp, lastProcs) {
			continue
		}

		// Hide blacklisted args if the Scrubber is enabled
		fp.Cmdline = cfg.Scrubber.ScrubProcessCommand(fp)

		var ioStat *model.IOStat
		if fp.Stats.IORateStat != nil {
			ioStat = &model.IOStat{
				ReadRate:       float32(fp.Stats.IORateStat.ReadRate),
				WriteRate:      float32(fp.Stats.IORateStat.WriteRate),
				ReadBytesRate:  float32(fp.Stats.IORateStat.ReadBytesRate),
				WriteBytesRate: float32(fp.Stats.IORateStat.WriteBytesRate),
			}
		} else {
			ioStat = formatIO(fp.Stats, lastProcs[fp.Pid].Stats.IOStat, lastRun)
		}

		proc := &model.Process{
			Pid:                    fp.Pid,
			NsPid:                  fp.NsPid,
			Command:                formatCommand(fp),
			User:                   formatUser(fp),
			Memory:                 formatMemory(fp.Stats),
			Cpu:                    formatCPU(fp.Stats, lastProcs[fp.Pid].Stats, syst2, syst1),
			CreateTime:             fp.Stats.CreateTime,
			OpenFdCount:            fp.Stats.OpenFdCount,
			State:                  model.ProcessState(model.ProcessState_value[fp.Stats.Status]),
			IoStat:                 ioStat,
			VoluntaryCtxSwitches:   uint64(fp.Stats.CtxSwitches.Voluntary),
			InvoluntaryCtxSwitches: uint64(fp.Stats.CtxSwitches.Involuntary),
			ContainerId:            ctrByProc[int(fp.Pid)],
			Networks:               formatNetworks(connsByPID[fp.Pid], connCheckIntervalS),
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

func formatCommand(fp *procutil.Process) *model.Command {
	return &model.Command{
		Args:   fp.Cmdline,
		Cwd:    fp.Cwd,
		Root:   "",    // TODO
		OnDisk: false, // TODO
		Ppid:   fp.Ppid,
		Exe:    fp.Exe,
	}
}

func formatIO(fp *procutil.Stats, lastIO *procutil.IOCountersStat, before time.Time) *model.IOStat {
	// This will be nil for Mac
	if fp.IOStat == nil {
		return &model.IOStat{}
	}

	diff := time.Now().Unix() - before.Unix()
	if before.IsZero() || diff <= 0 {
		return &model.IOStat{}
	}
	// Reading -1 as counter means the file could not be opened due to permissions.
	// In that case we set the rate as -1 to distinguish from a real 0 in rates.
	readRate := float32(-1)
	if fp.IOStat.ReadCount >= 0 {
		readRate = calculateRate(uint64(fp.IOStat.ReadCount), uint64(lastIO.ReadCount), before)
	}
	writeRate := float32(-1)
	if fp.IOStat.WriteCount >= 0 {
		writeRate = calculateRate(uint64(fp.IOStat.WriteCount), uint64(lastIO.WriteCount), before)
	}
	readBytesRate := float32(-1)
	if fp.IOStat.ReadBytes >= 0 {
		readBytesRate = calculateRate(uint64(fp.IOStat.ReadBytes), uint64(lastIO.ReadBytes), before)
	}
	writeBytesRate := float32(-1)
	if fp.IOStat.WriteBytes >= 0 {
		writeBytesRate = calculateRate(uint64(fp.IOStat.WriteBytes), uint64(lastIO.WriteBytes), before)
	}
	return &model.IOStat{
		ReadRate:       readRate,
		WriteRate:      writeRate,
		ReadBytesRate:  readBytesRate,
		WriteBytesRate: writeBytesRate,
	}
}

func formatMemory(fp *procutil.Stats) *model.MemoryStat {
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

func formatNetworks(conns []*model.Connection, interval int) *model.ProcessNetworks {
	connRate := float32(len(conns)) / float32(interval)
	totalTraffic := uint64(0)
	for _, conn := range conns {
		totalTraffic += conn.LastBytesSent + conn.LastBytesReceived
	}
	bytesRate := float32(totalTraffic) / float32(interval)
	return &model.ProcessNetworks{ConnectionRate: connRate, BytesRate: bytesRate}
}

func formatCPU(statsNow, statsBefore *procutil.Stats, syst2, syst1 cpu.TimesStat) *model.CPUStat {
	if statsNow.CPUPercent != nil {
		return &model.CPUStat{
			LastCpu:   "cpu",
			TotalPct:  float32(statsNow.CPUPercent.UserPct + statsNow.CPUPercent.SystemPct),
			UserPct:   float32(statsNow.CPUPercent.UserPct),
			SystemPct: float32(statsNow.CPUPercent.UserPct),
		}
	}
	return formatCPUTimes(statsNow, statsNow.CPUTime, statsBefore.CPUTime, syst2, syst1)
}

// skipProcess will skip a given process if it's blacklisted or hasn't existed
// for multiple collections.
func skipProcess(
	cfg *config.AgentConfig,
	fp *procutil.Process,
	lastProcs map[int32]*procutil.Process,
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

func (p *ProcessCheck) storeCreateTimes() {
	createTimes := make(map[int32]int64, len(p.lastProcs))
	for pid, proc := range p.lastProcs {
		createTimes[pid] = proc.Stats.CreateTime
	}
	p.createTimes.Store(createTimes)
}

func (p *ProcessCheck) createTimesforPIDs(pids []int32) map[int32]int64 {
	createTimeForPID := make(map[int32]int64)
	if result := p.createTimes.Load(); result != nil {
		createTimesAllPIDs := result.(map[int32]int64)
		for _, pid := range pids {
			if ctime, ok := createTimesAllPIDs[pid]; ok {
				createTimeForPID[pid] = ctime
			}
		}
		return createTimeForPID
	}
	return createTimeForPID
}

// mergeProcWithSysprobeStats takes a process by PID map and fill the stats from system probe into the processes in the map
func mergeProcWithSysprobeStats(pids []int32, procs map[int32]*procutil.Process, pu *net.RemoteSysProbeUtil) {
	pStats, err := pu.GetProcStats(pids)
	if err == nil {
		for pid, proc := range procs {
			if s, ok := pStats.StatsByPID[pid]; ok {
				proc.Stats.OpenFdCount = s.OpenFDCount
				proc.Stats.IOStat.ReadCount = s.ReadCount
				proc.Stats.IOStat.WriteCount = s.WriteCount
				proc.Stats.IOStat.ReadBytes = s.ReadBytes
				proc.Stats.IOStat.WriteBytes = s.WriteBytes
			}
		}
	} else {
		log.Debugf("cannot do GetProcStats from system-probe for process check: %s", err)
	}
}
