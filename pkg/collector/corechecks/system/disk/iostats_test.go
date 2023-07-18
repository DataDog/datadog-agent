// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.
//go:build !windows

package disk

import (
	"regexp"
	"runtime"
	"testing"
	"time"

	"github.com/shirou/gopsutil/v3/disk"
	"github.com/shirou/gopsutil/v3/mem"

	"github.com/DataDog/datadog-agent/pkg/aggregator/mocksender"
	"github.com/DataDog/datadog-agent/pkg/autodiscovery/integration"
)

var (
	ioSamples = []map[string]disk.IOCountersStat{
		{
			"sda": {
				ReadCount:        443071,
				MergedReadCount:  104744,
				WriteCount:       10412454,
				MergedWriteCount: 310860,
				ReadBytes:        849293 * SectorSize,
				WriteBytes:       1406995 * SectorSize,
				ReadTime:         19699308,
				WriteTime:        418600,
				IopsInProgress:   0,
				IoTime:           343324,
				WeightedIO:       727464,
				Name:             "sda",
				SerialNumber:     "123456789WD",
			},
		}, {
			"sda": {
				ReadCount:        443071,
				MergedReadCount:  104744,
				WriteCount:       10412454,
				MergedWriteCount: 310860,
				ReadBytes:        849414 * SectorSize,
				WriteBytes:       1407070 * SectorSize,
				ReadTime:         19700964,
				WriteTime:        418628,
				IopsInProgress:   0,
				IoTime:           343352,
				WeightedIO:       727492,
				Name:             "sda",
				SerialNumber:     "123456789WD",
			},
		},
	}
	ioSamplesDM = []map[string]disk.IOCountersStat{
		{
			"dm0": {
				ReadCount:        443071,
				MergedReadCount:  104744,
				WriteCount:       10412454,
				MergedWriteCount: 310860,
				ReadBytes:        849293 * SectorSize,
				WriteBytes:       1406995 * SectorSize,
				ReadTime:         19699308,
				WriteTime:        418600,
				IopsInProgress:   0,
				IoTime:           343324,
				WeightedIO:       727464,
				Name:             "sda",
				SerialNumber:     "987654321WD",
				Label:            "virtual-1",
			},
		},
	}
)

var sampleIdx = 0

var (
	ioSampler   = func(names ...string) (map[string]disk.IOCountersStat, error) { return sampler(ioSamples, names...) }
	ioSamplerDM = func(names ...string) (map[string]disk.IOCountersStat, error) { return sampler(ioSamplesDM, names...) }
)

func SwapMemory() (*mem.SwapMemoryStat, error) {
	return &mem.SwapMemoryStat{
		Total:       100000,
		Used:        40000,
		Free:        60000,
		UsedPercent: 40,
		Sin:         21,
		Sout:        22,
		PgIn:        23,
		PgOut:       24,
	}, nil
}

func sampler(samples []map[string]disk.IOCountersStat, names ...string) (map[string]disk.IOCountersStat, error) {
	idx := sampleIdx
	sampleIdx++
	sampleIdx = sampleIdx % len(samples)
	return ioSamples[idx], nil
}

func TestIOCheckDM(t *testing.T) {
	ioCounters = ioSamplerDM
	swapMemory = SwapMemory
	ioCheck := new(IOCheck)
	ioCheck.Configure(integration.FakeConfigHash, nil, nil, "test")

	mock := mocksender.NewMockSender(ioCheck.ID())

	switch os := runtime.GOOS; os {
	case "windows":
		mock.On("Rate", "system.io.r_s", 443071.0, "", []string{"device:C:"}).Return().Times(1)
		mock.On("Rate", "system.io.w_s", 10412454.0, "", []string{"device:C:"}).Return().Times(1)
	default: // Should cover Unices (Linux, OSX, FreeBSD,...)
		mock.On("Rate", "system.io.r_s", 443071.0, "", []string{"device:dm0", "device_label:virtual-1"}).Return().Times(1)
		mock.On("Rate", "system.io.w_s", 10412454.0, "", []string{"device:dm0", "device_label:virtual-1"}).Return().Times(1)
		mock.On("Rate", "system.io.rrqm_s", 104744.0, "", []string{"device:dm0", "device_label:virtual-1"}).Return().Times(1)
		mock.On("Rate", "system.io.wrqm_s", 310860.0, "", []string{"device:dm0", "device_label:virtual-1"}).Return().Times(1)
		mock.On("Rate", "system.io.block_in", 23.0, "", []string(nil)).Return().Times(1)
		mock.On("Rate", "system.io.block_out", 24.0, "", []string(nil)).Return().Times(1)
	}
}

