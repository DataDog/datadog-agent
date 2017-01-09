package system

import (
	"testing"

	"github.com/shirou/gopsutil/cpu"
)

func CPUTimes(bool) ([]cpu.TimesStat, error) {
	return []cpu.TimesStat{
		{
			CPU:       "cpu-total",
			User:      1229386,
			Nice:      623,
			System:    263584,
			Idle:      25496761,
			Iowait:    12113,
			Irq:       0,
			Softirq:   1151,
			Steal:     0,
			Guest:     0,
			GuestNice: 0,
			Stolen:    0,
		},
	}, nil
}

func CPUInfo() ([]cpu.InfoStat, error) {
	return []cpu.InfoStat{
		{
			CPU:        0,
			VendorID:   "GenuineIntel",
			Family:     "6",
			Model:      "61",
			Stepping:   4,
			PhysicalID: "0",
			CoreID:     "0",
			Cores:      1,
			ModelName:  "Intel(R)Core(TM) i7-5557U CPU @3.10GHz",
			Mhz:        3400,
			CacheSize:  4096,
			Flags:      nil,
		},
	}, nil
}

func TestCPUCheckLinux(t *testing.T) {
	times = CPUTimes
	cpuInfo = CPUInfo
	cpuCheck := new(CPUCheck)
	cpuCheck.Configure(nil)

	mock := new(MockSender) // from common_test.go
	cpuCheck.sender = mock

	mock.On("Rate", "system.cpu.user", 1.229386e+08, "", []string(nil)).Return().Times(1)
	mock.On("Rate", "system.cpu.system", 2.63584e+07, "", []string(nil)).Return().Times(1)
	mock.On("Rate", "system.cpu.iowait", 1.2113e+06, "", []string(nil)).Return().Times(1)
	mock.On("Rate", "system.cpu.idle", 2.5496761e+09, "", []string(nil)).Return().Times(1)
	mock.On("Rate", "system.cpu.stolen", 0.0, "", []string(nil)).Return().Times(1)
	mock.On("Rate", "system.cpu.guest", 0.0, "", []string(nil)).Return().Times(1)
	mock.On("Commit").Return().Times(1)
	cpuCheck.Run()

	mock.AssertExpectations(t)
	mock.AssertNumberOfCalls(t, "Rate", 6)
	mock.AssertNumberOfCalls(t, "Commit", 1)
}
