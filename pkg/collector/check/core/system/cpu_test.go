package system

import (
	"testing"

	"github.com/shirou/gopsutil/cpu"
)

func CPUTimes(bool) ([]cpu.TimesStat, error) {
	return []cpu.TimesStat{
		{
			CPU:       "cpu-total",
			User:      83452.7,
			System:    21180.0,
			Idle:      1974678.0,
			Nice:      146.9,
			Iowait:    934.0,
			Irq:       21.0,
			Softirq:   312.8,
			Steal:     35.0,
			Guest:     40.0,
			GuestNice: 22.0,
			Stolen:    18.0,
		},
	}, nil
}

func TestCPUCheckLinux(t *testing.T) {
	times = CPUTimes
	cpuCheck := new(CPUCheck)

	mock := new(MockSender) // from common_test.go
	cpuCheck.sender = mock

	mock.On("Gauge", "system.cpu.user", 83452.7, "", []string(nil)).Return().Times(1)
	mock.On("Gauge", "system.cpu.system", 21180.0, "", []string(nil)).Return().Times(1)
	mock.On("Gauge", "system.cpu.iowait", 934.0, "", []string(nil)).Return().Times(1)
	mock.On("Gauge", "system.cpu.idle", 1974678.0, "", []string(nil)).Return().Times(1)
	mock.On("Gauge", "system.cpu.stolen", 18.0, "", []string(nil)).Return().Times(1)
	mock.On("Gauge", "system.cpu.guest", 40.0, "", []string(nil)).Return().Times(1)
	mock.On("Commit").Return().Times(1)
	cpuCheck.Run()

	mock.AssertExpectations(t)
	mock.AssertNumberOfCalls(t, "Gauge", 6)
	mock.AssertNumberOfCalls(t, "Commit", 1)
}
