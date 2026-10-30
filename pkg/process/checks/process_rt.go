// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package checks

import (
	"net/http"
	"time"

	model "github.com/DataDog/agent-payload/v5/process"
	"github.com/shirou/gopsutil/v4/cpu"

	"github.com/DataDog/datadog-agent/pkg/process/net"
	"github.com/DataDog/datadog-agent/pkg/process/procutil"
	proccontainers "github.com/DataDog/datadog-agent/pkg/process/util/containers"
	"github.com/DataDog/datadog-agent/pkg/util/log"
	ddslices "github.com/DataDog/datadog-agent/pkg/util/slices"
)

// runRealtime runs the realtime ProcessCheck to collect statistics about the running processes.
// Underlying procutil.Probe is responsible for the actual implementation
func (p *ProcessCheck) runRealtime(groupID int32) (RunResult, error) {
	start := p.clock.Now()
	cpuTimes, err := cpu.Times(false)
	if err != nil {
		return nil, err
	}
	if len(cpuTimes) == 0 {
		return nil, errEmptyCPUTime
	}

	// if processCheck haven't fetched any PIDs, return early
	if len(p.lastPIDs) == 0 {
		return CombinedRunResult{}, nil
	}

	procs, err := p.probe.StatsForPIDs(p.lastPIDs, start)
	if err != nil {
		return nil, err
	}
	procs = filterRealtimeStats(procs, p.ignoreZombieProcesses)

	if p.sysprobeClient != nil && p.sysProbeConfig.ProcessModuleEnabled {
		mergeStatWithSysprobeStats(p.lastPIDs, procs, p.sysprobeClient)
	}

	var containers []*model.Container
	var pidToCid map[int]string
	var lastContainerRates map[string]*proccontainers.ContainerRateMetrics
	containers, lastContainerRates, pidToCid, err = p.containerProvider.GetContainers(cacheValidityRT, p.realtimeLastContainerRates)
	if err == nil {
		p.realtimeLastContainerRates = lastContainerRates
	} else {
		log.Debugf("Unable to gather stats for containers, err: %v", err)
	}

	// End check early if this is our first run.
	if p.realtimeLastProcs == nil {
		p.realtimeLastProcs = procs
		p.realtimeLastCPUTime = cpuTimes[0]
		p.realtimeLastRun = start
		log.Debug("first run of rtprocess check - no stats to report")
		return CombinedRunResult{}, nil
	}

	result := CombinedRunResult{}
	procStats := convertProcessStats(procs, p.realtimeLastProcs, pidToCid, cpuTimes[0], p.realtimeLastCPUTime, p.realtimeLastRun, start)
	if len(procStats) > 0 {
		containerStats := ddslices.Map(containers, convertToContainerStat)
		result.Realtime = chunkMessages2(procStats, containerStats, p.maxBatchSize, func(procChunk []*model.ProcessStat, ctrChunk []*model.ContainerStat, groupSize int32) model.MessageBody {
			return &model.CollectorRealTime{
				HostName:          p.hostInfo.HostName,
				Stats:             procChunk,
				ContainerStats:    ctrChunk,
				GroupId:           groupID,
				GroupSize:         groupSize,
				NumCpus:           int32(len(p.hostInfo.SystemInfo.Cpus)),
				TotalMemory:       p.hostInfo.SystemInfo.TotalMemory,
				ContainerHostType: p.hostInfo.ContainerHostType,
			}
		})
	}

	// Store the filtered last state for comparison on the next run.
	p.realtimeLastRun = start
	p.realtimeLastProcs = procs
	p.realtimeLastCPUTime = cpuTimes[0]

	return result, nil
}

// convertProcessStats converts procutil.Stat into model.ProcessStat.
func convertProcessStats(
	procs, lastProcs map[int32]*procutil.Stats,
	pidToCid map[int]string,
	syst2, syst1 cpu.TimesStat,
	lastRun time.Time,
	now time.Time,
) []*model.ProcessStat {
	var procStats []*model.ProcessStat
	for pid, fp := range procs {
		if fp == nil {
			continue
		}

		// Skipping any processes that didn't exist in the previous run.
		// This means short-lived processes (<2s) will never be captured.
		previous, ok := lastProcs[pid]
		if !ok || previous == nil {
			continue
		}

		var ioStat *model.IOStat
		if fp.IORateStat != nil {
			ioStat = &model.IOStat{
				ReadRate:       float32(fp.IORateStat.ReadRate),
				WriteRate:      float32(fp.IORateStat.WriteRate),
				ReadBytesRate:  float32(fp.IORateStat.ReadBytesRate),
				WriteBytesRate: float32(fp.IORateStat.WriteBytesRate),
			}
		} else {
			ioStat = formatIO(fp, previous.IOStat, now, lastRun)
		}

		var voluntaryCtxSwitches, involuntaryCtxSwitches uint64
		if fp.CtxSwitches != nil {
			voluntaryCtxSwitches = uint64(fp.CtxSwitches.Voluntary)
			involuntaryCtxSwitches = uint64(fp.CtxSwitches.Involuntary)
		}
		stat := &model.ProcessStat{
			Pid:                    pid,
			CreateTime:             fp.CreateTime,
			Memory:                 formatMemory(fp),
			Cpu:                    formatCPU(fp, previous, syst2, syst1),
			Nice:                   fp.Nice,
			Threads:                fp.NumThreads,
			OpenFdCount:            fp.OpenFdCount,
			ProcessState:           model.ProcessState(model.ProcessState_value[fp.Status]),
			IoStat:                 ioStat,
			VoluntaryCtxSwitches:   voluntaryCtxSwitches,
			InvoluntaryCtxSwitches: involuntaryCtxSwitches,
			ContainerId:            pidToCid[int(pid)],
		}

		procStats = append(procStats, stat)
	}
	return procStats
}

// calculateRate returns the average counter growth per second, or zero when there is no valid baseline.
func calculateRate(cur, prev uint64, now, before time.Time) float32 {
	diff := now.Unix() - before.Unix()
	if before.IsZero() || diff <= 0 || prev == 0 || prev > cur {
		return 0
	}
	return float32(cur-prev) / float32(diff)
}

// filterRealtimeStats removes nil entries and, when configured, zombie entries
// from realtime process stats.
func filterRealtimeStats(stats map[int32]*procutil.Stats, zombiesIgnored bool) map[int32]*procutil.Stats {
	filtered := make(map[int32]*procutil.Stats, len(stats))
	for pid, stat := range stats {
		if stat == nil || (zombiesIgnored && stat.IsZombie()) {
			continue
		}
		filtered[pid] = stat
	}
	return filtered
}

func pidsForRealtimeSystemProbeStats(pids []int32, stats map[int32]*procutil.Stats) []int32 {
	filtered := make([]int32, 0, len(pids))
	for _, pid := range pids {
		stat, ok := stats[pid]
		if ok && stat != nil && !stat.IsZombie() {
			filtered = append(filtered, pid)
		}
	}
	return filtered
}

// mergeStatWithSysprobeStats takes a process by PID map and fill the stats from system probe into the processes in the map
func mergeStatWithSysprobeStats(pids []int32, stats map[int32]*procutil.Stats, client *http.Client) {
	pids = pidsForRealtimeSystemProbeStats(pids, stats)
	if len(pids) == 0 {
		return
	}

	pStats, err := net.GetProcStats(client, pids)
	if err == nil {
		for pid, stats := range stats {
			if stats.IsZombie() {
				continue
			}
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
