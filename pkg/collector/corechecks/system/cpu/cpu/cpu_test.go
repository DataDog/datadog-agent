// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.
//go:build !windows

package cpu

import (
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/DataDog/datadog-agent/comp/core/autodiscovery/integration"
	"github.com/DataDog/datadog-agent/pkg/aggregator"
	"github.com/DataDog/datadog-agent/pkg/aggregator/mocksender"
	"github.com/DataDog/datadog-agent/pkg/collector/check"
	"github.com/DataDog/datadog-agent/pkg/config"
	"github.com/DataDog/datadog-agent/pkg/metrics"

	"github.com/shirou/gopsutil/v3/cpu"
	"github.com/stretchr/testify/assert"
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
	cpuInfo = []cpu.InfoStat{
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
	}
)

func createCheck() check.Check {
	cpuCheckOpt := Factory()
	cpuCheckFunc, _ := cpuCheckOpt.Get()
	cpuCheck := cpuCheckFunc()
	return cpuCheck
}

func TestCPUCheckLinuxFirstRunOk(t *testing.T) {
	cpuInfoFunc = func() ([]cpu.InfoStat, error) {
		return cpuInfo, nil
	}
	cpuCheck := createCheck()
	m := mocksender.NewMockSender(cpuCheck.ID())
	if runtime.GOOS == "linux" {
		m.On(metrics.MonotonicCountType.String(), "system.cpu.context_switches", mock.AnythingOfType("float64"), mock.AnythingOfType("string"), mock.AnythingOfType("[]string")).Return().Times(1)
	}
	m.On(metrics.GaugeType.String(), "system.cpu.num_cores", float64(1), mock.AnythingOfType("string"), mock.AnythingOfType("[]string")).Return().Times(1)
	m.On("Commit").Return().Times(1)

	cpuCheck.Configure(m.GetSenderManager(), integration.FakeConfigHash, nil, nil, "test")
	cpuTimesFunc = func(bool) ([]cpu.TimesStat, error) {
		return firstSample, nil
	}
	err := cpuCheck.Run()

	assert.Nil(t, err)
	m.AssertExpectations(t)
}

func TestCPUCheckLinuxTwoRunsOk(t *testing.T) {
	cpuInfoFunc = func() ([]cpu.InfoStat, error) {
		return cpuInfo, nil
	}
	cpuCheck := createCheck()
	m := mocksender.NewMockSender(cpuCheck.ID())
	if runtime.GOOS == "linux" {
		m.On(metrics.MonotonicCountType.String(), "system.cpu.context_switches", mock.AnythingOfType("float64"), mock.AnythingOfType("string"), mock.AnythingOfType("[]string")).Return().Times(2)
	}
	m.On(metrics.GaugeType.String(), "system.cpu.user", 0.1913803067769472, "", []string(nil)).Return().Times(1)
	m.On(metrics.GaugeType.String(), "system.cpu.system", 5.026101621048045, "", []string(nil)).Return().Times(1)
	m.On(metrics.GaugeType.String(), "system.cpu.interrupt", 0.2889653146879648, "", []string(nil)).Return().Times(1)
	m.On(metrics.GaugeType.String(), "system.cpu.iowait", 0.03789709045088063, "", []string(nil)).Return().Times(1)
	m.On(metrics.GaugeType.String(), "system.cpu.idle", 94.74272612720159, "", []string(nil)).Return().Times(1)
	m.On(metrics.GaugeType.String(), "system.cpu.stolen", 0.0018948545225440318, "", []string(nil)).Return().Times(1)
	m.On(metrics.GaugeType.String(), "system.cpu.guest", 0.0, "", []string(nil)).Return().Times(1)
	m.On(metrics.GaugeType.String(), "system.cpu.num_cores", 1.0, "", []string(nil)).Return().Times(2)
	m.On("Commit").Return().Times(2)

	cpuCheck.Configure(m.GetSenderManager(), integration.FakeConfigHash, nil, nil, "test")
	cpuTimesFunc = func(bool) ([]cpu.TimesStat, error) {
		return firstSample, nil
	}
	cpuCheck.Run()
	cpuTimesFunc = func(bool) ([]cpu.TimesStat, error) {
		return secondSample, nil
	}
	err := cpuCheck.Run()

	assert.Nil(t, err)
	m.AssertExpectations(t)
}

func TestCPUCheckLinuxErrorInInstanceConfig(t *testing.T) {
	cpuCheck := createCheck()
	m := mocksender.NewMockSender(cpuCheck.ID())

	err := cpuCheck.Configure(m.GetSenderManager(), integration.FakeConfigHash, []byte(`min_collection_interval: "string_value"`), nil, "test")

	assert.NotNil(t, err)
}

func TestCPUCheckLinuxErrorInCpuInfo(t *testing.T) {
	cpuInfoError := errors.New("cpu.Check: could not query CPU info")
	cpuInfoFunc = func() ([]cpu.InfoStat, error) {
		return nil, cpuInfoError
	}
	cpuCheck := createCheck()
	m := mocksender.NewMockSender(cpuCheck.ID())

	err := cpuCheck.Configure(m.GetSenderManager(), integration.FakeConfigHash, nil, nil, "test")

	assert.Equal(t, cpuInfoError, err)
}

