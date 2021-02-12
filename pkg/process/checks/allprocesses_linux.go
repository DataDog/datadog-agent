// +build linux

package checks

import (
	"time"

	"github.com/DataDog/datadog-agent/pkg/process/procutil"
)

// getAllProcesses uses a probe to fetch processes using procutil library,
// then convert them into FilledProcesses for compatibility
func getAllProcesses(probe *procutil.Probe) (map[int32]*procutil.Process, error) {
	return probe.ProcessesByPID(time.Now())
}

func getAllProcStats(probe *procutil.Probe, pids []int32) (map[int32]*procutil.Stats, error) {
	return probe.StatsForPIDs(pids, time.Now())
}
