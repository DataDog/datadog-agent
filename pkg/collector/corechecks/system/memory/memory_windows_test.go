// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build windows

package memory

import (
	"testing"

	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/comp/core/autodiscovery/integration"
	"github.com/DataDog/datadog-agent/pkg/aggregator/mocksender"
	pdhtest "github.com/DataDog/datadog-agent/pkg/util/pdhutil"
	"github.com/DataDog/datadog-agent/pkg/util/winutil/winmem"
)

func VirtualMemory() (*winmem.VirtualMemoryStat, error) {
	return &winmem.VirtualMemoryStat{
		Total:       12345667890,
		Available:   234567890,
		Used:        10000000000,
		UsedPercent: 81,
	}, nil
}

func SwapMemory() (*winmem.SwapMemoryStat, error) {
	return &winmem.SwapMemoryStat{
		Total:       100000,
		Used:        40000,
		Free:        60000,
		UsedPercent: 40,
	}, nil
}

func PagefileMemory() (*winmem.PagefileStat, error) {
	return &winmem.PagefileStat{
		Total:       120000,
		Available:   90000,
		Used:        30000,
		UsedPercent: 50,
	}, nil
}

func PagingFileMemory() ([]*winmem.PagingFileStat, error) {
	return []*winmem.PagingFileStat{
		{
			Name:        "C:\\pagefile.sys",
			Total:       120000,
			Available:   90000,
			Used:        30000,
			UsedPercent: 50,
		},
	}, nil
}

func addDefaultQueryReturnValues() {
	pdhtest.SetQueryReturnValue("\\\\.\\Memory\\Cache Bytes", 3456789000.0)
	pdhtest.SetQueryReturnValue("\\\\.\\Memory\\Committed Bytes", 2345678000.0)
	pdhtest.SetQueryReturnValue("\\\\.\\Memory\\Pool Paged Bytes", 323456000.0)
	pdhtest.SetQueryReturnValue("\\\\.\\Memory\\Pool Nonpaged Bytes", 168900000.0)
}

func TestMemoryCheckWindowsMocked(t *testing.T) {
	virtualMemory = VirtualMemory
	swapMemory = SwapMemory
	pageMemory = PagefileMemory
	pagingFileMemory = PagingFileMemory

	pdhtest.SetupTesting("..\\testfiles\\counter_indexes_en-us.txt", "..\\testfiles\\allcounters_en-us.txt")
	addDefaultQueryReturnValues()

	memCheck := new(Check)
	mock := mocksender.NewMockSender(memCheck.ID())
	memCheck.Configure(mock.GetSenderManager(), integration.FakeConfigHash, nil, nil, "test")

	mock.On("Gauge", "system.mem.cached", 3456789000.0/mbSize, "", []string(nil)).Return().Times(1)
	mock.On("Gauge", "system.mem.committed", 2345678000.0/mbSize, "", []string(nil)).Return().Times(1)
	mock.On("Gauge", "system.mem.paged", 323456000.0/mbSize, "", []string(nil)).Return().Times(1)
	mock.On("Gauge", "system.mem.nonpaged", 168900000.0/mbSize, "", []string(nil)).Return().Times(1)
	mock.On("Gauge", "system.mem.free", 234567890.0/mbSize, "", []string(nil)).Return().Times(1)
	mock.On("Gauge", "system.mem.usable", 234567890.0/mbSize, "", []string(nil)).Return().Times(1)
	mock.On("Gauge", "system.mem.used", 12111100000.0/mbSize, "", []string(nil)).Return().Times(1)
	mock.On("Gauge", "system.mem.total", 12345667890.0/mbSize, "", []string(nil)).Return().Times(1)
	mock.On("Gauge", "system.mem.pct_usable", 0.19, "", []string(nil)).Return().Times(1)
	mock.On("Gauge", "system.swap.total", 100000.0/mbSize, "", []string(nil)).Return().Times(1)
	mock.On("Gauge", "system.swap.free", 60000.0/mbSize, "", []string(nil)).Return().Times(1)
	mock.On("Gauge", "system.swap.used", 40000.0/mbSize, "", []string(nil)).Return().Times(1)
	mock.On("Gauge", "system.swap.pct_free", 0.6, "", []string(nil)).Return().Times(1)

	mock.On("Gauge", "system.mem.pagefile.pct_free", 0.5, "", []string(nil)).Return().Times(1)
	mock.On("Gauge", "system.mem.pagefile.total", 120000/mbSize, "", []string(nil)).Return().Times(1)
	mock.On("Gauge", "system.mem.pagefile.used", 30000/mbSize, "", []string(nil)).Return().Times(1)
	mock.On("Gauge", "system.mem.pagefile.free", 90000/mbSize, "", []string(nil)).Return().Times(1)

	mock.On("Gauge", "system.paging.total", 120000/mbSize, "", []string{"pagefile_path:C:\\pagefile.sys"}).Return().Times(1)
	mock.On("Gauge", "system.paging.free", 90000/mbSize, "", []string{"pagefile_path:C:\\pagefile.sys"}).Return().Times(1)
	mock.On("Gauge", "system.paging.used", 30000/mbSize, "", []string{"pagefile_path:C:\\pagefile.sys"}).Return().Times(1)
	mock.On("Gauge", "system.paging.pct_free", 0.5, "", []string{"pagefile_path:C:\\pagefile.sys"}).Return().Times(1)

	mock.On("Commit").Return().Times(1)

	err := memCheck.Run()
	require.Nil(t, err)

	mock.AssertExpectations(t)
	mock.AssertNumberOfCalls(t, "Gauge", 21)
	mock.AssertNumberOfCalls(t, "Commit", 1)
}

