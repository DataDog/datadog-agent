package checks

import (
	"time"

	"github.com/DataDog/datadog-agent/pkg/util/containers"
	"github.com/DataDog/gopsutil/cpu"
	"github.com/DataDog/gopsutil/process"

	"github.com/DataDog/datadog-agent/pkg/process/config"
	"github.com/DataDog/datadog-agent/pkg/process/model"
	"github.com/DataDog/datadog-agent/pkg/process/util"
)

// RTProcess is a singleton RTProcessCheck.
var RTProcess = &RTProcessCheck{}

// RTProcessCheck collects numeric statistics about the live processes.
// The instance stores state between checks for calculation of rates and CPU.
type RTProcessCheck struct {
	sysInfo      *model.SystemInfo
	lastCPUTime  cpu.TimesStat
	lastProcs    map[int32]*process.FilledProcess
	lastCtrRates map[string]util.ContainerRateMetrics
	lastRun      time.Time
}

// Init initializes a new RTProcessCheck instance.
func (r *RTProcessCheck) Init(cfg *config.AgentConfig, info *model.SystemInfo) {
	r.sysInfo = info
}

// Name returns the name of the RTProcessCheck.
func (r *RTProcessCheck) Name() string { return "rtprocess" }

// Endpoint returns the endpoint where this check is submitted.
func (r *RTProcessCheck) Endpoint() string { return "/api/v1/collector" }

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
	procs, err := getAllProcesses(cfg)
	if err != nil {
		return nil, err
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

	chunkedStats := fmtProcessStats(cfg, procs, r.lastProcs,
		ctrList, cpuTimes[0], r.lastCPUTime, r.lastRun)
	groupSize := len(chunkedStats)
	chunkedCtrStats := fmtContainerStats(ctrList, r.lastCtrRates, r.lastRun, groupSize)
	messages := make([]model.MessageBody, 0, groupSize)
	for i := 0; i < groupSize; i++ {
		messages = append(messages, &model.CollectorRealTime{
			HostName:       cfg.HostName,
			Stats:          chunkedStats[i],
			ContainerStats: chunkedCtrStats[i],
			GroupId:        groupID,
			GroupSize:      int32(groupSize),
			NumCpus:        int32(len(r.sysInfo.Cpus)),
			TotalMemory:    r.sysInfo.TotalMemory,
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
	procs, lastProcs map[int32]*process.FilledProcess,
	ctrList []*containers.Container,
	syst2, syst1 cpu.TimesStat,
	lastRun time.Time,
) [][]*model.ProcessStat {
	cidByPid := make(map[int32]string, len(ctrList))
	for _, c := range ctrList {
		for _, p := range c.Pids {
			cidByPid[p] = c.ID
		}
	}

	chunked := make([][]*model.ProcessStat, 0)
	chunk := make([]*model.ProcessStat, 0, cfg.MaxPerMessage)
	for _, fp := range procs {
		if skipProcess(cfg, fp, lastProcs) {
			continue
		}

		connsCounts := Connections.connectionsCountsForPids(fp.Pid)

		chunk = append(chunk, &model.ProcessStat{
			Pid:                    fp.Pid,
			CreateTime:             fp.CreateTime,
			Memory:                 formatMemory(fp),
			Cpu:                    formatCPU(fp, fp.CpuTime, lastProcs[fp.Pid].CpuTime, syst2, syst1),
			Nice:                   fp.Nice,
			Threads:                fp.NumThreads,
			OpenFdCount:            fp.OpenFdCount,
			ProcessState:           model.ProcessState(model.ProcessState_value[fp.Status]),
			IoStat:                 formatIO(fp, lastProcs[fp.Pid].IOStat, lastRun),
			VoluntaryCtxSwitches:   uint64(fp.CtxSwitches.Voluntary),
			InvoluntaryCtxSwitches: uint64(fp.CtxSwitches.Involuntary),
			ContainerId:            cidByPid[fp.Pid],
			Conns: &model.ConnsStat{
				OpenedTcp: connsCounts.TCP,
				OpenedUdp: connsCounts.UDP,
			},
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