func TestCPUCheckLinuxErrorStoppedSender(t *testing.T) {
	stoppedSenderError := errors.New("demultiplexer is stopped")
	cpuInfoFunc = func() ([]cpu.InfoStat, error) {
		return cpuInfo, nil
	}
	cpuCheck := createCheck()
	m := mocksender.NewMockSender(cpuCheck.ID())

	cpuCheck.Configure(m.GetSenderManager(), integration.FakeConfigHash, nil, nil, "test")
	m.GetSenderManager().(*aggregator.AgentDemultiplexer).Stop(false)
	err := cpuCheck.Run()

	assert.Equal(t, stoppedSenderError, err)
}

func TestCPUCheckLinuxErrorProcFsPathNoExists(t *testing.T) {
	config.Datadog().SetDefault("procfs_path", "/tmp")
	cpuInfoFunc = func() ([]cpu.InfoStat, error) {
		return cpuInfo, nil
	}
	cpuTimesFunc = func(bool) ([]cpu.TimesStat, error) {
		return firstSample, nil
	}
	cpuCheck := createCheck()
	m := mocksender.NewMockSender(cpuCheck.ID())
	m.On(metrics.GaugeType.String(), "system.cpu.num_cores", float64(1), mock.AnythingOfType("string"), mock.AnythingOfType("[]string")).Return().Times(1)
	m.On("Commit").Return().Times(1)

	cpuCheck.Configure(m.GetSenderManager(), integration.FakeConfigHash, nil, nil, "test")
	err := cpuCheck.Run()

	assert.Nil(t, err)
	m.AssertExpectations(t)
	m.AssertNumberOfCalls(t, metrics.MonotonicCountType.String(), 0)
}

func TestCPUCheckLinuxErrorProcFsPathEmptyFile(t *testing.T) {
	tempFilePath := filepath.Join(os.TempDir(), "stat")
	tempFile, err := os.Create(tempFilePath)
	if err != nil {
		t.Fatal("Error creating temporary file:", err)
	}
	defer os.Remove(tempFile.Name())
	config.Datadog().SetDefault("procfs_path", os.TempDir())
	cpuInfoFunc = func() ([]cpu.InfoStat, error) {
		return cpuInfo, nil
	}
	cpuTimesFunc = func(bool) ([]cpu.TimesStat, error) {
		return firstSample, nil
	}
	cpuCheck := createCheck()
	m := mocksender.NewMockSender(cpuCheck.ID())
	m.On(metrics.GaugeType.String(), "system.cpu.num_cores", float64(1), mock.AnythingOfType("string"), mock.AnythingOfType("[]string")).Return().Times(1)
	m.On("Commit").Return().Times(1)

	cpuCheck.Configure(m.GetSenderManager(), integration.FakeConfigHash, nil, nil, "test")
	err = cpuCheck.Run()

	assert.Nil(t, err)
	m.AssertExpectations(t)
	m.AssertNumberOfCalls(t, metrics.MonotonicCountType.String(), 0)
}

func TestCPUCheckLinuxErrorProcFsPathWrongFormat(t *testing.T) {
	tempFilePath := filepath.Join(os.TempDir(), "stat")
	tempFile, err := os.Create(tempFilePath)
	if err != nil {
		t.Fatal("Error creating temporary file:", err)
	}
	defer os.Remove(tempFile.Name())
	_, err = tempFile.WriteString("ctxt string_value\n")
	if err != nil {
		t.Fatal("Error writing to temporary file:", err)
	}
	config.Datadog().SetDefault("procfs_path", os.TempDir())
	cpuInfoFunc = func() ([]cpu.InfoStat, error) {
		return cpuInfo, nil
	}
	cpuTimesFunc = func(bool) ([]cpu.TimesStat, error) {
		return firstSample, nil
	}
	cpuCheck := createCheck()
	m := mocksender.NewMockSender(cpuCheck.ID())
	m.On(metrics.GaugeType.String(), "system.cpu.num_cores", float64(1), mock.AnythingOfType("string"), mock.AnythingOfType("[]string")).Return().Times(1)
	m.On("Commit").Return().Times(1)

	cpuCheck.Configure(m.GetSenderManager(), integration.FakeConfigHash, nil, nil, "test")
	err = cpuCheck.Run()

	assert.Nil(t, err)
	m.AssertExpectations(t)
	m.AssertNumberOfCalls(t, metrics.MonotonicCountType.String(), 0)
}

func TestCPUCheckLinuxErrorInCpuTimes(t *testing.T) {
	cpuTimesError := errors.New("cpu Times error")
	cpuInfoFunc = func() ([]cpu.InfoStat, error) {
		return cpuInfo, nil
	}
	cpuTimesFunc = func(bool) ([]cpu.TimesStat, error) {
		return nil, cpuTimesError
	}
	cpuCheck := createCheck()
	m := mocksender.NewMockSender(cpuCheck.ID())

	cpuCheck.Configure(m.GetSenderManager(), integration.FakeConfigHash, nil, nil, "test")
	err := cpuCheck.Run()

	assert.Equal(t, cpuTimesError, err)
}

func TestCPUCheckLinuxEmptyCpuTimes(t *testing.T) {
	expectedError := errors.New("no cpu stats retrieve (empty results)")
	cpuInfoFunc = func() ([]cpu.InfoStat, error) {
		return cpuInfo, nil
	}
	cpuTimesFunc = func(bool) ([]cpu.TimesStat, error) {
		return []cpu.TimesStat{}, nil
	}
	cpuCheck := createCheck()
	m := mocksender.NewMockSender(cpuCheck.ID())

	cpuCheck.Configure(m.GetSenderManager(), integration.FakeConfigHash, nil, nil, "test")
	err := cpuCheck.Run()

	assert.Equal(t, expectedError, err)
}
