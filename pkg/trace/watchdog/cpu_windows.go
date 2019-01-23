package watchdog

import (
	"syscall"

	"golang.org/x/sys/windows"
)

// this code was copied over from shirou/gopsutil/process because we can't import this package on Windows,
// due to its "wmi" dependency.

func cpuTimeUser(pid int32) (float64, error) {
	t, err := getProcessCPUTimes(pid)
	if err != nil {
		return 0, err
	}
	return float64(t.UserTime.HighDateTime)*429.4967296 + float64(t.UserTime.LowDateTime)*1e-7, nil
}

type systemTimes struct {
	CreateTime syscall.Filetime
	ExitTime   syscall.Filetime
	KernelTime syscall.Filetime
	UserTime   syscall.Filetime
}

func getProcessCPUTimes(pid int32) (systemTimes, error) {
	var times systemTimes

	// PROCESS_QUERY_LIMITED_INFORMATION is 0x1000
	h, err := windows.OpenProcess(0x1000, false, uint32(pid))
	if err != nil {
		return times, err
	}
	defer windows.CloseHandle(h)

	err = syscall.GetProcessTimes(
		syscall.Handle(h),
		&times.CreateTime,
		&times.ExitTime,
		&times.KernelTime,
		&times.UserTime,
	)

	return times, err
}
