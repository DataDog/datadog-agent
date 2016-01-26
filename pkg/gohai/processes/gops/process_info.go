// Extract the information on running processes from gopsutil
package gops

import (
	"log"

	"github.com/shirou/gopsutil/mem"
	"github.com/shirou/gopsutil/process"
)

type ProcessInfo struct {
	PID      int32
	Name     string
	RSS      uint64
	PctMem   float64
	VMS      uint64
	Username string
}

// Return a slice of all the processes that are running
func GetProcesses() ([]*ProcessInfo, error) {
	processInfos := make([]*ProcessInfo, 0, 10)

	virtMemStat, err := mem.VirtualMemory()
	if err != nil {
		log.Printf("Error fetching system memory stats: %s", err)
		return nil, err
	}
	totalMem := float64(virtMemStat.Total)

	pids, err := process.Pids()
	if err != nil {
		log.Printf("Error fetching PIDs: %s", err)
		return nil, err
	}

	for _, pid := range pids {
		p, err := process.NewProcess(pid)
		if err != nil {
			log.Printf("Error fetching info for pid %d: %s", pid, err)
			continue
		}

		processInfo, err := newProcessInfo(p, pid, totalMem)
		if err != nil {
			log.Printf("Error fetching info for pid %d: %s", pid, err)
			continue
		}

		processInfos = append(processInfos, processInfo)
	}

	return processInfos, nil
}

// Make a new ProcessInfo from a Process from gopsutil
func newProcessInfo(p *process.Process, pid int32, totalMem float64) (*ProcessInfo, error) {
	memInfo, err := p.MemoryInfo()
	if err != nil {
		return nil, err
	}

	name, err := pickName(p)
	if err != nil {
		return nil, err
	}

	pctMem := 100. * float64(memInfo.RSS) / totalMem

	username, err := p.Username()
	if err != nil {
		return nil, err
	}

	return &ProcessInfo{pid, name, memInfo.RSS, pctMem, memInfo.VMS, username}, nil
}
