// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build !windows

package cpu

import (
	"errors"
	"testing"

	"github.com/DataDog/datadog-agent/comp/core/autodiscovery/integration"
	"github.com/DataDog/datadog-agent/pkg/aggregator"
	"github.com/DataDog/datadog-agent/pkg/aggregator/mocksender"
	"github.com/DataDog/datadog-agent/pkg/collector/check"
	"github.com/shirou/gopsutil/v4/cpu"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
)

var (
	firstTotalSample = cpu.TimesStat{
		CPU:       "cpu-total",
		User:      29386,
		Nice:      623,
		System:    63584,
		Idle:      96761,
		Iowait:    12113,
		Irq:       10,
		Softirq:   1151,
		Steal:     0,
		Guest:     0,
		GuestNice: 0,
	}
	secondTotalSample = cpu.TimesStat{
		CPU:       "cpu-total",
		User:      29586,
		Nice:      625,
		System:    68584,
		Idle:      96761,
		Iowait:    12153,
		Irq:       15,
		Softirq:   1451,
		Steal:     2,
		Guest:     0,
		GuestNice: 0,
	}
	perCPUSamples = []cpu.TimesStat{
		{
			CPU:       "cpu0",
			User:      83970.9,
			Nice:      0.0,
			System:    64060.9,
			Idle:      208877.4,
			Iowait:    12.1,
			Irq:       43.5,
			Softirq:   8.6,
			Steal:     65.9,
			Guest:     2.4,
			GuestNice: 5.1,
		},
		{
			CPU:       "cpu1",
			User:      82638.9,
			Nice:      50.0,
			System:    61564.1,
			Idle:      212758.8,
			Iowait:    1.2,
			Irq:       2.3,
			Softirq:   3.4,
			Steal:     4.5,
			Guest:     5.6,
			GuestNice: 6.7,
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

func setupDefaultMocks() {
	getContextSwitches = func() (int64, error) {
		return 4, nil
	}
	getCPUInfo = func() ([]cpu.InfoStat, error) {
		return cpuInfo, nil
	}
	getCPUTimes = func(perCpu bool) ([]cpu.TimesStat, error) {
		if perCpu {
			return perCPUSamples, nil
		}
		return []cpu.TimesStat{firstTotalSample}, nil
	}
}

func TestCPUCheckLinuxErrorInInstanceConfig(t *testing.T) {
	cpuCheck := createCheck()
	m := mocksender.NewMockSender(cpuCheck.ID())

	err := cpuCheck.Configure(m.GetSenderManager(), integration.FakeConfigHash, []byte(`min_collection_interval: "string_value"`), nil, "test")

	assert.NotNil(t, err)
}

func TestCPUCheckLinuxErrorReportTotalPerCPUConfigNotBoolean(t *testing.T) {
	cpuCheck := createCheck()
	m := mocksender.NewMockSender(cpuCheck.ID())

	err := cpuCheck.Configure(m.GetSenderManager(), integration.FakeConfigHash, []byte(`report_total_percpu: "string_value"`), nil, "test")

	assert.NotNil(t, err)
}

func TestCPUCheckLinuxErrorStoppedSender(t *testing.T) {
	stoppedSenderError := errors.New("demultiplexer is stopped")
	getCPUInfo = func() ([]cpu.InfoStat, error) {
		return cpuInfo, nil
	}
	cpuCheck := createCheck()
	m := mocksender.NewMockSender(cpuCheck.ID())
	m.SetupAcceptAll()

	cpuCheck.Configure(m.GetSenderManager(), integration.FakeConfigHash, nil, nil, "test")
	m.GetSenderManager().(*aggregator.AgentDemultiplexer).Stop(false)
	err := cpuCheck.Run()

	assert.Equal(t, stoppedSenderError, err)
}

func TestContextSwitchesError(t *testing.T) {
	setupDefaultMocks()
	getContextSwitches = func() (int64, error) {
		return 0, errors.New("GetContextSwitches error")
	}
	cpuCheck := createCheck()
	m := mocksender.NewMockSender(cpuCheck.ID())
	m.SetupAcceptAll()

	cpuCheck.Configure(m.GetSenderManager(), integration.FakeConfigHash, nil, nil, "test")
	err := cpuCheck.Run()

	assert.Nil(t, err)
	m.AssertNotCalled(t, "MonotonicCount", "system.cpu.context_switches", mock.AnythingOfType("float64"), mock.AnythingOfType("string"), mock.AnythingOfType("[]string"))
}

func TestContextSwitchesOk(t *testing.T) {
	setupDefaultMocks()
	cpuCheck := createCheck()
	m := mocksender.NewMockSender(cpuCheck.ID())
	m.SetupAcceptAll()

	cpuCheck.Configure(m.GetSenderManager(), integration.FakeConfigHash, nil, nil, "test")
	err := cpuCheck.Run()

	assert.Nil(t, err)
	m.AssertMetric(t, "MonotonicCount", "system.cpu.context_switches", 4, "", []string(nil))
}

func TestNumCoresError(t *testing.T) {
	setupDefaultMocks()
	cpuInfoError := errors.New("cpu.Check: could not query CPU info")
	getCPUInfo = func() ([]cpu.InfoStat, error) {
		return nil, cpuInfoError
	}
	cpuCheck := createCheck()
	m := mocksender.NewMockSender(cpuCheck.ID())
	m.SetupAcceptAll()

	cpuCheck.Configure(m.GetSenderManager(), integration.FakeConfigHash, nil, nil, "test")
	err := cpuCheck.Run()

	assert.Equal(t, cpuInfoError, err)
	m.AssertNotCalled(t, "Gauge", "system.cpu.num_cores", mock.AnythingOfType("float64"), mock.AnythingOfType("string"), mock.AnythingOfType("[]string"))
}

func TestNumCoresOk(t *testing.T) {
	setupDefaultMocks()
	cpuCheck := createCheck()
	m := mocksender.NewMockSender(cpuCheck.ID())
	m.SetupAcceptAll()

	cpuCheck.Configure(m.GetSenderManager(), integration.FakeConfigHash, nil, nil, "test")
	err := cpuCheck.Run()

	assert.Equal(t, nil, err)
	m.AssertMetric(t, "Gauge", "system.cpu.num_cores", 1, "", nil)
}

func TestSystemCpuMetricsError(t *testing.T) {
	setupDefaultMocks()
	cpuTimesError := errors.New("cpu.Check: could not query CPU times")
	getCPUTimes = func(bool) ([]cpu.TimesStat, error) {
		return nil, cpuTimesError
	}
	cpuCheck := createCheck()
	m := mocksender.NewMockSender(cpuCheck.ID())
	m.SetupAcceptAll()

	cpuCheck.Configure(m.GetSenderManager(), integration.FakeConfigHash, nil, nil, "test")
	err := cpuCheck.Run()

	assert.Equal(t, cpuTimesError, err)
	m.AssertNotCalled(t, "Gauge", "system.cpu.user", mock.AnythingOfType("float64"), mock.AnythingOfType("string"), mock.AnythingOfType("[]string"))
	m.AssertNotCalled(t, "Gauge", "system.cpu.system", mock.AnythingOfType("float64"), mock.AnythingOfType("string"), mock.AnythingOfType("[]string"))
	m.AssertNotCalled(t, "Gauge", "system.cpu.interrupt", mock.AnythingOfType("float64"), mock.AnythingOfType("string"), mock.AnythingOfType("[]string"))
	m.AssertNotCalled(t, "Gauge", "system.cpu.iowait", mock.AnythingOfType("float64"), mock.AnythingOfType("string"), mock.AnythingOfType("[]string"))
	m.AssertNotCalled(t, "Gauge", "system.cpu.idle", mock.AnythingOfType("float64"), mock.AnythingOfType("string"), mock.AnythingOfType("[]string"))
	m.AssertNotCalled(t, "Gauge", "system.cpu.stolen", mock.AnythingOfType("float64"), mock.AnythingOfType("string"), mock.AnythingOfType("[]string"))
	m.AssertNotCalled(t, "Gauge", "system.cpu.guest", mock.AnythingOfType("float64"), mock.AnythingOfType("string"), mock.AnythingOfType("[]string"))
}

func TestSystemCpuMetricsEmpty(t *testing.T) {
	setupDefaultMocks()
	expectedError := errors.New("no cpu stats retrieve (empty results)")
	getCPUTimes = func(bool) ([]cpu.TimesStat, error) {
		return []cpu.TimesStat{}, nil
	}
	cpuCheck := createCheck()
	m := mocksender.NewMockSender(cpuCheck.ID())
	m.SetupAcceptAll()

	cpuCheck.Configure(m.GetSenderManager(), integration.FakeConfigHash, nil, nil, "test")
	err := cpuCheck.Run()

	assert.Equal(t, expectedError, err)
	m.AssertNotCalled(t, "Gauge", "system.cpu.user", mock.AnythingOfType("float64"), mock.AnythingOfType("string"), mock.AnythingOfType("[]string"))
	m.AssertNotCalled(t, "Gauge", "system.cpu.system", mock.AnythingOfType("float64"), mock.AnythingOfType("string"), mock.AnythingOfType("[]string"))
	m.AssertNotCalled(t, "Gauge", "system.cpu.interrupt", mock.AnythingOfType("float64"), mock.AnythingOfType("string"), mock.AnythingOfType("[]string"))
	m.AssertNotCalled(t, "Gauge", "system.cpu.iowait", mock.AnythingOfType("float64"), mock.AnythingOfType("string"), mock.AnythingOfType("[]string"))
	m.AssertNotCalled(t, "Gauge", "system.cpu.idle", mock.AnythingOfType("float64"), mock.AnythingOfType("string"), mock.AnythingOfType("[]string"))
	m.AssertNotCalled(t, "Gauge", "system.cpu.stolen", mock.AnythingOfType("float64"), mock.AnythingOfType("string"), mock.AnythingOfType("[]string"))
	m.AssertNotCalled(t, "Gauge", "system.cpu.guest", mock.AnythingOfType("float64"), mock.AnythingOfType("string"), mock.AnythingOfType("[]string"))
}

func TestSystemCpuMetricsNotReportedOnFirstCheck(t *testing.T) {
	setupDefaultMocks()
	cpuCheck := createCheck()
	m := mocksender.NewMockSender(cpuCheck.ID())
	m.SetupAcceptAll()

	cpuCheck.Configure(m.GetSenderManager(), integration.FakeConfigHash, nil, nil, "test")
	err := cpuCheck.Run()

	assert.Equal(t, nil, err)
	m.AssertNotCalled(t, "Gauge", "system.cpu.user", mock.AnythingOfType("float64"), mock.AnythingOfType("string"), mock.AnythingOfType("[]string"))
	m.AssertNotCalled(t, "Gauge", "system.cpu.system", mock.AnythingOfType("float64"), mock.AnythingOfType("string"), mock.AnythingOfType("[]string"))
	m.AssertNotCalled(t, "Gauge", "system.cpu.interrupt", mock.AnythingOfType("float64"), mock.AnythingOfType("string"), mock.AnythingOfType("[]string"))
	m.AssertNotCalled(t, "Gauge", "system.cpu.iowait", mock.AnythingOfType("float64"), mock.AnythingOfType("string"), mock.AnythingOfType("[]string"))
	m.AssertNotCalled(t, "Gauge", "system.cpu.idle", mock.AnythingOfType("float64"), mock.AnythingOfType("string"), mock.AnythingOfType("[]string"))
	m.AssertNotCalled(t, "Gauge", "system.cpu.stolen", mock.AnythingOfType("float64"), mock.AnythingOfType("string"), mock.AnythingOfType("[]string"))
	m.AssertNotCalled(t, "Gauge", "system.cpu.guest", mock.AnythingOfType("float64"), mock.AnythingOfType("string"), mock.AnythingOfType("[]string"))
}

func TestSystemCpuMetricsReportedOnSecondCheck(t *testing.T) {
	setupDefaultMocks()
	firstCall := true
	getCPUTimes = func(perCpu bool) ([]cpu.TimesStat, error) {
		if perCpu {
			return perCPUSamples, nil
		}
		if firstCall {
			firstCall = false
			return []cpu.TimesStat{firstTotalSample}, nil
		}
		return []cpu.TimesStat{secondTotalSample}, nil
	}
	cpuCheck := createCheck()
	m := mocksender.NewMockSender(cpuCheck.ID())
	m.SetupAcceptAll()

	cpuCheck.Configure(m.GetSenderManager(), integration.FakeConfigHash, nil, nil, "test")
	cpuCheck.Run()
	err := cpuCheck.Run()

	assert.Equal(t, nil, err)
	m.AssertMetric(t, "Gauge", "system.cpu.user", 3.640295548747522, "", []string(nil))
	m.AssertMetric(t, "Gauge", "system.cpu.system", 95.60281131735448, "", []string(nil))
	m.AssertMetric(t, "Gauge", "system.cpu.interrupt", 5.496485853306902, "", []string(nil))
	m.AssertMetric(t, "Gauge", "system.cpu.iowait", 0.7208506037123806, "", []string(nil))
	m.AssertMetric(t, "Gauge", "system.cpu.idle", 0.0, "", []string(nil))
	m.AssertMetric(t, "Gauge", "system.cpu.stolen", 0.03604253018561903, "", []string(nil))
	m.AssertMetric(t, "Gauge", "system.cpu.guest", 0.0, "", []string(nil))
}

func TestSystemCpuMetricsPerCpuError(t *testing.T) {
	setupDefaultMocks()
	cpuTimesError := errors.New("cpu.Check: could not query CPU times")
	getCPUTimes = func(perCpu bool) ([]cpu.TimesStat, error) {
		if perCpu {
			return nil, cpuTimesError
		}
		return []cpu.TimesStat{firstTotalSample}, nil
	}
	cpuCheck := createCheck()
	m := mocksender.NewMockSender(cpuCheck.ID())
	m.SetupAcceptAll()

	cpuCheck.Configure(m.GetSenderManager(), integration.FakeConfigHash, []byte(`report_total_percpu: true`), nil, "test")
	err := cpuCheck.Run()

	assert.Equal(t, cpuTimesError, err)
	m.AssertNotCalled(t, "Gauge", "system.cpu.user.total", mock.AnythingOfType("float64"), mock.AnythingOfType("string"), mock.AnythingOfType("[]string"))
	m.AssertNotCalled(t, "Gauge", "system.cpu.nice.total", mock.AnythingOfType("float64"), mock.AnythingOfType("string"), mock.AnythingOfType("[]string"))
	m.AssertNotCalled(t, "Gauge", "system.cpu.system.total", mock.AnythingOfType("float64"), mock.AnythingOfType("string"), mock.AnythingOfType("[]string"))
	m.AssertNotCalled(t, "Gauge", "system.cpu.idle.total", mock.AnythingOfType("float64"), mock.AnythingOfType("string"), mock.AnythingOfType("[]string"))
	m.AssertNotCalled(t, "Gauge", "system.cpu.iowait.total", mock.AnythingOfType("float64"), mock.AnythingOfType("string"), mock.AnythingOfType("[]string"))
	m.AssertNotCalled(t, "Gauge", "system.cpu.irq.total", mock.AnythingOfType("float64"), mock.AnythingOfType("string"), mock.AnythingOfType("[]string"))
	m.AssertNotCalled(t, "Gauge", "system.cpu.softirq.total", mock.AnythingOfType("float64"), mock.AnythingOfType("string"), mock.AnythingOfType("[]string"))
	m.AssertNotCalled(t, "Gauge", "system.cpu.steal.total", mock.AnythingOfType("float64"), mock.AnythingOfType("string"), mock.AnythingOfType("[]string"))
	m.AssertNotCalled(t, "Gauge", "system.cpu.guest.total", mock.AnythingOfType("float64"), mock.AnythingOfType("string"), mock.AnythingOfType("[]string"))
	m.AssertNotCalled(t, "Gauge", "system.cpu.guestnice.total", mock.AnythingOfType("float64"), mock.AnythingOfType("string"), mock.AnythingOfType("[]string"))

}

func TestSystemCpuMetricsPerCpuDefault(t *testing.T) {
	setupDefaultMocks()
	getCPUTimes = func(perCpu bool) ([]cpu.TimesStat, error) {
		if perCpu {
			return perCPUSamples, nil
		}
		return []cpu.TimesStat{firstTotalSample}, nil
	}
	cpuCheck := createCheck()
	m := mocksender.NewMockSender(cpuCheck.ID())
	m.SetupAcceptAll()

	cpuCheck.Configure(m.GetSenderManager(), integration.FakeConfigHash, nil, nil, "test")
	err := cpuCheck.Run()

	assert.Equal(t, nil, err)
	m.AssertMetric(t, "Gauge", "system.cpu.user.total", 29386, "", []string{"core:cpu-total"})
	m.AssertMetric(t, "Gauge", "system.cpu.nice.total", 623, "", []string{"core:cpu-total"})
	m.AssertMetric(t, "Gauge", "system.cpu.system.total", 63584, "", []string{"core:cpu-total"})
	m.AssertMetric(t, "Gauge", "system.cpu.idle.total", 96761, "", []string{"core:cpu-total"})
	m.AssertMetric(t, "Gauge", "system.cpu.iowait.total", 12113, "", []string{"core:cpu-total"})
	m.AssertMetric(t, "Gauge", "system.cpu.irq.total", 10, "", []string{"core:cpu-total"})
	m.AssertMetric(t, "Gauge", "system.cpu.softirq.total", 1151, "", []string{"core:cpu-total"})
	m.AssertMetric(t, "Gauge", "system.cpu.steal.total", 0.0, "", []string{"core:cpu-total"})
	m.AssertMetric(t, "Gauge", "system.cpu.guest.total", 0.0, "", []string{"core:cpu-total"})
	m.AssertMetric(t, "Gauge", "system.cpu.guestnice.total", 0.0, "", []string{"core:cpu-total"})
}
func TestSystemCpuMetricsPerCpuFalse(t *testing.T) {
	setupDefaultMocks()
	getCPUTimes = func(perCpu bool) ([]cpu.TimesStat, error) {
		if perCpu {
			return perCPUSamples, nil
		}
		return []cpu.TimesStat{firstTotalSample}, nil
	}
	cpuCheck := createCheck()
	m := mocksender.NewMockSender(cpuCheck.ID())
	m.SetupAcceptAll()

	cpuCheck.Configure(m.GetSenderManager(), integration.FakeConfigHash, []byte(`report_total_percpu: false`), nil, "test")
	err := cpuCheck.Run()

	assert.Equal(t, nil, err)
	m.AssertMetric(t, "Gauge", "system.cpu.user.total", 29386, "", []string{"core:cpu-total"})
	m.AssertMetric(t, "Gauge", "system.cpu.nice.total", 623, "", []string{"core:cpu-total"})
	m.AssertMetric(t, "Gauge", "system.cpu.system.total", 63584, "", []string{"core:cpu-total"})
	m.AssertMetric(t, "Gauge", "system.cpu.idle.total", 96761, "", []string{"core:cpu-total"})
	m.AssertMetric(t, "Gauge", "system.cpu.iowait.total", 12113, "", []string{"core:cpu-total"})
	m.AssertMetric(t, "Gauge", "system.cpu.irq.total", 10, "", []string{"core:cpu-total"})
	m.AssertMetric(t, "Gauge", "system.cpu.softirq.total", 1151, "", []string{"core:cpu-total"})
	m.AssertMetric(t, "Gauge", "system.cpu.steal.total", 0.0, "", []string{"core:cpu-total"})
	m.AssertMetric(t, "Gauge", "system.cpu.guest.total", 0.0, "", []string{"core:cpu-total"})
	m.AssertMetric(t, "Gauge", "system.cpu.guestnice.total", 0.0, "", []string{"core:cpu-total"})
}
func TestSystemCpuMetricsPerCpuTrue(t *testing.T) {
	setupDefaultMocks()
	getCPUTimes = func(perCpu bool) ([]cpu.TimesStat, error) {
		if perCpu {
			return perCPUSamples, nil
		}
		return []cpu.TimesStat{firstTotalSample}, nil
	}
	cpuCheck := createCheck()
	m := mocksender.NewMockSender(cpuCheck.ID())
	m.SetupAcceptAll()

	cpuCheck.Configure(m.GetSenderManager(), integration.FakeConfigHash, []byte(`report_total_percpu: true`), nil, "test")
	err := cpuCheck.Run()

	assert.Equal(t, nil, err)
	m.AssertMetric(t, "Gauge", "system.cpu.user.total", 83970.9, "", []string{"core:cpu0"})
	m.AssertMetric(t, "Gauge", "system.cpu.user.total", 82638.9, "", []string{"core:cpu1"})
	m.AssertMetric(t, "Gauge", "system.cpu.nice.total", 0.0, "", []string{"core:cpu0"})
	m.AssertMetric(t, "Gauge", "system.cpu.nice.total", 50.0, "", []string{"core:cpu1"})
	m.AssertMetric(t, "Gauge", "system.cpu.system.total", 64060.9, "", []string{"core:cpu0"})
	m.AssertMetric(t, "Gauge", "system.cpu.system.total", 61564.1, "", []string{"core:cpu1"})
	m.AssertMetric(t, "Gauge", "system.cpu.idle.total", 208877.4, "", []string{"core:cpu0"})
	m.AssertMetric(t, "Gauge", "system.cpu.idle.total", 212758.8, "", []string{"core:cpu1"})
	m.AssertMetric(t, "Gauge", "system.cpu.iowait.total", 12.1, "", []string{"core:cpu0"})
	m.AssertMetric(t, "Gauge", "system.cpu.iowait.total", 1.2, "", []string{"core:cpu1"})
	m.AssertMetric(t, "Gauge", "system.cpu.irq.total", 43.5, "", []string{"core:cpu0"})
	m.AssertMetric(t, "Gauge", "system.cpu.irq.total", 2.3, "", []string{"core:cpu1"})
	m.AssertMetric(t, "Gauge", "system.cpu.softirq.total", 8.6, "", []string{"core:cpu0"})
	m.AssertMetric(t, "Gauge", "system.cpu.softirq.total", 3.4, "", []string{"core:cpu1"})
	m.AssertMetric(t, "Gauge", "system.cpu.steal.total", 65.9, "", []string{"core:cpu0"})
	m.AssertMetric(t, "Gauge", "system.cpu.steal.total", 4.5, "", []string{"core:cpu1"})
	m.AssertMetric(t, "Gauge", "system.cpu.guest.total", 2.4, "", []string{"core:cpu0"})
	m.AssertMetric(t, "Gauge", "system.cpu.guest.total", 5.6, "", []string{"core:cpu1"})
	m.AssertMetric(t, "Gauge", "system.cpu.guestnice.total", 5.1, "", []string{"core:cpu0"})
	m.AssertMetric(t, "Gauge", "system.cpu.guestnice.total", 6.7, "", []string{"core:cpu1"})
}
