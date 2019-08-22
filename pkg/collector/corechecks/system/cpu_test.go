// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-2019 Datadog, Inc.
// +build !windows

package system

import (
	"testing"

	"github.com/DataDog/datadog-agent/pkg/aggregator/mocksender"
	"github.com/shirou/gopsutil/cpu"
)

var (
	firstSample = []cpu.TimesStat{
		{
			CPU:       "cpu-total",
			User:      1229386,
			Nice:      623,
			System:    263584,
			Idle:      25496761,
			Iowait:    12113,
			Irq:       10,
			Softirq:   1151,
			Steal:     0,
			Guest:     0,
			GuestNice: 0,
			Stolen:    0,
		},
	}
	secondSample = []cpu.TimesStat{
		{
			CPU:       "cpu-total",
			User:      1229586,
			Nice:      625,
			System:    268584,
			Idle:      25596761,
			Iowait:    12153,
			Irq:       15,
			Softirq:   1451,
			Steal:     2,
			Guest:     0,
			GuestNice: 0,
			Stolen:    0,
		},
	}
)

var sample = firstSample

func CPUTimes(bool) ([]cpu.TimesStat, error) {
	return sample, nil
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
	cpuCheck.Configure(nil, nil, "test")

	mock := mocksender.NewMockSender(cpuCheck.ID())

	sample = firstSample
	cpuCheck.Run()

	mock.AssertExpectations(t)
	mock.AssertNumberOfCalls(t, "Gauge", 0)
	mock.AssertNumberOfCalls(t, "Commit", 0)

	sample = secondSample
	mock.On("Gauge", "system.cpu.user", 0.1913803067769472, "", []string(nil)).Return().Times(1)
	mock.On("Gauge", "system.cpu.system", 5.026101621048045, "", []string(nil)).Return().Times(1)
	mock.On("Gauge", "system.cpu.iowait", 0.03789709045088063, "", []string(nil)).Return().Times(1)
	mock.On("Gauge", "system.cpu.idle", 94.74272612720159, "", []string(nil)).Return().Times(1)
	mock.On("Gauge", "system.cpu.stolen", 0.0018948545225440318, "", []string(nil)).Return().Times(1)
	mock.On("Gauge", "system.cpu.guest", 0.0, "", []string(nil)).Return().Times(1)
	mock.On("Commit").Return().Times(1)
	cpuCheck.Run()

	mock.AssertExpectations(t)
	mock.AssertNumberOfCalls(t, "Gauge", 6)
	mock.AssertNumberOfCalls(t, "Commit", 1)
}
