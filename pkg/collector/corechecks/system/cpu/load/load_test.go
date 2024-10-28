// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.
//go:build !windows

package load

import (
	"testing"

	"github.com/shirou/gopsutil/v3/load"

	"github.com/shirou/gopsutil/v3/cpu"

	"github.com/DataDog/datadog-agent/comp/core/autodiscovery/integration"
	"github.com/DataDog/datadog-agent/pkg/aggregator/mocksender"
)

var avgSample = load.AvgStat{
	Load1:  0.83,
	Load5:  0.96,
	Load15: 1.15,
}

func Avg() (*load.AvgStat, error) {
	return &avgSample, nil
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

func TestLoadCheckLinux(t *testing.T) {
	loadAvg = Avg
	cpuInfo = CPUInfo
	loadCheck := new(LoadCheck)
	mock := mocksender.NewMockSender(loadCheck.ID())
	loadCheck.Configure(mock.GetSenderManager(), integration.FakeConfigHash, nil, nil, "test")

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
