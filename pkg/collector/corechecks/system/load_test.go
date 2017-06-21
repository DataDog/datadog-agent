package system

import (
	"testing"

	"github.com/shirou/gopsutil/load"
)

var (
	avgSample = load.AvgStat{
		Load1:  0.83,
		Load5:  0.96,
		Load15: 1.15,
	}
)

func Avg() (*load.AvgStat, error) {
	return &avgSample, nil
}

func TestLoadCheckLinux(t *testing.T) {
	loadAvg = Avg
	cpuInfo = CPUInfo
	loadCheck := new(LoadCheck)
	loadCheck.Configure(nil, nil)

	mock := new(MockSender)
	loadCheck.sender = mock

	var nbCPU float64
	info, _ := cpuInfo()
	for _, i := range info {
		nbCPU += float64(i.Cores)
	}

	mock.On("Gauge", "system.load.1", 0.83, "", []string(nil)).Return().Times(1)
	mock.On("Gauge", "system.load.5", 0.96, "", []string(nil)).Return().Times(1)
	mock.On("Gauge", "system.load.15", 1.15, "", []string(nil)).Return().Times(1)
	mock.On("Gauge", "system.load.norm.1", 0.83/nbCPU, "", []string(nil)).Return().Times(1)
	mock.On("Gauge", "system.load.norm.5", 0.96/nbCPU, "", []string(nil)).Return().Times(1)
	mock.On("Gauge", "system.load.norm.15", 1.15/nbCPU, "", []string(nil)).Return().Times(1)
	mock.On("Commit").Return().Times(1)
	loadCheck.Run()

	mock.AssertExpectations(t)
	mock.AssertNumberOfCalls(t, "Gauge", 6)
	mock.AssertNumberOfCalls(t, "Commit", 1)
}
