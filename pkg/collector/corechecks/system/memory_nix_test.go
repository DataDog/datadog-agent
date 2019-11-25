// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-2019 Datadog, Inc.

// +build !windows

package system

import (
	"fmt"
	"testing"

	"github.com/DataDog/datadog-agent/pkg/aggregator/mocksender"
	"github.com/shirou/gopsutil/mem"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func VirtualMemory() (*mem.VirtualMemoryStat, error) {
	return &mem.VirtualMemoryStat{
		Total:        12345667890,
		Available:    234567890,
		Used:         10000000000,
		UsedPercent:  81,
		Free:         11554304000,
		Active:       2506516070400,
		Inactive:     970587111424,
		Wired:        0,
		Buffers:      353818902528,
		Cached:       2596446142464,
		Writeback:    0,
		Dirty:        314572800,
		WritebackTmp: 0,
		Shared:       327680000000,
		Slab:         327680000000,
		PageTables:   37790679040,
		SwapCached:   25000000000,
		CommitLimit:  785338368,
		CommittedAS:  433750016,
	}, nil
}

func SwapMemory() (*mem.SwapMemoryStat, error) {
	return &mem.SwapMemoryStat{
		Total:       100000,
		Used:        40000,
		Free:        60000,
		UsedPercent: 40,
		Sin:         21,
		Sout:        22,
	}, nil
}

func TestMemoryCheckLinux(t *testing.T) {
	virtualMemory = VirtualMemory
	swapMemory = SwapMemory
	memCheck := new(MemoryCheck)

	mock := mocksender.NewMockSender(memCheck.ID())

	runtimeOS = "linux"

	mock.On("Gauge", "system.mem.free", 11554304000.0/mbSize, "", []string(nil)).Return().Times(1)
	mock.On("Gauge", "system.mem.usable", 234567890.0/mbSize, "", []string(nil)).Return().Times(1)
	mock.On("Gauge", "system.mem.total", 12345667890.0/mbSize, "", []string(nil)).Return().Times(1)
	mock.On("Gauge", "system.mem.used", 791363890/mbSize, "", []string(nil)).Return().Times(1)
	mock.On("Gauge", "system.mem.pct_usable", 0.019000016207304602, "", []string(nil)).Return().Times(1)
	mock.On("Gauge", "system.mem.cached", 2596446142464.0/mbSize, "", []string(nil)).Return().Times(1)
	mock.On("Gauge", "system.mem.buffered", 353818902528.0/mbSize, "", []string(nil)).Return().Times(1)
	mock.On("Gauge", "system.mem.shared", 327680000000.0/mbSize, "", []string(nil)).Return().Times(1)
	mock.On("Gauge", "system.mem.slab", 327680000000.0/mbSize, "", []string(nil)).Return().Times(1)
	mock.On("Gauge", "system.mem.page_tables", 37790679040.0/mbSize, "", []string(nil)).Return().Times(1)
	mock.On("Gauge", "system.mem.commit_limit", 785338368.0/mbSize, "", []string(nil)).Return().Times(1)
	mock.On("Gauge", "system.mem.committed_as", 433750016.0/mbSize, "", []string(nil)).Return().Times(1)
	mock.On("Gauge", "system.swap.total", 100000.0/mbSize, "", []string(nil)).Return().Times(1)
	mock.On("Gauge", "system.swap.free", 60000.0/mbSize, "", []string(nil)).Return().Times(1)
	mock.On("Gauge", "system.swap.used", 40000.0/mbSize, "", []string(nil)).Return().Times(1)
	mock.On("Gauge", "system.swap.pct_free", 0.6, "", []string(nil)).Return().Times(1)
	mock.On("Gauge", "system.swap.cached", 25000000000.0/mbSize, "", []string(nil)).Return().Times(1)
	mock.On("Commit").Return().Times(1)

	err := memCheck.Run()
	require.Nil(t, err)

	mock.AssertExpectations(t)
	mock.AssertNumberOfCalls(t, "Gauge", 17)
	mock.AssertNumberOfCalls(t, "Commit", 1)
}

func TestMemoryCheckFreebsd(t *testing.T) {
	virtualMemory = VirtualMemory
	swapMemory = SwapMemory
	memCheck := new(MemoryCheck)

	mock := mocksender.NewMockSender(memCheck.ID())

	runtimeOS = "freebsd"

	mock.On("Gauge", "system.mem.total", 12345667890.0/mbSize, "", []string(nil)).Return().Times(1)
	mock.On("Gauge", "system.mem.free", 11554304000.0/mbSize, "", []string(nil)).Return().Times(1)
	mock.On("Gauge", "system.mem.used", 791363890/mbSize, "", []string(nil)).Return().Times(1)
	mock.On("Gauge", "system.mem.usable", 234567890.0/mbSize, "", []string(nil)).Return().Times(1)
	mock.On("Gauge", "system.mem.pct_usable", 0.019000016207304602, "", []string(nil)).Return().Times(1)
	mock.On("Gauge", "system.mem.cached", 2596446142464.0/mbSize, "", []string(nil)).Return().Times(1)
	mock.On("Gauge", "system.swap.total", 100000.0/mbSize, "", []string(nil)).Return().Times(1)
	mock.On("Gauge", "system.swap.free", 60000.0/mbSize, "", []string(nil)).Return().Times(1)
	mock.On("Gauge", "system.swap.used", 40000.0/mbSize, "", []string(nil)).Return().Times(1)
	mock.On("Gauge", "system.swap.pct_free", 0.6, "", []string(nil)).Return().Times(1)
	mock.On("Commit").Return().Times(1)
	err := memCheck.Run()
	require.Nil(t, err)

	mock.AssertExpectations(t)
	mock.AssertNumberOfCalls(t, "Gauge", 10)
	mock.AssertNumberOfCalls(t, "Commit", 1)
}

func TestMemoryCheckDarwin(t *testing.T) {
	virtualMemory = VirtualMemory
	swapMemory = SwapMemory
	memCheck := new(MemoryCheck)

	mock := mocksender.NewMockSender(memCheck.ID())

	runtimeOS = "darwin"

	mock.On("Gauge", "system.mem.total", 12345667890.0/mbSize, "", []string(nil)).Return().Times(1)
	mock.On("Gauge", "system.mem.free", 11554304000.0/mbSize, "", []string(nil)).Return().Times(1)
	mock.On("Gauge", "system.mem.used", 791363890/mbSize, "", []string(nil)).Return().Times(1)
	mock.On("Gauge", "system.mem.usable", 234567890.0/mbSize, "", []string(nil)).Return().Times(1)
	mock.On("Gauge", "system.mem.pct_usable", 0.019000016207304602, "", []string(nil)).Return().Times(1)
	mock.On("Gauge", "system.swap.total", 100000.0/mbSize, "", []string(nil)).Return().Times(1)
	mock.On("Gauge", "system.swap.free", 60000.0/mbSize, "", []string(nil)).Return().Times(1)
	mock.On("Gauge", "system.swap.used", 40000.0/mbSize, "", []string(nil)).Return().Times(1)
	mock.On("Gauge", "system.swap.pct_free", 0.6, "", []string(nil)).Return().Times(1)
	mock.On("Commit").Return().Times(1)
	err := memCheck.Run()
	require.Nil(t, err)

	mock.AssertExpectations(t)
	mock.AssertNumberOfCalls(t, "Gauge", 9)
	mock.AssertNumberOfCalls(t, "Commit", 1)
}

func TestMemoryError(t *testing.T) {
	virtualMemory = func() (*mem.VirtualMemoryStat, error) { return nil, fmt.Errorf("some error") }
	swapMemory = func() (*mem.SwapMemoryStat, error) { return nil, fmt.Errorf("some error") }
	memCheck := new(MemoryCheck)

	mock := mocksender.NewMockSender(memCheck.ID())

	runtimeOS = "linux"

	err := memCheck.Run()
	assert.NotNil(t, err)

	mock.AssertExpectations(t)
	mock.AssertNumberOfCalls(t, "Gauge", 0)
	mock.AssertNumberOfCalls(t, "Commit", 0)
}

func TestSwapMemoryError(t *testing.T) {
	virtualMemory = VirtualMemory
	swapMemory = func() (*mem.SwapMemoryStat, error) { return nil, fmt.Errorf("some error") }
	memCheck := new(MemoryCheck)

	mock := mocksender.NewMockSender(memCheck.ID())

	runtimeOS = "linux"

	mock.On("Gauge", "system.mem.total", 12345667890.0/mbSize, "", []string(nil)).Return().Times(1)
	mock.On("Gauge", "system.mem.free", 11554304000.0/mbSize, "", []string(nil)).Return().Times(1)
	mock.On("Gauge", "system.mem.used", 791363890/mbSize, "", []string(nil)).Return().Times(1)
	mock.On("Gauge", "system.mem.usable", 234567890.0/mbSize, "", []string(nil)).Return().Times(1)
	mock.On("Gauge", "system.mem.pct_usable", 0.019000016207304602, "", []string(nil)).Return().Times(1)
	mock.On("Gauge", "system.mem.cached", 2596446142464.0/mbSize, "", []string(nil)).Return().Times(1)
	mock.On("Gauge", "system.mem.buffered", 353818902528.0/mbSize, "", []string(nil)).Return().Times(1)
	mock.On("Gauge", "system.mem.shared", 327680000000.0/mbSize, "", []string(nil)).Return().Times(1)
	mock.On("Gauge", "system.mem.slab", 327680000000.0/mbSize, "", []string(nil)).Return().Times(1)
	mock.On("Gauge", "system.mem.page_tables", 37790679040.0/mbSize, "", []string(nil)).Return().Times(1)
	mock.On("Gauge", "system.mem.commit_limit", 785338368.0/mbSize, "", []string(nil)).Return().Times(1)
	mock.On("Gauge", "system.mem.committed_as", 433750016.0/mbSize, "", []string(nil)).Return().Times(1)
	mock.On("Gauge", "system.swap.cached", 25000000000.0/mbSize, "", []string(nil)).Return().Times(1)
	mock.On("Commit").Return().Times(1)
	err := memCheck.Run()
	require.Nil(t, err)

	mock.AssertExpectations(t)
	mock.AssertNumberOfCalls(t, "Gauge", 13)
	mock.AssertNumberOfCalls(t, "Commit", 1)
}

func TestVirtualMemoryError(t *testing.T) {
	virtualMemory = func() (*mem.VirtualMemoryStat, error) { return nil, fmt.Errorf("some error") }
	swapMemory = SwapMemory
	memCheck := new(MemoryCheck)

	mock := mocksender.NewMockSender(memCheck.ID())

	runtimeOS = "linux"

	mock.On("Gauge", "system.swap.total", 100000.0/mbSize, "", []string(nil)).Return().Times(1)
	mock.On("Gauge", "system.swap.free", 60000.0/mbSize, "", []string(nil)).Return().Times(1)
	mock.On("Gauge", "system.swap.used", 40000.0/mbSize, "", []string(nil)).Return().Times(1)
	mock.On("Gauge", "system.swap.pct_free", 0.6, "", []string(nil)).Return().Times(1)
	mock.On("Commit").Return().Times(1)
	err := memCheck.Run()
	require.Nil(t, err)

	mock.AssertExpectations(t)
	mock.AssertNumberOfCalls(t, "Gauge", 4)
	mock.AssertNumberOfCalls(t, "Commit", 1)
}