func TestIOCheck(t *testing.T) {
	startNow := time.Now().UnixNano()
	nowNano = func() int64 { return startNow } // time of the first run
	defer func() { nowNano = time.Now().UnixNano }()

	ioCounters = ioSampler
	swapMemory = SwapMemory
	ioCheck := new(IOCheck)
	ioCheck.Configure(integration.FakeConfigHash, nil, nil, "test")

	mock := mocksender.NewMockSender(ioCheck.ID())

	expectedRates := 2
	expectedGauges := 0

	switch os := runtime.GOOS; os {
	case "windows":
		mock.On("Rate", "system.io.r_s", 443071.0, "", []string{"device:C:"}).Return().Times(1)
		mock.On("Rate", "system.io.w_s", 10412454.0, "", []string{"device:C:"}).Return().Times(1)
	default: // Should cover Unices (Linux, OSX, FreeBSD,...)
		mock.On("Rate", "system.io.r_s", 443071.0, "", []string{"device:sda", "device_name:sda"}).Return().Times(1)
		mock.On("Rate", "system.io.w_s", 10412454.0, "", []string{"device:sda", "device_name:sda"}).Return().Times(1)
		mock.On("Rate", "system.io.rrqm_s", 104744.0, "", []string{"device:sda", "device_name:sda"}).Return().Times(1)
		mock.On("Rate", "system.io.wrqm_s", 310860.0, "", []string{"device:sda", "device_name:sda"}).Return().Times(1)
		mock.On("Rate", "system.io.block_in", 23.0, "", []string(nil)).Return().Times(1)
		mock.On("Rate", "system.io.block_out", 24.0, "", []string(nil)).Return().Times(1)
		expectedRates += 4
	}
	mock.On("Commit").Return().Times(1)

	ioCheck.Run()
	mock.AssertExpectations(t)
	mock.AssertNumberOfCalls(t, "Gauge", expectedGauges)
	mock.AssertNumberOfCalls(t, "Rate", expectedRates)
	mock.AssertNumberOfCalls(t, "Commit", 1)

	// simulate a 1s interval
	nowNano = func() int64 { return startNow + int64(1*time.Second) } // time of the second run

	switch os := runtime.GOOS; os {
	case "windows":
		mock.On("Gauge", "system.io.r_s", 443071.0, "", []string{"device:sda", "device_name:sda"}).Return().Times(1)
		mock.On("Gauge", "system.io.w_s", 10412454.0, "", []string{"device:sda", "device_name:sda"}).Return().Times(1)
	default: // Should cover Unices (Linux, OSX, FreeBSD,...)
		mock.On("Rate", "system.io.r_s", 443071.0, "", []string{"device:sda", "device_name:sda"}).Return().Times(1)
		mock.On("Rate", "system.io.w_s", 10412454.0, "", []string{"device:sda", "device_name:sda"}).Return().Times(1)
		mock.On("Rate", "system.io.rrqm_s", 104744.0, "", []string{"device:sda", "device_name:sda"}).Return().Times(1)
		mock.On("Rate", "system.io.wrqm_s", 310860.0, "", []string{"device:sda", "device_name:sda"}).Return().Times(1)
		mock.On("Gauge", "system.io.rkb_s", 60.5, "", []string{"device:sda", "device_name:sda"}).Return().Times(1)
		mock.On("Gauge", "system.io.wkb_s", 37.5, "", []string{"device:sda", "device_name:sda"}).Return().Times(1)
		mock.On("Gauge", "system.io.avg_rq_sz", 0.0, "", []string{"device:sda", "device_name:sda"}).Return().Times(1)
		mock.On("Gauge", "system.io.await", 0.0, "", []string{"device:sda", "device_name:sda"}).Return().Times(1)
		mock.On("Gauge", "system.io.r_await", 0.0, "", []string{"device:sda", "device_name:sda"}).Return().Times(1)
		mock.On("Gauge", "system.io.w_await", 0.0, "", []string{"device:sda", "device_name:sda"}).Return().Times(1)
		mock.On("Gauge", "system.io.avg_q_sz", 0.03, "", []string{"device:sda", "device_name:sda"}).Return().Times(1)
		mock.On("Gauge", "system.io.util", 2.8, "", []string{"device:sda", "device_name:sda"}).Return().Times(1)
		mock.On("Gauge", "system.io.svctm", 0.0, "", []string{"device:sda", "device_name:sda"}).Return().Times(1)
		mock.On("Rate", "system.io.block_in", 23.0, "", []string(nil)).Return().Times(1)
		mock.On("Rate", "system.io.block_out", 24.0, "", []string(nil)).Return().Times(1)
		expectedRates += 6
		expectedGauges += 9
	}

	mock.On("Commit").Return().Times(1)

	ioCheck.Run()
	mock.AssertExpectations(t)
	mock.AssertNumberOfCalls(t, "Gauge", expectedGauges)
	mock.AssertNumberOfCalls(t, "Rate", expectedRates)
	mock.AssertNumberOfCalls(t, "Commit", 2)
}

func TestIOCheckBlacklist(t *testing.T) {
	ioCounters = ioSampler
	swapMemory = SwapMemory
	ioCheck := new(IOCheck)
	ioCheck.Configure(integration.FakeConfigHash, nil, nil, "test")

	mock := mocksender.NewMockSender(ioCheck.ID())

	expectedRates := 0
	expectedGauges := 0

	// set blacklist
	bl, err := regexp.Compile("sd.*")
	if err != nil {
		t.FailNow()
	}
	ioCheck.blacklist = bl

	switch os := runtime.GOOS; os {
	case "windows":
		break
	default: // Should cover Unices (Linux, OSX, FreeBSD,...)
		mock.On("Rate", "system.io.block_in", 23.0, "", []string(nil)).Return().Times(1)
		mock.On("Rate", "system.io.block_out", 24.0, "", []string(nil)).Return().Times(1)
		expectedRates += 2
	}

	mock.On("Commit").Return().Times(1)

	ioCheck.Run()
	mock.AssertExpectations(t)
	mock.AssertNumberOfCalls(t, "Gauge", expectedGauges)
	mock.AssertNumberOfCalls(t, "Rate", expectedRates)
	mock.AssertNumberOfCalls(t, "Commit", 1)
}
