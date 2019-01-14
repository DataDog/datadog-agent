// +build !windows

package watchdog

import "github.com/shirou/gopsutil/process"

func cpuTimeUser(pid int32) (float64, error) {
	p, err := process.NewProcess(pid)
	if err != nil {
		return 0, err
	}
	times, err := p.Times()
	if err != nil {
		return 0, err
	}
	return times.User, nil
}
