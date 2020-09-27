package util

import (
	"golang.org/x/sys/windows"
)

// this code was copied over from shirou/gopsutil/process because we can't import this package on Windows,
// due to its "wmi" dependency.

type systemTimes struct {
	CreateTime windows.Filetime
	ExitTime   windows.Filetime
	KernelTime windows.Filetime
	UserTime   windows.Filetime
}

//GetProcessCreateTime returns the create time for a specific process
func GetProcessCreateTime(pid int32) (int64, error) {
	var times systemTimes

	// PROCESS_QUERY_LIMITED_INFORMATION is 0x1000
	h, err := windows.OpenProcess(0x1000, false, uint32(pid))
	if err != nil {
		return 0, err
	}
	defer windows.CloseHandle(h)

	err = windows.GetProcessTimes(
		windows.Handle(h),
		&times.CreateTime,
		&times.ExitTime,
		&times.KernelTime,
		&times.UserTime,
	)

	return times.CreateTime.Nanoseconds(), err
}
