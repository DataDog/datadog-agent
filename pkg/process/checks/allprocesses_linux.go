// +build linux

package checks

import (
	"time"

	"github.com/DataDog/datadog-agent/pkg/process/procutil"
	"github.com/DataDog/gopsutil/process"
)

// getAllProcesses uses a probe to fetch processes using procutil library,
// then convert them into FilledProcesses for compatibility
func getAllProcesses(probe *procutil.Probe) (map[int32]*process.FilledProcess, error) {
	procs, err := probe.ProcessesByPID(time.Now())
	if err != nil {
		return nil, err
	}
	return procutil.ConvertAllProcesses(procs), nil
}

func getAllProcStats(probe *procutil.Probe, pids []int32) (map[int32]*process.FilledProcess, error) {
	stats, err := probe.StatsForPIDs(pids, time.Now())
	if err != nil {
		return nil, err
	}

	procs := make(map[int32]*process.FilledProcess, len(stats))
	for pid, stat := range stats {
		procs[pid] = procutil.ConvertToFilledProcess(&procutil.Process{Pid: pid, Stats: stat})
	}
	return procs, nil
}
