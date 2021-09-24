// +build linux

package checks

import (
	"time"

	"github.com/DataDog/datadog-agent/pkg/process/procutil"
)

func getAllProcesses(probe *procutil.Probe, collectStats bool) (map[int32]*procutil.Process, error) {
	return probe.ProcessesByPID(time.Now(), collectStats)
}

func getAllProcStats(probe *procutil.Probe, pids []int32) (map[int32]*procutil.Stats, error) {
	return probe.StatsForPIDs(pids, time.Now())
}
