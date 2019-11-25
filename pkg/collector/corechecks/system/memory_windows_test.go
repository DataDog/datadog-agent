// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-2019 Datadog, Inc.

// +build windows

package system

import (
	"testing"

	"github.com/DataDog/datadog-agent/pkg/aggregator/mocksender"
	"github.com/DataDog/datadog-agent/pkg/util/winutil"
	"github.com/stretchr/testify/require"
)

func VirtualMemory() (*winutil.VirtualMemoryStat, error) {
	return &winutil.VirtualMemoryStat{
		Total:       12345667890,
		Available:   234567890,
		Used:        10000000000,
		UsedPercent: 81,
	}, nil
}

func SwapMemory() (*winutil.SwapMemoryStat, error) {
	return &winutil.SwapMemoryStat{
		Total:       100000,
		Used:        40000,
		Free:        60000,
		UsedPercent: 40,
	}, nil
}

func PagefileMemory() (*winutil.PagefileStat, error) {
	return &winutil.PagefileStat{
		Total:       120000,
		Available:   90000,
		Used:        30000,
		UsedPercent: 50,
	}, nil
}

func TestMemoryCheckWindows(t *testing.T) {
	virtualMemory = VirtualMemory
	swapMemory = SwapMemory
	pageMemory = PagefileMemory
	memCheck := new(MemoryCheck)

	mock := mocksender.NewMockSender(memCheck.ID())

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

	mock.On("Commit").Return().Times(1)

	err := memCheck.Run()
	require.Nil(t, err)

	mock.AssertExpectations(t)
	mock.AssertNumberOfCalls(t, "Gauge", 13)
	mock.AssertNumberOfCalls(t, "Commit", 1)
}
