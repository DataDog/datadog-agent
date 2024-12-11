// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build windows

package cpu

import (
	"errors"
	"strings"
	"testing"

	"github.com/DataDog/datadog-agent/pkg/metrics"

	"github.com/DataDog/datadog-agent/comp/core/autodiscovery/integration"
	"github.com/DataDog/datadog-agent/pkg/aggregator"
	"github.com/DataDog/datadog-agent/pkg/aggregator/mocksender"
	"github.com/DataDog/datadog-agent/pkg/collector/check"
	gohaicpu "github.com/DataDog/datadog-agent/pkg/gohai/cpu"
	gohaiutils "github.com/DataDog/datadog-agent/pkg/gohai/utils"
	"github.com/DataDog/datadog-agent/pkg/util/pdhutil"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
)

func createCheck() check.Check {
	cpuCheckOpt := Factory()
	cpuCheckFunc, _ := cpuCheckOpt.Get()
	cpuCheck := cpuCheckFunc()
	return cpuCheck
}
func TestCPUCheckWindowsRunOk(t *testing.T) {
	cpuInfoFunc = func() *gohaicpu.Info {
		return &gohaicpu.Info{
			CPULogicalProcessors: gohaiutils.NewValue(uint64(1)),
		}
	}
	pdhutil.SetupTesting("..\\testfiles\\counter_indexes_en-us.txt", "..\\testfiles\\allcounters_en-us.txt")
	// The counters will have GetValue called twice because of the "Processor Information" issue workaround
	// see AddToQuery() in cpu_windows.go
	for i := 0; i < 2; i++ {
		pdhutil.SetQueryReturnValue("\\\\.\\Processor Information(_Total)\\% Interrupt Time", 0.1)
		pdhutil.SetQueryReturnValue("\\\\.\\Processor Information(_Total)\\% Idle Time", 80.1)
		pdhutil.SetQueryReturnValue("\\\\.\\Processor Information(_Total)\\% User Time", 11.3)
		pdhutil.SetQueryReturnValue("\\\\.\\Processor Information(_Total)\\% Privileged Time", 8.5)
	}
	cpuCheck := createCheck()
	m := mocksender.NewMockSender(cpuCheck.ID())
	m.On(metrics.GaugeType.String(), "system.cpu.num_cores", 1.0, "", []string(nil)).Return().Times(1)
	m.On(metrics.GaugeType.String(), "system.cpu.interrupt", 0.1, "", []string(nil)).Return().Times(1)
	m.On(metrics.GaugeType.String(), "system.cpu.idle", 80.1, "", []string(nil)).Return().Times(1)
	m.On(metrics.GaugeType.String(), "system.cpu.user", 11.3, "", []string(nil)).Return().Times(1)
	m.On(metrics.GaugeType.String(), "system.cpu.system", 8.5, "", []string(nil)).Return().Times(1)
	m.On(metrics.GaugeType.String(), "system.cpu.iowait", 0.0, "", []string(nil)).Return().Times(1)
	m.On(metrics.GaugeType.String(), "system.cpu.stolen", 0.0, "", []string(nil)).Return().Times(1)
	m.On(metrics.GaugeType.String(), "system.cpu.guest", 0.0, "", []string(nil)).Return().Times(1)
	m.On("Commit").Return().Times(1)

	cpuCheck.Configure(m.GetSenderManager(), integration.FakeConfigHash, nil, nil, "test")
	err := cpuCheck.Run()

	m.AssertExpectations(t)
	assert.Nil(t, err)
}

func TestCPUCheckWindowsErrorInInstanceConfig(t *testing.T) {
	cpuCheck := createCheck()
	m := mocksender.NewMockSender(cpuCheck.ID())

	err := cpuCheck.Configure(m.GetSenderManager(), integration.FakeConfigHash, []byte(`min_collection_interval: "string_value"`), nil, "test")

	assert.NotNil(t, err)
}

