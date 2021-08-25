package checks

import (
	"time"

	model "github.com/DataDog/agent-payload/process"
	"github.com/DataDog/datadog-agent/pkg/process/config"
	"github.com/DataDog/datadog-agent/pkg/process/net"
	"github.com/DataDog/datadog-agent/pkg/process/procutil"
	"github.com/DataDog/datadog-agent/pkg/process/util"
	"github.com/DataDog/datadog-agent/pkg/util/containers"
	"github.com/DataDog/datadog-agent/pkg/util/log"
	"github.com/DataDog/gopsutil/cpu"
)

// RTProcess is a singleton RTProcessCheck.
var RTProcess = &RTProcessCheck{probe: procutil.NewProcessProbe()}

// RTProcessCheck collects numeric statistics about the live processes.
// The instance stores state between checks for calculation of rates and CPU.
type RTProcessCheck struct {
	sysInfo      *model.SystemInfo
	lastCPUTime  cpu.TimesStat
	lastProcs    map[int32]*procutil.Stats
	lastCtrRates map[string]util.ContainerRateMetrics
	lastRun      time.Time

	notInitializedLogLimit *util.LogLimit

	probe *procutil.Probe
}

// Init initializes a new RTProcessCheck instance.
func (r *RTProcessCheck) Init(_ *config.AgentConfig, info *model.SystemInfo) {
	r.sysInfo = info
	r.notInitializedLogLimit = util.NewLogLimit(1, time.Minute*10)
}

// Name returns the name of the RTProcessCheck.
func (r *RTProcessCheck) Name() string { return config.RTProcessCheckName }

// RealTime indicates if this check only runs in real-time mode.
func (r *RTProcessCheck) RealTime() bool { return true }

// Run runs the RTProcessCheck to collect statistics about the running processes.
// On most POSIX systems these statistics are collected from procfs. The bulk
// of this collection is abstracted into the `gopsutil` library.
// Processes are split up into a chunks of at most 100 processes per message to
// limit the message size on intake.
// See agent.proto for the schema of the message and models used.
func (r *RTProcessCheck) Run(cfg *config.AgentConfig, groupID int32) ([]model.MessageBody, error) {
	cpuTimes, err := cpu.Times(false)
	if err != nil {
		return nil, err
	}
	if len(cpuTimes) == 0 {
		return nil, errEmptyCPUTime
	}

	// if processCheck haven't fetched any PIDs, return early
	lastPIDs := Process.GetLastPIDs()
	if len(lastPIDs) == 0 {
		return nil, nil
	}

	var sysProbeUtil *net.RemoteSysProbeUtil
	// if the Process module is disabled, we allow Probe to collect
	// fields that require elevated permission to collect with best effort
	if !cfg.CheckIsEnabled(config.ProcessModuleCheckName) {
		procutil.WithPermission(true)(r.probe)
	} else {
		procutil.WithPermission(false)(r.probe)
		if pu, err := net.GetRemoteSystemProbeUtil(); err == nil {
			sysProbeUtil = pu
		} else if r.notInitializedLogLimit.ShouldLog() {
			log.Warnf("could not initialize system-probe connection in rtprocess check: %v (will only log every 10 minutes)", err)
		}
	}

	procs, err := getAllProcStats(r.probe, lastPIDs)
	if err != nil {
		return nil, err
	}

	if sysProbeUtil != nil {
		mergeStatWithSysprobeStats(lastPIDs, procs, sysProbeUtil)
	}

	ctrList, _ := util.GetContainers()

	// End check early if this is our first run.
	if r.lastProcs == nil {
		r.lastCtrRates = util.ExtractContainerRateMetric(ctrList)
		r.lastProcs = procs
		r.lastCPUTime = cpuTimes[0]
		r.lastRun = time.Now()
		return nil, nil
	}

	connsByPID := Connections.getLastConnectionsByPID()
	chunkedStats := fmtProcessStats(cfg, procs, r.lastProcs, ctrList, cpuTimes[0], r.lastCPUTime, r.lastRun, connsByPID)
	groupSize := len(chunkedStats)
	chunkedCtrStats := fmtContainerStats(ctrList, r.lastCtrRates, r.lastRun, groupSize)
	messages := make([]model.MessageBody, 0, groupSize)
	for i := 0; i < groupSize; i++ {
		messages = append(messages, &model.CollectorRealTime{
			HostName:          cfg.HostName,
			Stats:             chunkedStats[i],
			ContainerStats:    chunkedCtrStats[i],
			GroupId:           groupID,
			GroupSize:         int32(groupSize),
			NumCpus:           int32(len(r.sysInfo.Cpus)),
			TotalMemory:       r.sysInfo.TotalMemory,
			ContainerHostType: cfg.ContainerHostType,
		})
	}

	// Store the last state for comparison on the next run.
	// Note: not storing the filtered in case there are new processes that haven't had a chance to show up twice.
	r.lastRun = time.Now()
	r.lastProcs = procs
	r.lastCtrRates = util.ExtractContainerRateMetric(ctrList)
	r.lastCPUTime = cpuTimes[0]

	return messages, nil
}

