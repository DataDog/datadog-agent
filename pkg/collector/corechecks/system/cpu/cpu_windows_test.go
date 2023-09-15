// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build windows

package cpu

import (
	"testing"

	"github.com/DataDog/datadog-agent/pkg/metrics"

	"github.com/DataDog/datadog-agent/pkg/aggregator/mocksender"
	"github.com/DataDog/datadog-agent/pkg/autodiscovery/integration"
	gohaicpu "github.com/DataDog/datadog-agent/pkg/gohai/cpu"
	gohaiutils "github.com/DataDog/datadog-agent/pkg/gohai/utils"
	pdhtest "github.com/DataDog/datadog-agent/pkg/util/winutil/pdhutil"
)

func CPUInfo() *gohaicpu.Info {
	return &gohaicpu.Info{
		CPULogicalProcessors: gohaiutils.NewValue(uint64(1)),
	}
}

func TestCPUCheckWindows(t *testing.T) {
	cpuInfo = CPUInfo
	pdhtest.SetupTesting("..\\testfiles\\counter_indexes_en-us.txt", "..\\testfiles\\allcounters_en-us.txt")
	// The counters will have GetValue called twice because of the "Processor Information" issue workaround
	// see AddToQuery() in cpu_windows.go
	for i := 0; i < 2; i++ {
		pdhtest.SetQueryReturnValue("\\\\.\\Processor Information(_Total)\\% Interrupt Time", 0.1)
		pdhtest.SetQueryReturnValue("\\\\.\\Processor Information(_Total)\\% Idle Time", 80.1)
		pdhtest.SetQueryReturnValue("\\\\.\\Processor Information(_Total)\\% User Time", 11.3)
		pdhtest.SetQueryReturnValue("\\\\.\\Processor Information(_Total)\\% Privileged Time", 8.5)
	}

	cpuCheck := new(Check)
	m := mocksender.NewMockSender(cpuCheck.ID())
	cpuCheck.Configure(m.GetSenderManager(), integration.FakeConfigHash, nil, nil, "test")

	m.On(metrics.GaugeType.String(), "system.cpu.num_cores", 1.0, "", []string(nil)).Return().Times(1)
	m.On(metrics.GaugeType.String(), "system.cpu.interrupt", 0.1, "", []string(nil)).Return().Times(1)
	m.On(metrics.GaugeType.String(), "system.cpu.idle", 80.1, "", []string(nil)).Return().Times(1)
	m.On(metrics.GaugeType.String(), "system.cpu.user", 11.3, "", []string(nil)).Return().Times(1)
	m.On(metrics.GaugeType.String(), "system.cpu.system", 8.5, "", []string(nil)).Return().Times(1)
	m.On(metrics.GaugeType.String(), "system.cpu.iowait", 0.0, "", []string(nil)).Return().Times(1)
	m.On(metrics.GaugeType.String(), "system.cpu.stolen", 0.0, "", []string(nil)).Return().Times(1)
	m.On(metrics.GaugeType.String(), "system.cpu.guest", 0.0, "", []string(nil)).Return().Times(1)
	m.On("Commit").Return().Times(1)

	cpuCheck.Run()

	m.AssertExpectations(t)
	m.AssertNumberOfCalls(t, metrics.GaugeType.String(), 8)
	m.AssertNumberOfCalls(t, "Commit", 1)
}
