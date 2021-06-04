package checks

import (
	"errors"
	"sync"
	"sync/atomic"
	"time"

	model "github.com/DataDog/agent-payload/process"
	"github.com/DataDog/datadog-agent/pkg/process/config"
	"github.com/DataDog/datadog-agent/pkg/process/net"
	"github.com/DataDog/datadog-agent/pkg/process/procutil"
	"github.com/DataDog/datadog-agent/pkg/process/statsd"
	"github.com/DataDog/datadog-agent/pkg/process/util"
	agentutil "github.com/DataDog/datadog-agent/pkg/util"
	"github.com/DataDog/datadog-agent/pkg/util/containers"
	"github.com/DataDog/datadog-agent/pkg/util/log"
	"github.com/DataDog/gopsutil/cpu"
)

const emptyCtrID = ""

// Process is a singleton ProcessCheck.
var Process = &ProcessCheck{probe: procutil.NewProcessProbe()}

var errEmptyCPUTime = errors.New("empty CPU time information returned")

// ctrProcMsgFactory builds a CollectorProc
type ctrProcMsgFactory func([]*model.Process, ...*model.Container) *model.CollectorProc

// ProcessCheck collects full state, including cmdline args and related metadata,
// for live and running processes. The instance will store some state between
// checks that will be used for rates, cpu calculations, etc.
type ProcessCheck struct {
	sync.RWMutex

	probe *procutil.Probe

	sysInfo         *model.SystemInfo
	lastCPUTime     cpu.TimesStat
	lastProcs       map[int32]*procutil.Process
	lastCtrRates    map[string]util.ContainerRateMetrics
	lastCtrIDForPID map[int32]string
	lastRun         time.Time
	networkID       string

	notInitializedLogLimit *util.LogLimit

	// lastPIDs is []int32 that holds PIDs that the check fetched last time,
	// will be reused by RTProcessCheck to get stats
	lastPIDs atomic.Value
}

// Init initializes the singleton ProcessCheck.
func (p *ProcessCheck) Init(_ *config.AgentConfig, info *model.SystemInfo) {
	p.sysInfo = info
	p.notInitializedLogLimit = util.NewLogLimit(1, time.Minute*10)

	networkID, err := agentutil.GetNetworkID()
	if err != nil {
		log.Infof("no network ID detected: %s", err)
	}
	p.networkID = networkID
}

// Name returns the name of the ProcessCheck.
func (p *ProcessCheck) Name() string { return config.ProcessCheckName }

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

	var sysProbeUtil *net.RemoteSysProbeUtil
	// if the Process module is disabled, we allow Probe to collect
	// fields that require elevated permission to collect with best effort
	if !cfg.CheckIsEnabled(config.ProcessModuleCheckName) {
		procutil.WithPermission(true)(p.probe)
	} else {
		procutil.WithPermission(false)(p.probe)
		if pu, err := net.GetRemoteSystemProbeUtil(); err == nil {
			sysProbeUtil = pu
		} else if p.notInitializedLogLimit.ShouldLog() {
			log.Warnf("could not initialize system-probe connection in process check: %v (will only log every 10 minutes)", err)
		}
	}

	procs, err := getAllProcesses(p.probe)
	if err != nil {
		return nil, err
	}

	// stores lastPIDs to be used by RTProcess
	lastPIDs := make([]int32, 0, len(procs))
	for pid := range procs {
		lastPIDs = append(lastPIDs, pid)
	}
	p.lastPIDs.Store(lastPIDs)

	if sysProbeUtil != nil {
		mergeProcWithSysprobeStats(lastPIDs, procs, sysProbeUtil)
	}

	ctrList, _ := util.GetContainers()

	// Keep track of containers addresses
	LocalResolver.LoadAddrs(ctrList)

	ctrByProc := ctrIDForPID(ctrList)
	// End check early if this is our first run.
	if p.lastProcs == nil {
		p.lastProcs = procs
		p.lastCPUTime = cpuTimes[0]
		p.lastCtrRates = util.ExtractContainerRateMetric(ctrList)
		p.lastCtrIDForPID = ctrByProc
		p.lastRun = time.Now()
		return nil, nil
	}

	connsByPID := Connections.getLastConnectionsByPID()
	procsByCtr := fmtProcesses(cfg, procs, p.lastProcs, ctrByProc, cpuTimes[0], p.lastCPUTime, p.lastRun, connsByPID)

	ctrs := fmtContainers(ctrList, p.lastCtrRates, p.lastRun)

	messages, totalProcs, totalContainers := createProcCtrMessages(procsByCtr, ctrs, cfg, p.sysInfo, groupID, p.networkID)

	// Store the last state for comparison on the next run.
	// Note: not storing the filtered in case there are new processes that haven't had a chance to show up twice.
	p.lastProcs = procs
	p.lastCtrRates = util.ExtractContainerRateMetric(ctrList)
	p.lastCPUTime = cpuTimes[0]
	p.lastRun = time.Now()
	p.lastCtrIDForPID = ctrByProc

	statsd.Client.Gauge("datadog.process.containers.host_count", float64(totalContainers), []string{}, 1) //nolint:errcheck
	statsd.Client.Gauge("datadog.process.processes.host_count", float64(totalProcs), []string{}, 1)       //nolint:errcheck
	log.Debugf("collected processes in %s", time.Now().Sub(start))
	return messages, nil
}

