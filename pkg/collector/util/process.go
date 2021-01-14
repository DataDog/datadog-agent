// +build !windows

package util

import "github.com/shirou/gopsutil/process"

//GetProcessCreateTime returns the create time for a specific process
func GetProcessCreateTime(pid int32) (int64, error) {
	p, err := process.NewProcess(pid)
	if err != nil {
		return 0, err
	}
	return p.CreateTime()
}
