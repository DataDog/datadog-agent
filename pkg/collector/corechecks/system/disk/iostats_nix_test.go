// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build !windows

package disk

import (
	"math"
	"testing"
	"time"

	"github.com/shirou/gopsutil/v3/disk"
	"github.com/stretchr/testify/assert"

	"github.com/DataDog/datadog-agent/pkg/aggregator/mocksender"
	"github.com/DataDog/datadog-agent/pkg/autodiscovery/integration"
)

var currentStats = map[string]disk.IOCountersStat{
	"sda": {
		ReadCount:        41,
		MergedReadCount:  41,
		WriteCount:       41,
		MergedWriteCount: 41,
		ReadBytes:        42 * kB,
		WriteBytes:       42 * kB,
		ReadTime:         41,
		WriteTime:        41,
		IopsInProgress:   0,
		IoTime:           41,
		WeightedIO:       42 * kB,
		Name:             "sda",
		SerialNumber:     "123456789WD",
	},
}

var lastStats = map[string]disk.IOCountersStat{
	"sda": {
		ReadCount:        maxULong,
		MergedReadCount:  maxULong,
		WriteCount:       maxULong,
		MergedWriteCount: maxULong,
		ReadBytes:        maxULong,
		WriteBytes:       maxULong,
		ReadTime:         uint64(math.MaxUint32),
		WriteTime:        uint64(math.MaxUint32),
		IopsInProgress:   0,
		IoTime:           maxULong,
		WeightedIO:       uint64(math.MaxUint32),
		Name:             "sda",
		SerialNumber:     "123456789WD",
	},
}

func TestOverflow32(t *testing.T) {
	increment := incrementWithOverflow(0, math.MaxUint32)
	assert.Equal(t, int64(1), increment)
}

func TestOverflow64(t *testing.T) {
	increment := incrementWithOverflow(0, math.MaxUint64)
	assert.Equal(t, int64(1), increment)
}

func TestWithRealValues32(t *testing.T) {
	increment := incrementWithOverflow(123456, math.MaxUint32-2)
	assert.Equal(t, int64(123459), increment)
}

func TestWithRealValues64(t *testing.T) {
	increment := incrementWithOverflow(123456, math.MaxUint64-2)
	assert.Equal(t, int64(123459), increment)
}

func TestIncrementWithOverflow(t *testing.T) {
	assert.Equal(t, int64(1), incrementWithOverflow(maxULong-1, maxULong-2))
	assert.Equal(t, int64(1), incrementWithOverflow(maxULong, maxULong-1))
	assert.Equal(t, int64(1), incrementWithOverflow(0, maxULong))
	assert.Equal(t, int64(1), incrementWithOverflow(1, 0))
}

func TestIoStatsOverflow(t *testing.T) {
	ioCheck := new(IOCheck)
	mock := mocksender.NewMockSender(ioCheck.ID())
	ioCheck.Configure(mock.GetSenderManager(), integration.FakeConfigHash, nil, nil, "test")
	ioCheck.stats = lastStats
	ioCheck.ts = 1000
	ioCounters = func(names ...string) (map[string]disk.IOCountersStat, error) {
		return currentStats, nil
	}
	swapMemory = SwapMemory

	mock.On("Rate", "system.io.r_s", 41.0, "", []string{"device:sda", "device_name:sda"}).Return().Times(1)
	mock.On("Rate", "system.io.w_s", 41.0, "", []string{"device:sda", "device_name:sda"}).Return().Times(1)
	mock.On("Rate", "system.io.rrqm_s", 41.0, "", []string{"device:sda", "device_name:sda"}).Return().Times(1)
	mock.On("Rate", "system.io.wrqm_s", 41.0, "", []string{"device:sda", "device_name:sda"}).Return().Times(1)
	mock.On("Gauge", "system.io.rkb_s", 42.0, "", []string{"device:sda", "device_name:sda"}).Return().Times(1)
	mock.On("Gauge", "system.io.wkb_s", 42.0, "", []string{"device:sda", "device_name:sda"}).Return().Times(1)
	mock.On("Gauge", "system.io.avg_rq_sz", 2.0, "", []string{"device:sda", "device_name:sda"}).Return().Times(1)
	mock.On("Gauge", "system.io.await", 1.0, "", []string{"device:sda", "device_name:sda"}).Return().Times(1)
	mock.On("Gauge", "system.io.r_await", 1.0, "", []string{"device:sda", "device_name:sda"}).Return().Times(1)
	mock.On("Gauge", "system.io.w_await", 1.0, "", []string{"device:sda", "device_name:sda"}).Return().Times(1)
	mock.On("Gauge", "system.io.avg_q_sz", 42.0, "", []string{"device:sda", "device_name:sda"}).Return().Times(1)
	mock.On("Gauge", "system.io.util", 4.2, "", []string{"device:sda", "device_name:sda"}).Return().Times(1)
	mock.On("Gauge", "system.io.svctm", 0.5, "", []string{"device:sda", "device_name:sda"}).Return().Times(1)
	mock.On("Rate", "system.io.block_in", 23.0, "", []string(nil)).Return().Times(1)
	mock.On("Rate", "system.io.block_out", 24.0, "", []string(nil)).Return().Times(1)
	mock.On("Commit").Return().Times(1)

	// simulate a 1s interval
	nowNano = func() int64 { return 2000 * 1000000 } // time of the second run
	defer func() { nowNano = time.Now().UnixNano }()

	ioCheck.Run()

	mock.AssertExpectations(t)
}