// GetLastPIDs returns the lastPIDs as []int32 slice
func (p *ProcessCheck) GetLastPIDs() []int32 {
	if result := p.lastPIDs.Load(); result != nil {
		return result.([]int32)
	}
	return nil
}

func createProcCtrMessages(
	procsByCtr map[string][]*model.Process,
	containers []*model.Container,
	cfg *config.AgentConfig,
	sysInfo *model.SystemInfo,
	groupID int32,
	networkID string,
) ([]model.MessageBody, int, int) {
	var totalProcs, totalContainers int
	var msgs []*model.CollectorProc

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

	procCtrMessages := packProcCtrMessages(cfg.MaxCtrProcessesPerMessage, procsByCtr, containers,
		func(p []*model.Process, c ...*model.Container) *model.CollectorProc {
			return &model.CollectorProc{
				HostName:          cfg.HostName,
				NetworkId:         networkID,
				Info:              sysInfo,
				Processes:         p,
				Containers:        c,
				GroupId:           groupID,
				ContainerHostType: cfg.ContainerHostType,
			}
		},
	)

	msgs = append(msgs, procCtrMessages...)

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

// packProcCtrMessages packs container processes into messages using the next-fit bin packing algorithm. The
// container and its processes are placed into a CollectorProc up to the provided capacity. Some containers may have
// more processes than the supplied capacity, for these they simply get packed into its own message.
func packProcCtrMessages(
	capacity int,
	procsByCtr map[string][]*model.Process,
	containers []*model.Container,
	msgFn ctrProcMsgFactory,
) []*model.CollectorProc {
	var msgs []*model.CollectorProc
	var ctrs []*model.Container
	var ctrProcs []*model.Process

	space := capacity

	for _, ctr := range containers {
		procs := procsByCtr[ctr.Id]

		if len(procs) > capacity {
			// this container has more process then the msg capacity, so we send it separately
			msgs = append(msgs, msgFn(procs, ctr))
			continue
		}

		if len(procs) > space {
			// there is not enough space to fit the next set of container processes, so complete the payload with
			// the previous container processes and reset
			msgs = append(msgs, msgFn(ctrProcs, ctrs...))
			ctrs = nil
			ctrProcs = nil
			space = capacity
		}

		ctrs = append(ctrs, ctr)
		ctrProcs = append(ctrProcs, procs...)
		space -= len(procs)
	}

	if len(ctrs) > 0 {
		// create messages with any remaining containers and processes
		msgs = append(msgs, msgFn(ctrProcs, ctrs...))
	}

	log.Tracef("Created %d container process messages", len(msgs))

	return msgs
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
	procs, lastProcs map[int32]*procutil.Process,
	ctrByProc map[int32]string,
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

		proc := &model.Process{
			Pid:                    fp.Pid,
			NsPid:                  fp.NsPid,
			Command:                formatCommand(fp),
			User:                   formatUser(fp),
			Memory:                 formatMemory(fp.Stats),
			Cpu:                    formatCPU(fp.Stats, fp.Stats.CPUTime, lastProcs[fp.Pid].Stats.CPUTime, syst2, syst1),
			CreateTime:             fp.Stats.CreateTime,
			OpenFdCount:            fp.Stats.OpenFdCount,
			State:                  model.ProcessState(model.ProcessState_value[fp.Stats.Status]),
			IoStat:                 formatIO(fp.Stats, lastProcs[fp.Pid].Stats.IOStat, lastRun),
			VoluntaryCtxSwitches:   uint64(fp.Stats.CtxSwitches.Voluntary),
			InvoluntaryCtxSwitches: uint64(fp.Stats.CtxSwitches.Involuntary),
			ContainerId:            ctrByProc[fp.Pid],
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

func (p *ProcessCheck) createTimesforPIDs(pids []int32) map[int32]int64 {
	p.RLock()
	defer p.RUnlock()

	createTimeForPID := make(map[int32]int64)
	for _, pid := range pids {
		if p, ok := p.lastProcs[pid]; ok {
			createTimeForPID[pid] = p.Stats.CreateTime
		}
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