func TestCPUCheckWindowsErrorCPULogicalProcessors(t *testing.T) {
	cpuInfoFunc = func() *gohaicpu.Info {
		return &gohaicpu.Info{
			CPULogicalProcessors: gohaiutils.NewErrorValue[uint64](gohaiutils.ErrNotCollectable),
		}
	}
	cpuCheck := createCheck()
	m := mocksender.NewMockSender(cpuCheck.ID())

	err := cpuCheck.Configure(m.GetSenderManager(), integration.FakeConfigHash, nil, nil, "test")

	assert.True(t, strings.Contains(err.Error(), "cpu.Check: could not get number of CPU:"))
}

func TestCPUCheckWindowsErrorCreatePdhQuery(t *testing.T) {
	cpuInfoFunc = func() *gohaicpu.Info {
		return &gohaicpu.Info{
			CPULogicalProcessors: gohaiutils.NewValue(uint64(1)),
		}
	}
	createPdhQueryError := errors.New("createPdhQuery error")
	createPdhQuery = func() (PdhQueryInterface, error) {
		return nil, createPdhQueryError
	}
	cpuCheck := createCheck()
	m := mocksender.NewMockSender(cpuCheck.ID())

	err := cpuCheck.Configure(m.GetSenderManager(), integration.FakeConfigHash, nil, nil, "test")

	assert.Equal(t, createPdhQueryError, err)
}

type PdhQueryMock struct {
	mock.Mock
}

func (m *PdhQueryMock) AddCounter(counter pdhutil.PdhCounter) {
	m.Called(counter)
}

func (m *PdhQueryMock) CollectQueryData() error {
	args := m.Called()
	return args.Error(0)
}

func TestCPUCheckWindowsErrorCollectQueryData(t *testing.T) {
	cpuInfoFunc = func() *gohaicpu.Info {
		return &gohaicpu.Info{
			CPULogicalProcessors: gohaiutils.NewValue(uint64(1)),
		}
	}
	pdhQueryMock := &PdhQueryMock{}
	createPdhQuery = func() (PdhQueryInterface, error) {
		return pdhQueryMock, nil
	}
	pdhQueryMock.On("AddCounter", mock.Anything).Return()
	pdhQueryMock.On("CollectQueryData").Return(errors.New("collectQueryData error")).Times(1)
	cpuCheck := createCheck()
	m := mocksender.NewMockSender(cpuCheck.ID())
	m.On(metrics.GaugeType.String(), "system.cpu.num_cores", 1.0, "", []string(nil)).Return().Times(1)
	m.On(metrics.GaugeType.String(), "system.cpu.iowait", 0.0, "", []string(nil)).Return().Times(1)
	m.On(metrics.GaugeType.String(), "system.cpu.stolen", 0.0, "", []string(nil)).Return().Times(1)
	m.On(metrics.GaugeType.String(), "system.cpu.guest", 0.0, "", []string(nil)).Return().Times(1)
	m.On("Commit").Return().Times(1)

	cpuCheck.Configure(m.GetSenderManager(), integration.FakeConfigHash, nil, nil, "test")
	err := cpuCheck.Run()

	m.AssertExpectations(t)
	assert.Nil(t, err)
}

func TestCPUCheckWindowsErrorStoppedSender(t *testing.T) {
	stoppedSenderError := errors.New("demultiplexer is stopped")
	cpuInfoFunc = func() *gohaicpu.Info {
		return &gohaicpu.Info{
			CPULogicalProcessors: gohaiutils.NewValue(uint64(1)),
		}
	}
	cpuCheck := createCheck()
	m := mocksender.NewMockSender(cpuCheck.ID())

	cpuCheck.Configure(m.GetSenderManager(), integration.FakeConfigHash, nil, nil, "test")
	m.GetSenderManager().(*aggregator.AgentDemultiplexer).Stop(false)
	err := cpuCheck.Run()

	assert.Equal(t, stoppedSenderError, err)
}
