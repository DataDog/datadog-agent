// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.
//go:build !windows

package cpu

import (
	"runtime"
	"testing"

	"github.com/DataDog/datadog-agent/pkg/aggregator/mocksender"
	"github.com/DataDog/datadog-agent/pkg/autodiscovery/integration"
	"github.com/DataDog/datadog-agent/pkg/metrics"

	"github.com/shirou/gopsutil/v3/cpu"
	"github.com/stretchr/testify/mock"
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
	cpuCheck := new(Check)
	cpuCheck.Configure(integration.FakeConfigHash, nil, nil, "test")

	m := mocksender.NewMockSender(cpuCheck.ID())
	m.On(metrics.GaugeType.String(), "system.cpu.num_cores", 1.0, "", []string(nil)).Return().Times(1)
	if runtime.GOOS == "linux" {
		m.On(metrics.MonotonicCountType.String(), "system.cpu.context_switches", mock.AnythingOfType("float64"), "", []string(nil)).Return().Times(1)
	}

	m.On("Commit").Return().Times(1)

	sample = firstSample
	cpuCheck.Run()

	m.AssertExpectations(t)
	m.AssertNumberOfCalls(t, metrics.GaugeType.String(), 1)
	if runtime.GOOS == "linux" {
		m.AssertNumberOfCalls(t, metrics.MonotonicCountType.String(), 1)
	}
	m.AssertNumberOfCalls(t, "Commit", 1)

	sample = secondSample
	m.On(metrics.GaugeType.String(), "system.cpu.user", 0.1913803067769472, "", []string(nil)).Return().Times(1)
	m.On(metrics.GaugeType.String(), "system.cpu.system", 5.026101621048045, "", []string(nil)).Return().Times(1)
	m.On(metrics.GaugeType.String(), "system.cpu.interrupt", 0.2889653146879648, "", []string(nil)).Return().Times(1)
	m.On(metrics.GaugeType.String(), "system.cpu.iowait", 0.03789709045088063, "", []string(nil)).Return().Times(1)
	m.On(metrics.GaugeType.String(), "system.cpu.idle", 94.74272612720159, "", []string(nil)).Return().Times(1)
	m.On(metrics.GaugeType.String(), "system.cpu.stolen", 0.0018948545225440318, "", []string(nil)).Return().Times(1)
	m.On(metrics.GaugeType.String(), "system.cpu.guest", 0.0, "", []string(nil)).Return().Times(1)
	m.On(metrics.GaugeType.String(), "system.cpu.num_cores", 1.0, "", []string(nil)).Return().Times(1)
	if runtime.GOOS == "linux" {
		m.On(metrics.MonotonicCountType.String(), "system.cpu.context_switches", mock.AnythingOfType("float64"), "", []string(nil)).Return().Times(1)
	}
	m.On("Commit").Return().Times(1)
	cpuCheck.Run()

	m.AssertExpectations(t)
	m.AssertNumberOfCalls(t, metrics.GaugeType.String(), 9)
	if runtime.GOOS == "linux" {
		m.AssertNumberOfCalls(t, metrics.MonotonicCountType.String(), 2)
	}
	m.AssertNumberOfCalls(t, "Commit", 2)
}