// fmtProcessStats formats and chunks a slice of ProcessStat into chunks.
func fmtProcessStats(
	cfg *config.AgentConfig,
	procs, lastProcs map[int32]*procutil.Stats,
	ctrList []*containers.Container,
	syst2, syst1 cpu.TimesStat,
	lastRun time.Time,
	connsByPID map[int32][]*model.Connection,
) [][]*model.ProcessStat {
	cidByPid := make(map[int32]string, len(ctrList))
	for _, c := range ctrList {
		for _, p := range c.Pids {
			cidByPid[p] = c.ID
		}
	}

	connCheckIntervalS := int(cfg.CheckIntervals[config.ConnectionsCheckName] / time.Second)

	chunked := make([][]*model.ProcessStat, 0)
	chunk := make([]*model.ProcessStat, 0, cfg.MaxPerMessage)

	for pid, fp := range procs {
		// Skipping any processes that didn't exist in the previous run.
		// This means short-lived processes (<2s) will never be captured.
		if _, ok := lastProcs[pid]; !ok {
			continue
		}

		chunk = append(chunk, &model.ProcessStat{
			Pid:                    pid,
			CreateTime:             fp.CreateTime,
			Memory:                 formatMemory(fp),
			Cpu:                    formatCPU(fp, fp.CPUTime, lastProcs[pid].CPUTime, syst2, syst1),
			Nice:                   fp.Nice,
			Threads:                fp.NumThreads,
			OpenFdCount:            fp.OpenFdCount,
			ProcessState:           model.ProcessState(model.ProcessState_value[fp.Status]),
			IoStat:                 formatIO(fp, lastProcs[pid].IOStat, lastRun),
			VoluntaryCtxSwitches:   uint64(fp.CtxSwitches.Voluntary),
			InvoluntaryCtxSwitches: uint64(fp.CtxSwitches.Involuntary),
			ContainerId:            cidByPid[pid],
			Networks:               formatNetworks(connsByPID[pid], connCheckIntervalS),
		})
		if len(chunk) == cfg.MaxPerMessage {
			chunked = append(chunked, chunk)
			chunk = make([]*model.ProcessStat, 0, cfg.MaxPerMessage)
		}
	}
	if len(chunk) > 0 {
		chunked = append(chunked, chunk)
	}
	return chunked
}

func calculateRate(cur, prev uint64, before time.Time) float32 {
	now := time.Now()
	diff := now.Unix() - before.Unix()
	if before.IsZero() || diff <= 0 || prev == 0 || prev > cur {
		return 0
	}
	return float32(cur-prev) / float32(diff)
}

// mergeStatWithSysprobeStats takes a process by PID map and fill the stats from system probe into the processes in the map
func mergeStatWithSysprobeStats(pids []int32, stats map[int32]*procutil.Stats, pu *net.RemoteSysProbeUtil) {
	pStats, err := pu.GetProcStats(pids)
	if err == nil {
		for pid, stats := range stats {
			if s, ok := pStats.StatsByPID[pid]; ok {
				stats.OpenFdCount = s.OpenFDCount
				stats.IOStat.ReadCount = s.ReadCount
				stats.IOStat.WriteCount = s.WriteCount
				stats.IOStat.ReadBytes = s.ReadBytes
				stats.IOStat.WriteBytes = s.WriteBytes
			}
		}
	} else {
		log.Debugf("cannot do GetProcStats from system-probe for rtprocess check: %s", err)
	}
}