func TestMemoryCheckWindows(t *testing.T) {

	instanceConfig := []byte(``)

	memCheck := new(Check)
	m := mocksender.NewMockSender(memCheck.ID())
	memCheck.Configure(m.GetSenderManager(), integration.FakeConfigHash, instanceConfig, nil, "test")

	// PDH counters may not be available in all environments (e.g., Windows containers)
	// Use Maybe() to allow these to be optional
	m.On("Gauge", "system.mem.cached", mock.AnythingOfType("float64"), "", []string(nil)).Return().Maybe()
	m.On("Gauge", "system.mem.committed", mock.AnythingOfType("float64"), "", []string(nil)).Return().Maybe()
	m.On("Gauge", "system.mem.paged", mock.AnythingOfType("float64"), "", []string(nil)).Return().Maybe()
	m.On("Gauge", "system.mem.nonpaged", mock.AnythingOfType("float64"), "", []string(nil)).Return().Maybe()

	// Core memory metrics should always be available
	m.On("Gauge", "system.mem.free", mock.AnythingOfType("float64"), "", []string(nil)).Return().Times(1)
	m.On("Gauge", "system.mem.usable", mock.AnythingOfType("float64"), "", []string(nil)).Return().Times(1)
	m.On("Gauge", "system.mem.used", mock.AnythingOfType("float64"), "", []string(nil)).Return().Times(1)
	m.On("Gauge", "system.mem.total", mock.AnythingOfType("float64"), "", []string(nil)).Return().Times(1)
	m.On("Gauge", "system.mem.pct_usable", mock.AnythingOfType("float64"), "", []string(nil)).Return().Times(1)
	m.On("Gauge", "system.swap.total", mock.AnythingOfType("float64"), "", []string(nil)).Return().Times(1)
	m.On("Gauge", "system.swap.free", mock.AnythingOfType("float64"), "", []string(nil)).Return().Times(1)
	m.On("Gauge", "system.swap.used", mock.AnythingOfType("float64"), "", []string(nil)).Return().Times(1)
	m.On("Gauge", "system.swap.pct_free", mock.AnythingOfType("float64"), "", []string(nil)).Return().Times(1)
	m.On("Gauge", "system.mem.pagefile.pct_free", mock.AnythingOfType("float64"), "", []string(nil)).Return().Times(1)
	m.On("Gauge", "system.mem.pagefile.total", mock.AnythingOfType("float64"), "", []string(nil)).Return().Times(1)
	m.On("Gauge", "system.mem.pagefile.used", mock.AnythingOfType("float64"), "", []string(nil)).Return().Times(1)
	m.On("Gauge", "system.mem.pagefile.free", mock.AnythingOfType("float64"), "", []string(nil)).Return().Times(1)

	m.On("Gauge", "system.paging.total", mock.AnythingOfType("float64"), "", mock.AnythingOfType("[]string")).Return().Times(1)
	m.On("Gauge", "system.paging.free", mock.AnythingOfType("float64"), "", mock.AnythingOfType("[]string")).Return().Times(1)
	m.On("Gauge", "system.paging.used", mock.AnythingOfType("float64"), "", mock.AnythingOfType("[]string")).Return().Times(1)
	m.On("Gauge", "system.paging.pct_free", mock.AnythingOfType("float64"), "", mock.AnythingOfType("[]string")).Return().Times(1)

	m.On("Commit").Return().Times(1)

	err := memCheck.Run()
	require.Nil(t, err)

	m.AssertExpectations(t)
	// Core metrics (17 minimum): 5 mem + 4 swap + 4 pagefile + 4 paging
	// Plus optional PDH counters (up to 4 more): cached, committed, paged, nonpaged
	// Total can be between 17-21 depending on environment
	require.GreaterOrEqual(t, len(m.Calls), 18, "Expected at least 18 calls (17 Gauge + 1 Commit)")
	require.LessOrEqual(t, len(m.Calls), 22, "Expected at most 22 calls (21 Gauge + 1 Commit)")
	m.AssertNumberOfCalls(t, "Commit", 1)
}
