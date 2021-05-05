// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// +build windows

package cpu

import (
	"github.com/DataDog/datadog-agent/pkg/metrics"
	"testing"

	"github.com/DataDog/datadog-agent/pkg/aggregator/mocksender"
	"github.com/stretchr/testify/mock"
)

var (
	firstSample = []TimesStat{
		{
			CPU:    "cpu-total",
			User:   1229386,
			System: 263584,
			Idle:   25496761,
			Kernel: 25496761 + 263584,
		},
	}
	secondSample = []TimesStat{
		{
			CPU:    "cpu-total",
			User:   1229586,
			System: 268584,
			Idle:   25596761,
			Kernel: 25596761 + 268584,
		},
	}
)

var sample = firstSample

func CPUTimes() ([]TimesStat, error) {
	return sample, nil
}

func CPUInfo() (map[string]string, error) {
	return map[string]string{
		"cpu_logical_processors": "1",
	}, nil
}

func TestCPUCheckWindows(t *testing.T) {
	times = CPUTimes
	cpuInfo = CPUInfo
	cpuCheck := new(Check)
	cpuCheck.Configure(nil, nil, "test")

	m := mocksender.NewMockSender(cpuCheck.ID())
	m.On(metrics.GaugeType.String(), "system.cpu.num_cores", 1.0, "", []string(nil)).Return().Times(1)
	m.On(metrics.GaugeType.String(), "system.cpu.interrupt", mock.AnythingOfType("float64"), "", []string(nil)).Return().Times(1)

	m.On("Commit").Return().Times(1)

	sample = firstSample
	cpuCheck.Run()

	m.AssertExpectations(t)
	m.AssertNumberOfCalls(t, metrics.GaugeType.String(), 2)
	m.AssertNumberOfCalls(t, "Commit", 1)

	sample = secondSample
	m.On(metrics.GaugeType.String(), "system.cpu.user", 0.09746588693957114, "", []string(nil)).Return().Times(1)
	m.On(metrics.GaugeType.String(), "system.cpu.system", 2.4366471734892787, "", []string(nil)).Return().Times(1)
	m.On(metrics.GaugeType.String(), "system.cpu.iowait", 0.0, "", []string(nil)).Return().Times(1)
	m.On(metrics.GaugeType.String(), "system.cpu.idle", 48.732943469785575, "", []string(nil)).Return().Times(1)
	m.On(metrics.GaugeType.String(), "system.cpu.stolen", 0.0, "", []string(nil)).Return().Times(1)
	m.On(metrics.GaugeType.String(), "system.cpu.guest", 0.0, "", []string(nil)).Return().Times(1)
	m.On(metrics.GaugeType.String(), "system.cpu.num_cores", 1.0, "", []string(nil)).Return().Times(1)
	m.On(metrics.GaugeType.String(), "system.cpu.interrupt", mock.AnythingOfType("float64"), "", []string(nil)).Return().Times(1)
	m.On("Commit").Return().Times(1)
	cpuCheck.Run()

	m.AssertExpectations(t)
	m.AssertNumberOfCalls(t, metrics.GaugeType.String(), 10)
	m.AssertNumberOfCalls(t, "Commit", 2)
}
