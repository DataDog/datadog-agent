// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-2019 Datadog, Inc.

// +build !windows

package system

import (
	"math"
	"testing"
	"time"

	"github.com/DataDog/datadog-agent/pkg/aggregator/mocksender"
	"github.com/shirou/gopsutil/disk"
	"github.com/stretchr/testify/assert"
)

var currentStats = map[string]disk.IOCountersStat{
	"sda": {
		ReadCount:        42,
		MergedReadCount:  42,
		WriteCount:       42,
		MergedWriteCount: 42,
		ReadBytes:        42 * kB,
		WriteBytes:       42 * kB,
		ReadTime:         42,
		WriteTime:        42,
		IopsInProgress:   0,
		IoTime:           42,
		WeightedIO:       42 * kB,
		Name:             "sda",
		SerialNumber:     "123456789WD",
	},
}

var lastStats = map[string]disk.IOCountersStat{
	"sda": {
		ReadCount:        uint64(maxLong),
		MergedReadCount:  uint64(maxLong),
		WriteCount:       uint64(maxLong),
		MergedWriteCount: uint64(maxLong),
		ReadBytes:        uint64(maxLong),
		WriteBytes:       uint64(maxLong),
		ReadTime:         uint64(math.MaxUint32),
		WriteTime:        uint64(math.MaxUint32),
		IopsInProgress:   0,
		IoTime:           uint64(maxLong),
		WeightedIO:       uint64(maxLong),
		Name:             "sda",
		SerialNumber:     "123456789WD",
	},
}

func TestWithRealValues(t *testing.T) {
	increment := incrementWithOverflow(6176672, 4292830204, math.MaxUint32)
	assert.Equal(t, int64(8313763), increment)
}

func TestIncrementWithOverflow(t *testing.T) {
	prev := uint64(maxLong) - 2
	for i := -1; i < 2; i++ {
		curr := uint64(maxLong) + uint64(i)
		if curr >= uint64(maxLong) {
			curr -= uint64(maxLong)
		}
		increment := incrementWithOverflow(curr, prev, maxLong)
		assert.Equal(t, int64(1), increment)
		prev = curr
	}
}

func TestIoStatsOverflow(t *testing.T) {

	ioCheck := new(IOCheck)
	ioCheck.Configure(nil, nil)
	ioCheck.stats = lastStats
	ioCheck.ts = 1000
	ioCounters = func(names ...string) (map[string]disk.IOCountersStat, error) {
		return currentStats, nil
	}

	mock := mocksender.NewMockSender(ioCheck.ID())

	mock.On("Rate", "system.io.r_s", 42.0, "", []string{"device:sda"}).Return().Times(1)
	mock.On("Rate", "system.io.w_s", 42.0, "", []string{"device:sda"}).Return().Times(1)
	mock.On("Rate", "system.io.rrqm_s", 42.0, "", []string{"device:sda"}).Return().Times(1)
	mock.On("Rate", "system.io.wrqm_s", 42.0, "", []string{"device:sda"}).Return().Times(1)
	mock.On("Gauge", "system.io.rkb_s", 42.0, "", []string{"device:sda"}).Return().Times(1)
	mock.On("Gauge", "system.io.wkb_s", 42.0, "", []string{"device:sda"}).Return().Times(1)
	mock.On("Gauge", "system.io.avg_rq_sz", 2.0, "", []string{"device:sda"}).Return().Times(1)
	mock.On("Gauge", "system.io.await", 1.0, "", []string{"device:sda"}).Return().Times(1)
	mock.On("Gauge", "system.io.r_await", 1.0, "", []string{"device:sda"}).Return().Times(1)
	mock.On("Gauge", "system.io.w_await", 1.0, "", []string{"device:sda"}).Return().Times(1)
	mock.On("Gauge", "system.io.avg_q_sz", 42.0, "", []string{"device:sda"}).Return().Times(1)
	mock.On("Gauge", "system.io.util", 4.2, "", []string{"device:sda"}).Return().Times(1)
	mock.On("Gauge", "system.io.svctm", 0.5, "", []string{"device:sda"}).Return().Times(1)
	mock.On("Commit").Return().Times(1)

	// simulate a 1s interval
	nowNano = func() int64 { return 2000 * 1000000 } // time of the second run
	defer func() { nowNano = time.Now().UnixNano }()

	ioCheck.Run()

	mock.AssertExpectations(t)
}
