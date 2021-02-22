// +build !windows,!linux

package checks

import (
	"github.com/DataDog/datadog-agent/pkg/process/procutil"
	"github.com/DataDog/gopsutil/process"
)

func getAllProcesses(probe *procutil.Probe) (map[int32]*procutil.Process, error) {
	procs, err := process.AllProcesses()
	if err != nil {
		return nil, err
	}
	return procutil.ConvertAllFilledProcesses(procs), nil
}

func getAllProcStats(probe *procutil.Probe, pids []int32) (map[int32]*procutil.Stats, error) {
	procs, err := process.AllProcesses()
	if err != nil {
		return nil, err
	}
	return procutil.ConvertAllFilledProcessesToStats(procs), nil
}
